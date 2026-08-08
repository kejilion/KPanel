package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"time"
)

type ChatMessage struct {
	Role        string       `json:"role"`
	Name        string       `json:"name,omitempty"`
	Content     string       `json:"content,omitempty"`
	ToolCallID  string       `json:"toolCallId,omitempty"`
	ToolCalls   []ToolCall   `json:"toolCalls,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
	ReadOnly    bool            `json:"readOnly"`
}

type CompletionRequest struct {
	Model           string
	System          string
	Messages        []ChatMessage
	Tools           []ToolDefinition
	ThinkingLevel   ThinkingLevel
	NativeReasoning bool
}

type CompletionEvent struct {
	Delta     string
	ToolCalls []ToolCall
	Usage     Usage
	Done      bool
}

type ProviderError struct {
	Status    int
	Code      string
	Message   string
	Retryable bool
}

func (e *ProviderError) Error() string { return e.Message }

type ModelClient interface {
	Stream(context.Context, Provider, string, CompletionRequest, func(CompletionEvent) error) error
	Models(context.Context, Provider, string) ([]Model, error)
}

type hostResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type HTTPModelClient struct {
	resolver       hostResolver
	fallbackLookup func(context.Context, string) ([]net.IPAddr, error)
	timeout        time.Duration
}

func NewHTTPModelClient() *HTTPModelClient {
	return &HTTPModelClient{resolver: net.DefaultResolver, fallbackLookup: lookupViaPublicDNS, timeout: 180 * time.Second}
}

func chatMessageText(message ChatMessage) string {
	var value strings.Builder
	value.WriteString(message.Content)
	for _, attachment := range message.Attachments {
		if attachment.Kind != "text" {
			continue
		}
		if value.Len() > 0 {
			value.WriteString("\n\n")
		}
		value.WriteString("<untrusted_attachment name=")
		encodedName, _ := json.Marshal(attachment.Name)
		value.Write(encodedName)
		value.WriteString(">\n")
		value.Write(attachment.Data)
		value.WriteString("\n</untrusted_attachment>")
	}
	return value.String()
}

func openAIContent(message ChatMessage, responses bool) any {
	text := chatMessageText(message)
	hasImage := false
	for _, attachment := range message.Attachments {
		hasImage = hasImage || attachment.Kind == "image"
	}
	if !hasImage {
		return text
	}
	blocks := make([]map[string]any, 0, len(message.Attachments)+1)
	if text != "" {
		blockType := "text"
		if responses {
			blockType = "input_text"
		}
		blocks = append(blocks, map[string]any{"type": blockType, "text": text})
	}
	for _, attachment := range message.Attachments {
		if attachment.Kind != "image" {
			continue
		}
		dataURL := "data:" + attachment.MimeType + ";base64," + base64.StdEncoding.EncodeToString(attachment.Data)
		if responses {
			blocks = append(blocks, map[string]any{"type": "input_image", "image_url": dataURL})
		} else {
			blocks = append(blocks, map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURL}})
		}
	}
	return blocks
}

func (c *HTTPModelClient) Stream(ctx context.Context, provider Provider, apiKey string, request CompletionRequest, emit func(CompletionEvent) error) error {
	switch provider.Protocol {
	case ProtocolOpenAICompatible:
		if provider.APIMode == OpenAIResponses {
			return c.streamOpenAIResponses(ctx, provider, apiKey, request, emit)
		}
		return c.streamOpenAI(ctx, provider, apiKey, request, emit)
	case ProtocolAnthropic:
		return c.streamAnthropic(ctx, provider, apiKey, request, emit)
	case ProtocolGemini:
		return c.streamGemini(ctx, provider, apiKey, request, emit)
	default:
		return errors.New("unsupported provider protocol")
	}
}

func (c *HTTPModelClient) streamOpenAIResponses(ctx context.Context, provider Provider, apiKey string, request CompletionRequest, emit func(CompletionEvent) error) error {
	input := make([]any, 0, len(request.Messages))
	for _, message := range request.Messages {
		if (message.Content != "" || len(message.Attachments) > 0) && message.ToolCallID == "" {
			input = append(input, map[string]any{"role": message.Role, "content": openAIContent(message, true)})
		}
		for _, call := range message.ToolCalls {
			input = append(input, responsesProviderItems(provider.ID, call.ProviderData)...)
			input = append(input, map[string]any{
				"type": "function_call", "call_id": call.ID, "name": call.Name, "arguments": string(call.Arguments),
			})
		}
		if message.ToolCallID != "" {
			input = append(input, map[string]any{
				"type": "function_call_output", "call_id": message.ToolCallID, "output": message.Content,
			})
		}
	}
	payload := map[string]any{
		"model": request.Model, "instructions": request.System, "input": input, "stream": true, "store": false,
		"include": []string{"reasoning.encrypted_content"},
	}
	if request.NativeReasoning && request.ThinkingLevel.Valid() {
		payload["reasoning"] = map[string]any{"effort": request.ThinkingLevel}
	}
	if len(request.Tools) > 0 {
		tools := make([]map[string]any, 0, len(request.Tools))
		for _, tool := range request.Tools {
			var schema any
			_ = json.Unmarshal(tool.Schema, &schema)
			tools = append(tools, map[string]any{
				"type": "function", "name": tool.Name, "description": tool.Description, "parameters": schema,
			})
		}
		payload["tools"] = tools
	}
	response, err := c.do(ctx, provider, apiKey, http.MethodPost, "/responses", payload, map[string]string{"Authorization": "Bearer " + apiKey})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	toolParts := map[int]*ToolCall{}
	reasoningItems := []json.RawMessage{}
	textEmitted := false
	completed := false
	err = scanSSE(response.Body, func(event, data string) error {
		if data == "[DONE]" {
			if completed {
				return nil
			}
			completed = true
			return emit(responsesCompletionEvent(provider.ID, toolParts, reasoningItems, Usage{}))
		}
		var envelope struct {
			Item json.RawMessage `json:"item"`
		}
		_ = json.Unmarshal([]byte(data), &envelope)
		var chunk struct {
			Type        string `json:"type"`
			Delta       string `json:"delta"`
			OutputIndex int    `json:"output_index"`
			Arguments   string `json:"arguments"`
			Item        struct {
				Type      string `json:"type"`
				ID        string `json:"id"`
				CallID    string `json:"call_id"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"item"`
			Response struct {
				Error struct{ Code, Message string } `json:"error"`
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
					TotalTokens  int `json:"total_tokens"`
				} `json:"usage"`
				IncompleteDetails struct{ Reason string } `json:"incomplete_details"`
				Output            []json.RawMessage       `json:"output"`
			} `json:"response"`
			Error struct{ Code, Message string } `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return err
		}
		if chunk.Type == "" {
			chunk.Type = event
		}
		switch chunk.Type {
		case "response.output_text.delta":
			textEmitted = textEmitted || chunk.Delta != ""
			return emit(CompletionEvent{Delta: chunk.Delta})
		case "response.output_item.added":
			if chunk.Item.Type == "function_call" {
				id := chunk.Item.CallID
				if id == "" {
					id = chunk.Item.ID
				}
				toolParts[chunk.OutputIndex] = &ToolCall{ID: id, Name: chunk.Item.Name, Arguments: json.RawMessage(chunk.Item.Arguments)}
			}
		case "response.function_call_arguments.delta":
			if call := toolParts[chunk.OutputIndex]; call != nil {
				call.Arguments = append(call.Arguments, chunk.Delta...)
			}
		case "response.function_call_arguments.done":
			if call := toolParts[chunk.OutputIndex]; call != nil && chunk.Arguments != "" {
				call.Arguments = json.RawMessage(chunk.Arguments)
			}
		case "response.output_item.done":
			if chunk.Item.Type == "reasoning" && len(envelope.Item) > 0 {
				reasoningItems = append(reasoningItems, append(json.RawMessage(nil), envelope.Item...))
			}
			if chunk.Item.Type == "function_call" {
				call := toolParts[chunk.OutputIndex]
				if call == nil {
					id := chunk.Item.CallID
					if id == "" {
						id = chunk.Item.ID
					}
					call = &ToolCall{ID: id}
					toolParts[chunk.OutputIndex] = call
				}
				if chunk.Item.CallID != "" {
					call.ID = chunk.Item.CallID
				}
				if chunk.Item.Name != "" {
					call.Name = chunk.Item.Name
				}
				if chunk.Item.Arguments != "" {
					call.Arguments = json.RawMessage(chunk.Item.Arguments)
				}
			}
		case "response.completed":
			completed = true
			var fallbackText strings.Builder
			hadReasoningItems := len(reasoningItems) > 0
			for index, item := range chunk.Response.Output {
				var output struct {
					Type      string `json:"type"`
					ID        string `json:"id"`
					CallID    string `json:"call_id"`
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
					Content   []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				}
				if json.Unmarshal(item, &output) != nil {
					continue
				}
				switch output.Type {
				case "reasoning":
					if !hadReasoningItems {
						reasoningItems = append(reasoningItems, append(json.RawMessage(nil), item...))
					}
				case "function_call":
					if toolParts[index] == nil {
						id := output.CallID
						if id == "" {
							id = output.ID
						}
						toolParts[index] = &ToolCall{ID: id, Name: output.Name, Arguments: json.RawMessage(output.Arguments)}
					}
				case "message":
					if !textEmitted {
						for _, part := range output.Content {
							if part.Type == "output_text" {
								fallbackText.WriteString(part.Text)
							}
						}
					}
				}
			}
			if fallbackText.Len() > 0 {
				textEmitted = true
				if err := emit(CompletionEvent{Delta: fallbackText.String()}); err != nil {
					return err
				}
			}
			usage := Usage{InputTokens: chunk.Response.Usage.InputTokens, OutputTokens: chunk.Response.Usage.OutputTokens, TotalTokens: chunk.Response.Usage.TotalTokens}
			if usage.TotalTokens == 0 {
				usage.TotalTokens = usage.InputTokens + usage.OutputTokens
			}
			return emit(responsesCompletionEvent(provider.ID, toolParts, reasoningItems, usage))
		case "response.failed", "response.incomplete":
			message := chunk.Response.Error.Message
			code := chunk.Response.Error.Code
			if message == "" {
				message = chunk.Response.IncompleteDetails.Reason
			}
			if message == "" {
				message = chunk.Type
			}
			return &ProviderError{Status: response.StatusCode, Code: code, Message: redactAndLimit(strings.ReplaceAll(message, apiKey, "[REDACTED]"), 1000)}
		case "error":
			message := chunk.Error.Message
			if message == "" {
				message = "provider stream error"
			}
			return &ProviderError{Status: response.StatusCode, Code: chunk.Error.Code, Message: redactAndLimit(strings.ReplaceAll(message, apiKey, "[REDACTED]"), 1000)}
		}
		return nil
	})
	if err == nil && !completed {
		return io.ErrUnexpectedEOF
	}
	return err
}

