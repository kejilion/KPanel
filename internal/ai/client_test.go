package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderHTTPErrorHidesRawHTMLAndExtractsJSONMessage(t *testing.T) {
	htmlErr := providerHTTPError(http.StatusBadRequest, []byte("<html><head><title>400 Bad Request</title></head><body>nginx</body></html>"))
	if strings.Contains(strings.ToLower(htmlErr.Error()), "<html") || !strings.Contains(htmlErr.Error(), "HTTP 400") || !strings.Contains(htmlErr.Error(), "图片") {
		t.Fatalf("unsafe HTML provider error: %v", htmlErr)
	}
	jsonErr := providerHTTPError(http.StatusBadRequest, []byte(`{"error":{"message":"model does not support images"}}`))
	if jsonErr.Error() != "model does not support images" {
		t.Fatalf("JSON provider error=%v", jsonErr)
	}
}

func TestOpenAICompatibleStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("unexpected request %s, auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"你"}}]}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"好","tool_calls":[{"index":0,"id":"call_1","function":{"name":"host_","arguments":"{\"x\":"}}]}}]}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"read","arguments":"1}"}}]}}],"usage":{"prompt_tokens":5,"completion_tokens":3}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, "data: [DONE]")
		fmt.Fprintln(w)
	}))
	defer server.Close()
	client := NewHTTPModelClient()
	provider := Provider{Protocol: ProtocolOpenAICompatible, BaseURL: server.URL + "/v1", EndpointScope: EndpointPrivate}
	var text string
	var calls []ToolCall
	err := client.Stream(context.Background(), provider, "test-key", CompletionRequest{Model: "test"}, func(event CompletionEvent) error {
		text += event.Delta
		if event.Done {
			calls = event.ToolCalls
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "你好" || len(calls) != 1 || calls[0].Name != "host_read" || string(calls[0].Arguments) != `{"x":1}` {
		t.Fatalf("text=%q calls=%#v", text, calls)
	}
}

func TestOpenAIChatPreservesProviderReasoningAcrossToolCalls(t *testing.T) {
	for _, field := range []string{"reasoning_content", "reasoning_text", "reasoning"} {
		t.Run(field, func(t *testing.T) {
			requestNumber := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestNumber++
				var payload struct {
					Messages []map[string]any `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if requestNumber == 1 {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{%q:\"plan \"}}]}\n\n", field)
					fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{%q:\"next\",\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"docker_containers\",\"arguments\":\"{}\"}},{\"index\":1,\"id\":\"call_2\",\"function\":{\"name\":\"apps_list\",\"arguments\":\"{}\"}}]}}]}\n\n", field)
					fmt.Fprint(w, "data: [DONE]\n\n")
					return
				}
				var assistants []map[string]any
				for _, message := range payload.Messages {
					if message["role"] == "assistant" && message["tool_calls"] != nil {
						assistants = append(assistants, message)
					}
				}
				if len(assistants) != 2 {
					t.Fatalf("tool batch was not reconstructed: %#v", payload.Messages)
				}
				if requestNumber == 2 {
					for _, assistant := range assistants {
						if _, exists := assistant[field]; exists {
							t.Fatalf("standard continuation guessed a provider field: %#v", assistant)
						}
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprintf(w, `{"error":{"type":"invalid_request_error","message":"The %s in the thinking mode must be passed back to the API."}}`, field)
					return
				}
				for _, assistant := range assistants {
					if assistant[field] != "plan next" {
						t.Fatalf("reasoning was not replayed across the tool batch: %#v", payload.Messages)
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer server.Close()

			client := NewHTTPModelClient()
			provider := Provider{ID: "provider-" + field, Protocol: ProtocolOpenAICompatible, BaseURL: server.URL, EndpointScope: EndpointPrivate}
			var calls []ToolCall
			err := client.Stream(context.Background(), provider, "key", CompletionRequest{Model: "model", Messages: []ChatMessage{{Role: "user", Content: "inspect"}}}, func(event CompletionEvent) error {
				if event.Done {
					calls = event.ToolCalls
				}
				return nil
			})
			if err != nil || len(calls) != 2 || len(calls[0].ProviderData) == 0 || len(calls[1].ProviderData) != 0 {
				t.Fatalf("calls=%#v err=%v", calls, err)
			}
			publicCall, _ := json.Marshal(calls[0])
			if strings.Contains(string(publicCall), "plan next") || strings.Contains(string(publicCall), "providerData") {
				t.Fatalf("hidden reasoning leaked through public tool JSON: %s", publicCall)
			}
			err = client.Stream(context.Background(), provider, "key", CompletionRequest{Model: "model", Messages: []ChatMessage{
				{Role: "assistant", ToolCalls: calls[:1]},
				{Role: "tool", ToolCallID: calls[0].ID, Content: `{"ok":true}`},
				{Role: "assistant", ToolCalls: calls[1:]},
				{Role: "tool", ToolCallID: calls[1].ID, Content: `{"ok":true}`},
			}}, func(CompletionEvent) error { return nil })
			if err != nil || requestNumber != 3 {
				t.Fatalf("reasoning continuation requests=%d err=%v", requestNumber, err)
			}
		})
	}
}

func TestOpenAIChatAdaptsLegacyToolHistoryToRequiredReasoningField(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		var payload struct {
			Messages []map[string]any `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		var assistant map[string]any
		for _, message := range payload.Messages {
			if message["role"] == "assistant" && message["tool_calls"] != nil {
				assistant = message
				break
			}
		}
		if requestNumber == 1 {
			if _, exists := assistant["reasoning_text"]; exists {
				t.Fatalf("standard payload guessed a provider field: %#v", assistant)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"type":"invalid_request_error","message":"The reasoning_text in the thinking mode must be passed back to the API."}}`)
			return
		}
		if value, exists := assistant["reasoning_text"]; !exists || value != "" {
			t.Fatalf("compatibility retry did not pad legacy reasoning field: %#v", assistant)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewHTTPModelClient()
	provider := Provider{ID: "legacy-reasoning", Protocol: ProtocolOpenAICompatible, BaseURL: server.URL, EndpointScope: EndpointPrivate}
	request := CompletionRequest{Model: "model", Messages: []ChatMessage{
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_1", Name: "host_read", Arguments: json.RawMessage(`{}`)}}},
		{Role: "tool", ToolCallID: "call_1", Content: `{"ok":true}`},
	}}
	for attempt := 0; attempt < 2; attempt++ {
		if err := client.Stream(context.Background(), provider, "key", request, func(CompletionEvent) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if requestNumber != 3 {
		t.Fatalf("reasoning dialect was not cached, requests=%d", requestNumber)
	}
}

func TestOpenAIChatRetriesTextOnlyWhenOnlyHistoryContainsImages(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		body, _ := io.ReadAll(r.Body)
		payload := string(body)
		if requestNumber == 1 {
			if !strings.Contains(payload, `"type":"image_url"`) {
				t.Fatalf("first request did not preserve standard multimodal input: %s", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"type":"invalid_request_error","message":"Failed to deserialize the JSON body into the target type: messages[12]: unknown variant image_url, expected text"}}`)
			return
		}
		if strings.Contains(payload, `"type":"image_url"`) || strings.Contains(payload, "AQID") || !strings.Contains(payload, "unavailable_image_attachment") || !strings.Contains(payload, "continue") {
			t.Fatalf("text-only history fallback was unsafe or incomplete: %s", payload)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	image := Attachment{Name: "old.png", MimeType: "image/png", Kind: "image", Size: 3, Data: []byte{1, 2, 3}}
	client := NewHTTPModelClient()
	provider := Provider{ID: "text-only-history", Protocol: ProtocolOpenAICompatible, BaseURL: server.URL, EndpointScope: EndpointPrivate}
	request := CompletionRequest{Model: "model", Messages: []ChatMessage{
		{Role: "user", Content: "old image", Attachments: []Attachment{image}},
		{Role: "assistant", Content: "seen"},
		{Role: "user", Content: "continue", CurrentRun: true},
	}}
	for attempt := 0; attempt < 2; attempt++ {
		if err := client.Stream(context.Background(), provider, "key", request, func(CompletionEvent) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if requestNumber != 3 {
		t.Fatalf("text-only capability was not cached, requests=%d", requestNumber)
	}
}

func TestOpenAIChatNeverSilentlyDropsCurrentRunImage(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestNumber++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"type":"invalid_request_error","message":"unknown variant image_url, expected text"}}`)
	}))
	defer server.Close()

	image := Attachment{Name: "current.png", MimeType: "image/png", Kind: "image", Size: 3, Data: []byte{1, 2, 3}}
	client := NewHTTPModelClient()
	provider := Provider{ID: "text-only-current", Protocol: ProtocolOpenAICompatible, BaseURL: server.URL, EndpointScope: EndpointPrivate}
	request := CompletionRequest{Model: "model", Messages: []ChatMessage{{Role: "user", Content: "analyze", Attachments: []Attachment{image}, CurrentRun: true}}}
	for attempt := 0; attempt < 2; attempt++ {
		err := client.Stream(context.Background(), provider, "key", request, func(CompletionEvent) error { return nil })
		var providerErr *ProviderError
		if !errors.As(err, &providerErr) || providerErr.Code != "image_input_unsupported" || !strings.Contains(providerErr.Message, "图片未发送") {
			t.Fatalf("current image error=%#v", err)
		}
	}
	if requestNumber != 1 {
		t.Fatalf("current image must not be retried without its body, requests=%d", requestNumber)
	}
}

func TestOpenAIResponsesStreamAndToolRoundTrip(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		if r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("unexpected request %s, auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		payload := string(body)
		for _, marker := range []string{`"stream":true`, `"store":false`, `"reasoning.encrypted_content"`} {
			if !strings.Contains(payload, marker) {
				t.Fatalf("Responses payload missing %s: %s", marker, payload)
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if requestNumber == 1 {
			if !strings.Contains(payload, `"type":"function"`) || !strings.Contains(payload, `"name":"host_read"`) || strings.Contains(payload, `"function":{`) {
				t.Fatalf("Responses tools must use the flat function schema: %s", payload)
			}
			for _, event := range []string{
				`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs_1","encrypted_content":"opaque"}}`,
				`{"type":"response.output_text.delta","output_index":1,"delta":"working"}`,
				`{"type":"response.output_item.added","output_index":2,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"host_read","arguments":""}}`,
				`{"type":"response.function_call_arguments.delta","output_index":2,"delta":"{\"x\":"}`,
				`{"type":"response.function_call_arguments.delta","output_index":2,"delta":"1}"}`,
				`{"type":"response.function_call_arguments.done","output_index":2,"arguments":"{\"x\":1}"}`,
				`{"type":"response.output_item.done","output_index":2,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"host_read","arguments":"{\"x\":1}"}}`,
				`{"type":"response.completed","response":{"usage":{"input_tokens":7,"output_tokens":4,"total_tokens":11}}}`,
			} {
				fmt.Fprintf(w, "event: ignored\ndata: %s\n\n", event)
			}
			return
		}
		for _, marker := range []string{`"type":"reasoning"`, `"encrypted_content":"opaque"`, `"type":"function_call"`, `"call_id":"call_1"`, `"type":"function_call_output"`, `"output":"{\"ok\":true}"`} {
			if !strings.Contains(payload, marker) {
				t.Fatalf("tool continuation missing %s: %s", marker, payload)
			}
		}
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"done\"}]}],\"usage\":{\"input_tokens\":9,\"output_tokens\":2,\"total_tokens\":11}}}\n\n")
	}))
	defer server.Close()

	client := NewHTTPModelClient()
	provider := Provider{ID: "provider-1", Protocol: ProtocolOpenAICompatible, APIMode: OpenAIResponses, BaseURL: server.URL + "/v1", EndpointScope: EndpointPrivate}
	definition := ToolDefinition{Name: "host_read", Description: "Read host state", Schema: json.RawMessage(`{"type":"object","properties":{"x":{"type":"integer"}}}`), ReadOnly: true}
	var text string
	var calls []ToolCall
	var usage Usage
	err := client.Stream(context.Background(), provider, "test-key", CompletionRequest{Model: "test", System: "system", Messages: []ChatMessage{{Role: "user", Content: "inspect"}}, Tools: []ToolDefinition{definition}}, func(event CompletionEvent) error {
		text += event.Delta
		if event.Done {
			calls, usage = event.ToolCalls, event.Usage
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "working" || len(calls) != 1 || calls[0].ID != "call_1" || calls[0].Name != "host_read" || string(calls[0].Arguments) != `{"x":1}` {
		t.Fatalf("text=%q calls=%#v", text, calls)
	}
	if usage.InputTokens != 7 || usage.OutputTokens != 4 || usage.TotalTokens != 11 || len(calls[0].ProviderData) == 0 {
		t.Fatalf("usage=%#v providerData=%s", usage, calls[0].ProviderData)
	}
	publicCall, _ := json.Marshal(calls[0])
	if strings.Contains(string(publicCall), "opaque") || strings.Contains(string(publicCall), "providerData") {
		t.Fatalf("provider-native context leaked through public JSON: %s", publicCall)
	}

	text = ""
	err = client.Stream(context.Background(), provider, "test-key", CompletionRequest{Model: "test", System: "system", Messages: []ChatMessage{
		{Role: "assistant", ToolCalls: calls},
		{Role: "tool", ToolCallID: calls[0].ID, Content: `{"ok":true}`},
	}, Tools: []ToolDefinition{definition}}, func(event CompletionEvent) error { text += event.Delta; return nil })
	if err != nil || text != "done" {
		t.Fatalf("continuation text=%q err=%v", text, err)
	}
}

func TestOpenAIResponsesAdaptsToRequiredMessageIDs(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		var payload struct {
			Instructions string           `json:"instructions"`
			Input        []map[string]any `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if requestNumber == 1 {
			if payload.Instructions != "system" || len(payload.Input) != 4 {
				t.Fatalf("standard Responses payload changed: %#v", payload)
			}
			for _, item := range payload.Input {
				if _, exists := item["id"]; exists {
					t.Fatalf("standard first request must not guess provider extensions: %#v", payload.Input)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"message":"Upstream request failed: [invalid_request_error] Failed to deserialize the JSON body into the target type: messages[1]: missing field id at line 1 column 352"}}`)
			return
		}
		if payload.Instructions != "" || len(payload.Input) != 5 {
			t.Fatalf("compatibility retry must move system instructions into an identified message: %#v", payload)
		}
		wantIDs := []string{"", "msg_user", "msg_assistant", "", "msg_tool_result"}
		for index, item := range payload.Input {
			id, _ := item["id"].(string)
			if id == "" {
				t.Fatalf("compatibility item %d is missing id: %#v", index, payload.Input)
			}
			if wantIDs[index] != "" && id != wantIDs[index] {
				t.Fatalf("compatibility item %d id=%q want=%q", index, id, wantIDs[index])
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{}}\n\n")
	}))
	defer server.Close()

	client := NewHTTPModelClient()
	provider := Provider{ID: "provider-message-ids", Protocol: ProtocolOpenAICompatible, APIMode: OpenAIResponses, BaseURL: server.URL, EndpointScope: EndpointPrivate}
	request := CompletionRequest{Model: "model", System: "system", Messages: []ChatMessage{
		{ID: "msg_user", Role: "user", Content: "inspect"},
		{ID: "msg_assistant", Role: "assistant", Content: "checking"},
		{ID: "msg_tool_call", Role: "assistant", ToolCalls: []ToolCall{{ID: "call_1", Name: "host_read", Arguments: json.RawMessage(`{}`)}}},
		{ID: "msg_tool_result", Role: "tool", ToolCallID: "call_1", Content: `{"ok":true}`},
	}}
	for attempt := 0; attempt < 2; attempt++ {
		if err := client.Stream(context.Background(), provider, "key", request, func(CompletionEvent) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if requestNumber != 3 {
		t.Fatalf("message ID capability was not cached, requests=%d", requestNumber)
	}
}

func TestOpenAIResponsesMessageIDFallbackDoesNotReplayOutput(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestNumber++
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"error\",\"error\":{\"code\":\"invalid_request_error\",\"message\":\"messages[1]: missing field id\"}}\n\n")
	}))
	defer server.Close()

	var output strings.Builder
	err := NewHTTPModelClient().Stream(context.Background(), Provider{ID: "provider-no-replay", Protocol: ProtocolOpenAICompatible, APIMode: OpenAIResponses, BaseURL: server.URL, EndpointScope: EndpointPrivate}, "key", CompletionRequest{Model: "model", Messages: []ChatMessage{{Role: "user", Content: "inspect"}}}, func(event CompletionEvent) error {
		output.WriteString(event.Delta)
		return nil
	})
	if err == nil || output.String() != "partial" || requestNumber != 1 {
		t.Fatalf("output must not be replayed, output=%q requests=%d err=%v", output.String(), requestNumber, err)
	}
}