type responsesNativeContext struct {
	ProviderID string            `json:"providerId"`
	Items      []json.RawMessage `json:"items"`
}

func responsesProviderItems(providerID string, raw json.RawMessage) []any {
	if len(raw) == 0 {
		return nil
	}
	var native responsesNativeContext
	if json.Unmarshal(raw, &native) != nil || native.ProviderID != providerID {
		return nil
	}
	items := make([]any, 0, len(native.Items))
	for _, rawItem := range native.Items {
		if !isResponsesReasoningItem(rawItem) {
			continue
		}
		var item any
		if json.Unmarshal(rawItem, &item) == nil {
			items = append(items, item)
		}
	}
	return items
}

func responsesCompletionEvent(providerID string, parts map[int]*ToolCall, reasoningItems []json.RawMessage, usage Usage) CompletionEvent {
	calls := sortedToolCalls(parts)
	if len(calls) > 0 && len(reasoningItems) > 0 {
		raw, err := json.Marshal(responsesNativeContext{ProviderID: providerID, Items: reasoningItems})
		if err == nil {
			calls[0].ProviderData = raw
		}
	}
	return CompletionEvent{Done: true, ToolCalls: calls, Usage: usage}
}

func isResponsesReasoningItem(raw json.RawMessage) bool {
	var item struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(raw, &item) == nil && item.Type == "reasoning"
}