func TestOpenAIResponsesAdaptsToMessageIDStreamError(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		var payload struct {
			Input []map[string]any `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if requestNumber == 1 {
			fmt.Fprint(w, "data: {\"type\":\"error\",\"error\":{\"code\":\"invalid_request_error\",\"message\":\"messages[1]: missing field `id`\"}}\n\n")
			return
		}
		if len(payload.Input) != 1 || payload.Input[0]["id"] == "" {
			t.Fatalf("stream compatibility retry is missing generated ID: %#v", payload.Input)
		}
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{}}\n\n")
	}))
	defer server.Close()

	err := NewHTTPModelClient().Stream(context.Background(), Provider{ID: "provider-stream-message-ids", Protocol: ProtocolOpenAICompatible, APIMode: OpenAIResponses, BaseURL: server.URL, EndpointScope: EndpointPrivate}, "key", CompletionRequest{Model: "model", Messages: []ChatMessage{{Role: "user", Content: "inspect"}}}, func(CompletionEvent) error { return nil })
	if err != nil || requestNumber != 2 {
		t.Fatalf("stream message ID fallback failed, requests=%d err=%v", requestNumber, err)
	}
}

func TestOpenAIResponsesTerminalErrors(t *testing.T) {
	t.Run("failed event", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"model_error\",\"message\":\"generation failed\"}}}\n\n")
		}))
		defer server.Close()
		err := NewHTTPModelClient().Stream(context.Background(), Provider{Protocol: ProtocolOpenAICompatible, APIMode: OpenAIResponses, BaseURL: server.URL, EndpointScope: EndpointPrivate}, "key", CompletionRequest{Model: "test"}, func(CompletionEvent) error { return nil })
		var providerErr *ProviderError
		if !errors.As(err, &providerErr) || providerErr.Code != "model_error" || providerErr.Retryable {
			t.Fatalf("error=%#v", err)
		}
	})

	t.Run("truncated stream", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
		}))
		defer server.Close()
		err := NewHTTPModelClient().Stream(context.Background(), Provider{Protocol: ProtocolOpenAICompatible, APIMode: OpenAIResponses, BaseURL: server.URL, EndpointScope: EndpointPrivate}, "key", CompletionRequest{Model: "test"}, func(CompletionEvent) error { return nil })
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("error=%v", err)
		}
	})
}

func TestAnthropicStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, line := range []string{
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"host_read"}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"id\":1}"}}`,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"done"}}`,
			`data: {"type":"message_stop"}`,
		} {
			fmt.Fprintln(w, line)
			fmt.Fprintln(w)
		}
	}))
	defer server.Close()
	client := NewHTTPModelClient()
	var text string
	var calls []ToolCall
	err := client.Stream(context.Background(), Provider{Protocol: ProtocolAnthropic, BaseURL: server.URL, EndpointScope: EndpointPrivate}, "key", CompletionRequest{Model: "claude"}, func(event CompletionEvent) error {
		text += event.Delta
		if event.Done {
			calls = event.ToolCalls
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "done" || len(calls) != 1 || calls[0].Name != "host_read" {
		t.Fatalf("text=%q calls=%#v", text, calls)
	}
}

func TestGeminiStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ":streamGenerateContent") {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.Header.Get("X-Goog-Api-Key") != "key" || r.URL.Query().Get("key") != "" {
			t.Fatalf("Gemini key must only be sent in the request header")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `data: {"candidates":[{"content":{"parts":[{"text":"ok"},{"functionCall":{"name":"host_read","args":{"x":1}}}]}}]}`)
		fmt.Fprintln(w)
	}))
	defer server.Close()
	client := NewHTTPModelClient()
	var result CompletionEvent
	err := client.Stream(context.Background(), Provider{Protocol: ProtocolGemini, BaseURL: server.URL, EndpointScope: EndpointPrivate}, "key", CompletionRequest{Model: "gemini-test"}, func(event CompletionEvent) error { result = event; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if result.Delta != "ok" || len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "host_read" {
		t.Fatalf("event=%#v", result)
	}
}

func TestPortableToolSchemaIsPreservedForEveryProviderPayload(t *testing.T) {
	definition := ToolDefinition{
		Name:        "host_action",
		Description: "perform an action",
		Schema:      json.RawMessage(`{"type":"object","required":["action"],"properties":{"action":{"type":"string","enum":["inspect"]}},"additionalProperties":false}`),
	}
	tests := []struct {
		name     string
		provider Provider
		stream   string
	}{
		{name: "openai chat", provider: Provider{Protocol: ProtocolOpenAICompatible}, stream: "data: [DONE]\n\n"},
		{name: "openai responses", provider: Provider{Protocol: ProtocolOpenAICompatible, APIMode: OpenAIResponses}, stream: "data: {\"type\":\"response.completed\",\"response\":{}}\n\n"},
		{name: "anthropic", provider: Provider{Protocol: ProtocolAnthropic}, stream: "data: {\"type\":\"message_stop\"}\n\n"},
		{name: "gemini", provider: Provider{Protocol: ProtocolGemini}, stream: "data: {\"candidates\":[]}\n\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				tools, _ := payload["tools"].([]any)
				if len(tools) != 1 {
					t.Fatalf("tools=%#v", payload["tools"])
				}
				tool, _ := tools[0].(map[string]any)
				var schema map[string]any
				switch test.name {
				case "openai chat":
					function, _ := tool["function"].(map[string]any)
					schema, _ = function["parameters"].(map[string]any)
				case "openai responses":
					schema, _ = tool["parameters"].(map[string]any)
				case "anthropic":
					schema, _ = tool["input_schema"].(map[string]any)
				case "gemini":
					declarations, _ := tool["functionDeclarations"].([]any)
					if len(declarations) == 1 {
						declaration, _ := declarations[0].(map[string]any)
						schema, _ = declaration["parameters"].(map[string]any)
					}
				}
				properties, _ := schema["properties"].(map[string]any)
				action, _ := properties["action"].(map[string]any)
				if schema["type"] != "object" || action["type"] != "string" || schema["anyOf"] != nil {
					t.Fatalf("portable schema changed in %s payload: %#v", test.name, schema)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, test.stream)
			}))
			defer server.Close()
			provider := test.provider
			provider.BaseURL, provider.EndpointScope = server.URL, EndpointPrivate
			request := CompletionRequest{Model: "model", Messages: []ChatMessage{{Role: "user", Content: "hello"}}, Tools: []ToolDefinition{definition}}
			if err := NewHTTPModelClient().Stream(context.Background(), provider, "key", request, func(CompletionEvent) error { return nil }); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMultimodalAndThinkingPayloads(t *testing.T) {
	image := Attachment{Name: "screen.png", MimeType: "image/png", Kind: "image", Size: 3, Data: []byte{1, 2, 3}}
	tests := []struct {
		name     string
		provider Provider
		markers  []string
		stream   string
	}{
		{name: "openai chat", provider: Provider{Protocol: ProtocolOpenAICompatible}, markers: []string{`"reasoning_effort":"high"`, `"type":"image_url"`, `data:image/png;base64,AQID`}, stream: "data: [DONE]\n\n"},
		{name: "openai responses", provider: Provider{Protocol: ProtocolOpenAICompatible, APIMode: OpenAIResponses}, markers: []string{`"reasoning":{"effort":"high"}`, `"type":"input_image"`, `data:image/png;base64,AQID`}, stream: "data: {\"type\":\"response.completed\",\"response\":{}}\n\n"},
		{name: "anthropic", provider: Provider{Protocol: ProtocolAnthropic}, markers: []string{`"output_config":{"effort":"high"}`, `"type":"image"`, `"data":"AQID"`}, stream: "data: {\"type\":\"message_stop\"}\n\n"},
		{name: "gemini", provider: Provider{Protocol: ProtocolGemini}, markers: []string{`"thinkingConfig":{"thinkingLevel":"high"}`, `"inlineData"`, `"data":"AQID"`}, stream: "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]}}]}\n\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				for _, marker := range test.markers {
					if !strings.Contains(string(body), marker) {
						t.Fatalf("payload missing %s: %s", marker, body)
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, test.stream)
			}))
			defer server.Close()
			provider := test.provider
			provider.BaseURL, provider.EndpointScope = server.URL, EndpointPrivate
			err := NewHTTPModelClient().Stream(context.Background(), provider, "key", CompletionRequest{Model: "model", System: "system", Messages: []ChatMessage{{Role: "user", Content: "analyze", Attachments: []Attachment{image}}}, ThinkingLevel: ThinkingHigh, NativeReasoning: true}, func(CompletionEvent) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGeminiModelDiscoveryUsesHeaderKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" || r.Header.Get("X-Goog-Api-Key") != "key" || r.URL.RawQuery != "" {
			t.Fatalf("unexpected request path=%s query=%q key=%q", r.URL.Path, r.URL.RawQuery, r.Header.Get("X-Goog-Api-Key"))
		}
		fmt.Fprint(w, `{"models":[{"name":"models/gemini-test","displayName":"Gemini Test","inputTokenLimit":8192}]}`)
	}))
	defer server.Close()
	models, err := NewHTTPModelClient().Models(context.Background(), Provider{Protocol: ProtocolGemini, BaseURL: server.URL, EndpointScope: EndpointPrivate}, "key")
	if err != nil || len(models) != 1 || models[0].ModelID != "gemini-test" {
		t.Fatalf("models=%#v err=%v", models, err)
	}
}

func TestProviderHTTPErrorRetryPolicy(t *testing.T) {
	err := providerHTTPError(http.StatusUnauthorized, []byte("bad key")).(*ProviderError)
	if err.Retryable {
		t.Fatal("401 must not be retryable")
	}
	err = providerHTTPError(http.StatusTooManyRequests, nil).(*ProviderError)
	if !err.Retryable {
		t.Fatal("429 must be retryable before output")
	}
}

func TestProviderErrorDoesNotEchoAPIKey(t *testing.T) {
	const key = "sk-do-not-leak"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid key "+key, http.StatusUnauthorized)
	}))
	defer server.Close()
	_, err := NewHTTPModelClient().Models(context.Background(), Provider{Protocol: ProtocolOpenAICompatible, BaseURL: server.URL, EndpointScope: EndpointPrivate}, key)
	if err == nil || strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("unsafe provider error: %v", err)
	}
}

func TestNativeToolMessagesUseProviderSchemas(t *testing.T) {
	tests := []struct {
		name     string
		protocol ProviderProtocol
		want     []string
	}{
		{name: "openai", protocol: ProtocolOpenAICompatible, want: []string{`"tool_calls"`, `"tool_call_id":"call_1"`}},
		{name: "anthropic", protocol: ProtocolAnthropic, want: []string{`"type":"tool_use"`, `"type":"tool_result"`, `"tool_use_id":"call_1"`}},
		{name: "gemini", protocol: ProtocolGemini, want: []string{`"functionCall"`, `"functionResponse"`, `"name":"host_read"`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				for _, marker := range test.want {
					if !strings.Contains(string(body), marker) {
						t.Fatalf("payload missing %s: %s", marker, body)
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if test.protocol == ProtocolOpenAICompatible {
					fmt.Fprint(w, "data: [DONE]\n\n")
				} else if test.protocol == ProtocolAnthropic {
					fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
				} else {
					fmt.Fprint(w, "data: {\"candidates\":[]}\n\n")
				}
			}))
			defer server.Close()
			request := CompletionRequest{Model: "model", Messages: []ChatMessage{
				{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_1", Name: "host_read", Arguments: json.RawMessage(`{"x":1}`)}}},
				{Role: "tool", Name: "host_read", ToolCallID: "call_1", Content: `{"ok":true}`},
			}}
			provider := Provider{Protocol: test.protocol, BaseURL: server.URL, EndpointScope: EndpointPrivate}
			if err := NewHTTPModelClient().Stream(context.Background(), provider, "key", request, func(CompletionEvent) error { return nil }); err != nil {
				t.Fatal(err)
			}
		})
	}
}