func (c *HTTPModelClient) client(provider Provider) *http.Client {
	dialer := &net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{ForceAttemptHTTP2: true, MaxIdleConns: 10, IdleConnTimeout: 60 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := c.resolveProviderHost(ctx, provider, host)
		if err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
	}
	return &http.Client{Transport: transport, Timeout: c.timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("too many provider redirects")
		}
		validationURL := *req.URL
		validationURL.RawQuery = ""
		if _, err := ValidateProviderURL(validationURL.String(), provider.EndpointScope); err != nil {
			return err
		}
		if len(via) > 0 && !sameOrigin(via[0].URL, req.URL) {
			req.Header.Del("Authorization")
			req.Header.Del("X-Api-Key")
			req.Header.Del("X-Goog-Api-Key")
		}
		return nil
	}}
}

// resolveProviderHost 解析 Provider 域名并返回经过端点范围校验的地址列表。
// 系统 DNS 处于 fake-ip 代理环境时（解析结果包含非公网或 fake-ip 保留段），
// 回退到公共 DNS 重新解析以获取真实 IP，回退结果仍须通过端点范围校验。
func (c *HTTPModelClient) resolveProviderHost(ctx context.Context, provider Provider, host string) ([]net.IPAddr, error) {
	addresses, err := c.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if provider.EndpointScope == EndpointPublic && anyResolvedAreFakeIP(addresses) && c.fallbackLookup != nil {
		fallback, fallbackErr := c.fallbackLookup(ctx, host)
		if fallbackErr == nil && len(fallback) > 0 {
			addresses = fallback
		} else if fallbackErr != nil {
			slog.Debug("AI provider fallback DNS resolution failed", "host", host, "error", fallbackErr)
		}
	}
	if err := ValidateResolvedAddresses(provider.EndpointScope, addresses); err != nil {
		return nil, err
	}
	return addresses, nil
}

// fake-ip 保留段：RFC 2544 benchmark 段（Clash 等默认 fake-ip-range）、
// CGNAT 段和 240/4 保留段。这些段在 Go 中判定为公网单播地址，
// 但仅经代理 TUN 拦截才有意义，直连必然失败。
var (
	benchmarkPrefix = netip.MustParsePrefix("198.18.0.0/15")
	cgNATPrefix     = netip.MustParsePrefix("100.64.0.0/10")
	reservedPrefix  = netip.MustParsePrefix("240.0.0.0/4")
)

func isFakeIPAddress(ip net.IP) bool {
	if isBlockedIP(ip) {
		return true
	}
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	return benchmarkPrefix.Contains(address) || cgNATPrefix.Contains(address) || reservedPrefix.Contains(address)
}

func anyResolvedAreFakeIP(addresses []net.IPAddr) bool {
	for _, address := range addresses {
		if isFakeIPAddress(address.IP) {
			return true
		}
	}
	return false
}

// publicDNSServers 是 fake-ip 环境下的回退解析服务器列表。
var publicDNSServers = []string{"223.5.5.5", "119.29.29.29", "114.114.114.114", "1.1.1.1", "8.8.8.8"}

func lookupViaPublicDNS(ctx context.Context, host string) ([]net.IPAddr, error) {
	return lookupViaServers(ctx, host, publicDNSServers)
}

// lookupViaServers 依次用指定 DNS 服务器解析主机名，返回第一个成功的地址集，
// 结果优先排序 IPv4，避免主机有 IPv6 时拨号首选不可达的 IPv6 地址。
func lookupViaServers(ctx context.Context, host string, servers []string) ([]net.IPAddr, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var lastErr error
	for _, server := range servers {
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				d := net.Dialer{Timeout: 5 * time.Second}
				return d.DialContext(ctx, network, serverWithDefaultPort(server))
			},
		}
		addresses, err := resolver.LookupIPAddr(ctx, host)
		if err == nil && len(addresses) > 0 {
			return preferIPv4(addresses), nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("public DNS resolution returned no addresses")
	}
	return nil, lastErr
}

// serverWithDefaultPort 为不带端口的 DNS 服务器地址补上默认 53 端口，
// 已带端口（如测试中的本地随机端口 stub）则原样返回。
func serverWithDefaultPort(server string) string {
	if _, _, err := net.SplitHostPort(server); err == nil {
		return server
	}
	return net.JoinHostPort(server, "53")
}

func preferIPv4(addresses []net.IPAddr) []net.IPAddr {
	sorted := append([]net.IPAddr(nil), addresses...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].IP.To4() != nil && sorted[j].IP.To4() == nil
	})
	return sorted
}

func (c *HTTPModelClient) do(ctx context.Context, provider Provider, apiKey, method, endpoint string, payload any, headers map[string]string) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(provider.BaseURL, "/")+endpoint, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/event-stream")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := c.client(provider).Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(response.Body, 32<<10))
		providerErr := providerHTTPError(response.StatusCode, data).(*ProviderError)
		if apiKey != "" {
			providerErr.Message = strings.ReplaceAll(providerErr.Message, apiKey, "[REDACTED]")
		}
		providerErr.Message = redactAndLimit(providerErr.Message, 1000)
		return nil, providerErr
	}
	return response, nil
}

func (c *HTTPModelClient) streamOpenAI(ctx context.Context, provider Provider, apiKey string, request CompletionRequest, emit func(CompletionEvent) error) error {
	messages := []map[string]any{{"role": "system", "content": request.System}}
	for _, message := range request.Messages {
		item := map[string]any{"role": message.Role, "content": openAIContent(message, false)}
		if message.ToolCallID != "" {
			item["tool_call_id"] = message.ToolCallID
		}
		if len(message.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				calls = append(calls, map[string]any{"id": call.ID, "type": "function", "function": map[string]any{"name": call.Name, "arguments": string(call.Arguments)}})
			}
			item["tool_calls"] = calls
		}
		messages = append(messages, item)
	}
	payload := map[string]any{"model": request.Model, "messages": messages, "stream": true, "stream_options": map[string]bool{"include_usage": true}}
	if request.NativeReasoning && request.ThinkingLevel.Valid() {
		payload["reasoning_effort"] = request.ThinkingLevel
	}
	if len(request.Tools) > 0 {
		tools := make([]map[string]any, 0, len(request.Tools))
		for _, tool := range request.Tools {
			var schema any
			_ = json.Unmarshal(tool.Schema, &schema)
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{"name": tool.Name, "description": tool.Description, "parameters": schema}})
		}
		payload["tools"] = tools
	}
	response, err := c.do(ctx, provider, apiKey, http.MethodPost, "/chat/completions", payload, map[string]string{"Authorization": "Bearer " + apiKey})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	toolParts := map[int]*ToolCall{}
	completed := false
	err = scanSSE(response.Body, func(event, data string) error {
		if data == "[DONE]" {
			completed = true
			return emit(CompletionEvent{Done: true, ToolCalls: sortedToolCalls(toolParts)})
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int                              `json:"index"`
						ID       string                           `json:"id"`
						Function struct{ Name, Arguments string } `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Usage struct{ PromptTokens, CompletionTokens int } `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return err
		}
		result := CompletionEvent{Usage: Usage{InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens}}
		result.Usage.TotalTokens = result.Usage.InputTokens + result.Usage.OutputTokens
		if len(chunk.Choices) > 0 {
			result.Delta = chunk.Choices[0].Delta.Content
			for _, part := range chunk.Choices[0].Delta.ToolCalls {
				call := toolParts[part.Index]
				if call == nil {
					call = &ToolCall{ID: part.ID}
					toolParts[part.Index] = call
				}
				if part.ID != "" {
					call.ID = part.ID
				}
				call.Name += part.Function.Name
				call.Arguments = append(call.Arguments, part.Function.Arguments...)
			}
		}
		return emit(result)
	})
	if err == nil && !completed {
		return io.ErrUnexpectedEOF
	}
	return err
}

func (c *HTTPModelClient) streamAnthropic(ctx context.Context, provider Provider, apiKey string, request CompletionRequest, emit func(CompletionEvent) error) error {
	messages := make([]map[string]any, 0, len(request.Messages))
	for _, message := range request.Messages {
		switch {
		case len(message.ToolCalls) > 0:
			blocks := make([]map[string]any, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				var input any = map[string]any{}
				_ = json.Unmarshal(call.Arguments, &input)
				blocks = append(blocks, map[string]any{"type": "tool_use", "id": call.ID, "name": call.Name, "input": input})
			}
			messages = append(messages, map[string]any{"role": "assistant", "content": blocks})
		case message.ToolCallID != "":
			messages = append(messages, map[string]any{"role": "user", "content": []map[string]any{{"type": "tool_result", "tool_use_id": message.ToolCallID, "content": message.Content}}})
		default:
			blocks := make([]map[string]any, 0, len(message.Attachments)+1)
			for _, attachment := range message.Attachments {
				if attachment.Kind == "image" {
					blocks = append(blocks, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": attachment.MimeType, "data": base64.StdEncoding.EncodeToString(attachment.Data)}})
				}
			}
			if text := chatMessageText(message); text != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": text})
			}
			messages = append(messages, map[string]any{"role": message.Role, "content": blocks})
		}
	}
	payload := map[string]any{"model": request.Model, "system": request.System, "messages": messages, "max_tokens": 4096, "stream": true}
	if request.NativeReasoning && request.ThinkingLevel.Valid() {
		payload["output_config"] = map[string]any{"effort": request.ThinkingLevel}
	}
	if len(request.Tools) > 0 {
		tools := make([]map[string]any, 0, len(request.Tools))
		for _, tool := range request.Tools {
			var schema any
			_ = json.Unmarshal(tool.Schema, &schema)
			tools = append(tools, map[string]any{"name": tool.Name, "description": tool.Description, "input_schema": schema})
		}
		payload["tools"] = tools
	}
	response, err := c.do(ctx, provider, apiKey, http.MethodPost, "/messages", payload, map[string]string{"X-Api-Key": apiKey, "Anthropic-Version": "2023-06-01"})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	toolParts := map[int]*ToolCall{}
	completed := false
	err = scanSSE(response.Body, func(event, data string) error {
		var chunk struct {
			Type         string                                   `json:"type"`
			Index        int                                      `json:"index"`
			ContentBlock struct{ Type, ID, Name string }          `json:"content_block"`
			Delta        struct{ Type, Text, PartialJSON string } `json:"delta"`
			Message      struct {
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return err
		}
		switch chunk.Type {
		case "content_block_start":
			if chunk.ContentBlock.Type == "tool_use" {
				toolParts[chunk.Index] = &ToolCall{ID: chunk.ContentBlock.ID, Name: chunk.ContentBlock.Name}
			}
		case "content_block_delta":
			if call := toolParts[chunk.Index]; call != nil {
				call.Arguments = append(call.Arguments, chunk.Delta.PartialJSON...)
			}
			return emit(CompletionEvent{Delta: chunk.Delta.Text})
		case "message_start":
			return emit(CompletionEvent{Usage: Usage{InputTokens: chunk.Message.Usage.InputTokens, OutputTokens: chunk.Message.Usage.OutputTokens}})
		case "message_delta":
			return emit(CompletionEvent{Usage: Usage{OutputTokens: chunk.Usage.OutputTokens}})
		case "message_stop":
			completed = true
			return emit(CompletionEvent{Done: true, ToolCalls: sortedToolCalls(toolParts)})
		}
		return nil
	})
	if err == nil && !completed {
		return io.ErrUnexpectedEOF
	}
	return err
}

func (c *HTTPModelClient) streamGemini(ctx context.Context, provider Provider, apiKey string, request CompletionRequest, emit func(CompletionEvent) error) error {
	contents := make([]map[string]any, 0, len(request.Messages))
	for _, message := range request.Messages {
		role := message.Role
		if role == "assistant" {
			role = "model"
		}
		parts := make([]map[string]any, 0, 1+len(message.ToolCalls))
		if message.ToolCallID == "" {
			if text := chatMessageText(message); text != "" {
				parts = append(parts, map[string]any{"text": text})
			}
			for _, attachment := range message.Attachments {
				if attachment.Kind == "image" {
					parts = append(parts, map[string]any{"inlineData": map[string]any{"mimeType": attachment.MimeType, "data": base64.StdEncoding.EncodeToString(attachment.Data)}})
				}
			}
		}
		for _, call := range message.ToolCalls {
			var args any = map[string]any{}
			_ = json.Unmarshal(call.Arguments, &args)
			parts = append(parts, map[string]any{"functionCall": map[string]any{"name": call.Name, "args": args}})
		}
		if message.ToolCallID != "" {
			role = "user"
			parts = append(parts, map[string]any{"functionResponse": map[string]any{"name": message.Name, "response": map[string]any{"content": message.Content}}})
		}
		contents = append(contents, map[string]any{"role": role, "parts": parts})
	}
	payload := map[string]any{"systemInstruction": map[string]any{"parts": []map[string]string{{"text": request.System}}}, "contents": contents}
	if request.NativeReasoning && request.ThinkingLevel.Valid() {
		payload["generationConfig"] = map[string]any{"thinkingConfig": map[string]any{"thinkingLevel": request.ThinkingLevel}}
	}
	if len(request.Tools) > 0 {
		declarations := make([]map[string]any, 0, len(request.Tools))
		for _, tool := range request.Tools {
			var schema any
			_ = json.Unmarshal(tool.Schema, &schema)
			declarations = append(declarations, map[string]any{"name": tool.Name, "description": tool.Description, "parameters": schema})
		}
		payload["tools"] = []map[string]any{{"functionDeclarations": declarations}}
	}
	endpoint := "/models/" + url.PathEscape(request.Model) + ":streamGenerateContent?alt=sse"
	response, err := c.do(ctx, provider, apiKey, http.MethodPost, endpoint, payload, map[string]string{"X-Goog-Api-Key": apiKey})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return scanSSE(response.Body, func(event, data string) error {
		var chunk struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text         string `json:"text"`
						FunctionCall struct {
							Name string          `json:"name"`
							Args json.RawMessage `json:"args"`
						} `json:"functionCall"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
			Usage struct {
				Prompt     int `json:"promptTokenCount"`
				Candidates int `json:"candidatesTokenCount"`
			} `json:"usageMetadata"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return err
		}
		result := CompletionEvent{Usage: Usage{InputTokens: chunk.Usage.Prompt, OutputTokens: chunk.Usage.Candidates}}
		if len(chunk.Candidates) > 0 {
			for _, part := range chunk.Candidates[0].Content.Parts {
				result.Delta += part.Text
				if part.FunctionCall.Name != "" {
					result.ToolCalls = append(result.ToolCalls, ToolCall{ID: newID("call"), Name: part.FunctionCall.Name, Arguments: part.FunctionCall.Args})
				}
			}
		}
		return emit(result)
	})
}

func (c *HTTPModelClient) Models(ctx context.Context, provider Provider, apiKey string) ([]Model, error) {
	if provider.Protocol == ProtocolAnthropic {
		return nil, errors.New("Anthropic model discovery is not available; add models manually")
	}
	endpoint, headers := "/models", map[string]string{"Authorization": "Bearer " + apiKey}
	if provider.Protocol == ProtocolGemini {
		endpoint = "/models"
		headers = map[string]string{"X-Goog-Api-Key": apiKey}
	}
	response, err := c.do(ctx, provider, apiKey, http.MethodGet, endpoint, nil, headers)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var raw struct {
		Data []struct {
			ID, Name, DisplayName string
			InputTokenLimit       int `json:"inputTokenLimit"`
		} `json:"data"`
		Models []struct {
			Name, DisplayName string
			InputTokenLimit   int `json:"inputTokenLimit"`
		} `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&raw); err != nil {
		return nil, err
	}
	items := []Model{}
	for _, item := range raw.Data {
		items = append(items, Model{ModelID: item.ID, DisplayName: item.ID, ContextWindow: 32_000, ToolCalling: true, Enabled: true})
	}
	for _, item := range raw.Models {
		items = append(items, Model{ModelID: strings.TrimPrefix(item.Name, "models/"), DisplayName: item.DisplayName, ContextWindow: item.InputTokenLimit, ToolCalling: true, Enabled: true})
	}
	return items, nil
}

func scanSSE(reader io.Reader, handler func(event, data string) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 2<<20)
	event, data := "", ""
	flush := func() error {
		if data == "" {
			return nil
		}
		err := handler(event, strings.TrimSuffix(data, "\n"))
		event, data = "", ""
		return err
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
		if strings.HasPrefix(line, "data:") {
			data += strings.TrimSpace(strings.TrimPrefix(line, "data:")) + "\n"
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return flush()
}

func sortedToolCalls(parts map[int]*ToolCall) []ToolCall {
	items := make([]ToolCall, 0, len(parts))
	indexes := make([]int, 0, len(parts))
	for index := range parts {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		if parts[index] != nil {
			items = append(items, *parts[index])
		}
	}
	return items
}

func providerHTTPError(status int, body []byte) error {
	message := strings.TrimSpace(string(body))
	var payload struct {
		Message string `json:"message"`
		Detail  string `json:"detail"`
		Title   string `json:"title"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil {
		switch {
		case strings.TrimSpace(payload.Error.Message) != "":
			message = payload.Error.Message
		case strings.TrimSpace(payload.Message) != "":
			message = payload.Message
		case strings.TrimSpace(payload.Detail) != "":
			message = payload.Detail
		case strings.TrimSpace(payload.Title) != "":
			message = payload.Title
		}
	} else if strings.Contains(strings.ToLower(message), "<html") || strings.Contains(strings.ToLower(message), "<!doctype") {
		if status == http.StatusRequestEntityTooLarge {
			message = "模型 API 拒绝了过大的请求（HTTP 413），请上传更小的图片或检查 API 网关请求体上限"
		} else {
			message = fmt.Sprintf("模型 API 拒绝了请求（HTTP %d），请确认所选模型支持当前输入（如图片）并检查 API 端点协议", status)
		}
	}
	if len(message) > 1000 {
		message = message[:1000]
	}
	if message == "" {
		message = http.StatusText(status)
	}
	return &ProviderError{Status: status, Code: fmt.Sprintf("provider_http_%d", status), Message: message, Retryable: status == 429 || status == 502 || status == 503 || status == 504}
}

func sameOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host)
}
