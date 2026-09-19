package panel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/mcpaccess"
	"github.com/kejilion/kejilion-panel/internal/redact"
	"github.com/kejilion/kejilion-panel/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *Server) addManagedMCPTools(server *mcp.Server, policy *mcpaccess.Policy) {
	server.AddTool(&mcp.Tool{Name: "host_capabilities", Description: "Check this client's effective operations on one authorized host. Remote control requires an explicit grant on the target Panel. Light nodes and older versions may only support summaries.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"hostId": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}}, "required": []string{"hostId"}, "additionalProperties": false}, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true}}, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return s.callMCPHostCapabilities(ctx, request.Params.Arguments), nil
	})
	for _, op := range managedCatalog() {
		if !op.allowed(policy) {
			continue
		}
		encoded, _ := json.Marshal(op.Schema)
		var schema map[string]any
		_ = json.Unmarshal(encoded, &schema)
		properties := schema["properties"].(map[string]any)
		properties["hostId"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 128}
		required, _ := schema["required"].([]any)
		required = append(required, "hostId")
		description := op.Description
		if op.ReadOnly && op.Name != "host_file_list" {
			properties["pageOffset"] = map[string]any{"type": "integer", "minimum": 0, "maximum": 100000}
			properties["pageSize"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 50}
		} else if !op.ReadOnly {
			properties["requestKey"] = map[string]any{"type": "string", "minLength": 8, "maxLength": 128}
			required = append(required, "requestKey")
			description += " Creates a durable operation; use a new requestKey for each intended change and reuse it on retries. Approval may be required in Panel."
		}
		schema["required"] = required
		closed, destructive := false, !op.ReadOnly
		server.AddTool(&mcp.Tool{Name: op.Name, Description: description, InputSchema: schema, OutputSchema: map[string]any{"type": "object"}, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: op.ReadOnly, DestructiveHint: &destructive, OpenWorldHint: &closed}}, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return s.callManagedMCP(ctx, op, request.Params.Arguments), nil
		})
	}
	for _, name := range []string{"operation_status", "operation_execute", "operations_list"} {
		properties := map[string]any{}
		required := []string{}
		if name != "operations_list" {
			properties["operationId"] = map[string]any{"type": "string", "pattern": "^[a-f0-9]{32}$"}
			required = append(required, "operationId")
		}
		if name == "operation_execute" {
			properties["digest"] = map[string]any{"type": "string", "pattern": "^[a-f0-9]{64}$"}
			required = append(required, "digest")
		}
		readOnly := name != "operation_execute"
		if !readOnly && !policy.Write {
			continue
		}
		description := "Read this client's durable operation status. Pending operations require Panel approval; unknown means inspect the host before a new change."
		if name == "operation_execute" {
			description = "Execute a previously approved operation exactly once. Pass the operationId and digest returned by the original tool. The approval must already exist in Panel."
		}
		server.AddTool(&mcp.Tool{Name: name, Description: description, InputSchema: map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}, OutputSchema: map[string]any{"type": "object"}, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly}}, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return s.callMCPOperation(ctx, name, request.Params.Arguments), nil
		})
	}
}

func mcpBoundContext(ctx context.Context) (context.Context, context.CancelFunc, mcpRequest, bool) {
	call, ok := ctx.Value(mcpContextKey{}).(mcpRequest)
	if !ok || call.request == nil {
		return ctx, func() {}, call, false
	}
	bounded, cancel := context.WithTimeout(call.request.Context(), 10*time.Second)
	stop := context.AfterFunc(ctx, cancel)
	return bounded, func() { stop(); cancel() }, call, true
}

func (s *Server) auditMCP(call mcpRequest, action, host, result string) error {
	actorType, actorID := "mcp_client", call.principal.ID
	if call.controllerID != "" {
		actorType, actorID = "cluster_controller", call.controllerID
	}
	return s.store.AppendAudit(store.AuditEvent{ID: newRequestID(), OccurredAt: time.Now().UTC(), ActorType: actorType, ActorID: actorID, SourceIP: s.remoteIP(call.request), Action: "mcp." + action, TargetKind: "host", TargetID: host, Result: result, RequestID: requestID(call.request)}, store.MaxAuditEntries)
}

func (s *Server) callManagedMCP(ctx context.Context, op managedOperation, raw json.RawMessage) *mcp.CallToolResult {
	ctx, cancel, call, ok := mcpBoundContext(ctx)
	defer cancel()
	if !ok || !s.mcpCallActive(call) {
		return mcpToolError("mcp_authentication_required")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return mcpToolError("invalid_arguments")
	}
	var hostID, key string
	if json.Unmarshal(fields["hostId"], &hostID) != nil || hostID == "" {
		return mcpToolError("invalid_arguments")
	}
	delete(fields, "hostId")
	offset, limit := 0, 20
	if op.ReadOnly {
		for field, dest := range map[string]*int{"pageOffset": &offset, "pageSize": &limit} {
			if value, exists := fields[field]; exists {
				if op.Name == "host_file_list" {
					return mcpToolError("use_directory_offset_and_limit")
				}
				if string(value) == "null" || json.Unmarshal(value, dest) != nil {
					return mcpToolError("invalid_arguments")
				}
				delete(fields, field)
			}
		}
		if offset < 0 || offset > 100000 || limit < 1 || limit > 50 {
			return mcpToolError("invalid_arguments")
		}
	} else {
		if json.Unmarshal(fields["requestKey"], &key) != nil || len(key) < 8 || len(key) > 128 {
			return mcpToolError("invalid_arguments")
		}
		delete(fields, "requestKey")
	}
	arguments, _ := json.Marshal(fields)
	prepared, err := op.prepare(arguments, call.principal.Policy)
	if err != nil {
		return mcpToolError(err.Error())
	}
	host, err := s.cluster.Host(ctx, hostID)
	if err != nil || !s.mcpCallAllowed(call, host) {
		return mcpToolError("host_not_authorized")
	}
	if !host.IsLocal {
		if len(arguments) > 46<<10 {
			return mcpToolError("remote_arguments_too_large")
		}
		if !op.ReadOnly {
			if err := s.mcpRemoteCapabilities(ctx, host.ID, op); err != nil {
				return mcpToolError(err.Error())
			}
		}
	}
	if err := s.auditMCP(call, "tool."+op.Name, hostID, "intent"); err != nil {
		return mcpToolError("audit_unavailable")
	}
	if !op.ReadOnly {
		auto := call.principal.Policy.AutoApprove && op.automatic(arguments)
		operation, err := s.mcp.operations.Plan(call.principal.ID, key, hostID, s.mcpHostIdentity(host), op.Name, arguments, auto)
		if err != nil {
			return mcpToolError(err.Error())
		}
		if auto && operation.State == "approved" {
			operation, err = s.executeMCPOperation(ctx, call, operation.ID, operation.Digest)
		}
		if err != nil {
			return mcpToolError(err.Error())
		}
		return mcpManagedResult(mcpOperationView(operation))
	}
	var output map[string]any
	if host.IsLocal {
		if err := s.mcpPreflight(ctx, op.Name, arguments, call.principal.Policy); err != nil {
			return mcpToolError(err.Error())
		}
		response, callErr := s.mcpLocalRequest(ctx, op, prepared, newRequestID())
		if callErr != nil {
			return mcpToolError("agent_unavailable")
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return mcpToolError(mcpAgentError(response))
		}
		body, projectionErr := mcpFileReadProjection(op.Name, response.Body, call.principal.Policy)
		if projectionErr != nil {
			return mcpToolError(projectionErr.Error())
		}
		output, err = mcpManagedOutput(body, offset, limit)
	} else {
		output, err = s.readManagedRemote(ctx, call, host.ID, op, arguments, offset, limit)
	}
	if err != nil {
		return mcpToolError(err.Error())
	}
	current, err := s.cluster.Host(ctx, hostID)
	if err != nil || !s.mcpCallAllowed(call, current) {
		return mcpToolError("host_not_authorized")
	}
	if err := s.auditMCP(call, "tool."+op.Name, hostID, "success"); err != nil {
		return mcpToolError("audit_unavailable")
	}
	return mcpManagedResult(output)
}

func mcpAgentError(response AgentResponse) string {
	if response.StatusCode == 409 || response.StatusCode == 412 {
		return "resource_version_changed"
	}
	if response.StatusCode == 404 {
		return "resource_not_found"
	}
	if response.StatusCode == 422 || response.StatusCode == 400 {
		return "agent_validation_failed"
	}
	return "agent_operation_failed"
}

func mcpManagedResult(output map[string]any) *mcp.CallToolResult {
	b, err := json.Marshal(output)
	if err != nil || len(b) > 48<<10 {
		return mcpToolError("mcp_response_too_large_reduce_page_size")
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}, StructuredContent: output}
}

func mcpManagedOutput(body []byte, offset, limit int) (map[string]any, error) {
	if len(body) > 8<<20 {
		return nil, errors.New("agent_response_too_large")
	}
	var data any
	if json.Unmarshal(body, &data) != nil {
		return nil, errors.New("agent_response_invalid")
	}
	truncated := false
	var sanitize func(any, int) any
	sanitize = func(value any, depth int) any {
		if depth > 12 {
			truncated = true
			return nil
		}
		switch typed := value.(type) {
		case map[string]any:
			out := map[string]any{}
			for key, item := range typed {
				lower := strings.ToLower(key)
				sensitive := slices.Contains([]string{"env", "environment", "environmentvariables", "labels", "mounts", "config", "configuration", "raw", "databaseurl", "accessurl", "databases"}, lower)
				for _, part := range []string{"password", "passwd", "secret", "token", "authorization", "cookie", "credential", "privatekey", "apikey"} {
					sensitive = sensitive || strings.Contains(strings.ReplaceAll(lower, "_", ""), part)
				}
				if sensitive {
					out[key] = "[REDACTED]"
					continue
				}
				if key == "dataBase64" {
					out[key] = "[REDACTED]"
					continue
				}
				out[key] = sanitize(item, depth+1)
			}
			return out
		case []any:
			if len(typed) > 50 {
				typed = typed[:50]
				truncated = true
			}
			out := make([]any, len(typed))
			for i, item := range typed {
				out[i] = sanitize(item, depth+1)
			}
			return out
		case string:
			text := redact.Text(typed)
			if len(text) > 16384 {
				text = string([]rune(text)[:min(len([]rune(text)), 4096)])
				truncated = true
			}
			return text
		default:
			return value
		}
	}
	if object, ok := data.(map[string]any); ok {
		var err error
		truncated, err = mcpTerminalProjection(object)
		if err != nil {
			return nil, err
		}
		if items, ok := object["items"].([]any); ok {
			end := min(offset+limit, len(items))
			start := min(offset, len(items))
			object["items"] = items[start:end]
			object["offset"] = offset
			object["total"] = len(items)
			object["hasMore"] = end < len(items)
			if end < len(items) {
				object["nextOffset"] = end
			}
		}
	}
	output := map[string]any{"data": sanitize(data, 0), "observedAt": time.Now().UTC(), "redacted": true}
	output["truncated"] = truncated
	return output, nil
}

func mcpOperationView(o mcpaccess.Operation) map[string]any {
	view := map[string]any{"operationId": o.ID, "digest": o.Digest, "hostId": o.HostID, "tool": o.Tool, "state": o.State, "createdAt": o.CreatedAt, "expiresAt": o.ExpiresAt, "updatedAt": o.UpdatedAt}
	if (o.State == "pending" || o.State == "approved") && !time.Now().Before(o.ExpiresAt) {
		view["state"] = "expired"
	}
	if len(o.Result) > 0 {
		view["result"] = o.Result
	}
	if o.Error != "" {
		view["error"] = o.Error
	}
	if o.State == "pending" {
		view["nextAction"] = "approve_in_panel_settings_mcp"
	} else if o.State == "approved" {
		view["nextAction"] = "execute_same_operation_id_and_digest"
	}
	return view
}

func (s *Server) executeMCPOperation(ctx context.Context, call mcpRequest, id, digest string) (mcpaccess.Operation, error) {
	o, err := s.mcp.operations.Get(id)
	if err != nil || o.ClientID != call.principal.ID || !s.mcpCallActive(call) {
		return mcpaccess.Operation{}, errors.New("operation_not_authorized")
	}
	if o.Digest != digest {
		return mcpaccess.Operation{}, mcpaccess.ErrConflict
	}
	if o.State != "approved" {
		return o, nil
	}
	op, ok := managedCatalog()[o.Tool]
	if !ok || op.ReadOnly {
		return o, errors.New("unsupported_operation")
	}
	prepared, err := op.prepare(o.Arguments, call.principal.Policy)
	if err != nil {
		return o, err
	}
	host, err := s.cluster.Host(ctx, o.HostID)
	if err != nil || !s.mcpCallAllowed(call, host) || o.HostIdentity != s.mcpHostIdentity(host) {
		return o, errors.New("host_not_authorized")
	}
	if !host.IsLocal {
		if err := s.mcpRemoteCapabilities(ctx, host.ID, op); err != nil {
			return o, err
		}
	}
	if err := s.auditMCP(call, "operation.execute", o.HostID, "intent"); err != nil {
		return o, errors.New("audit_unavailable")
	}
	s.mcp.workerMu.Lock()
	if s.mcp.workerClosed || ctx.Err() != nil {
		s.mcp.workerMu.Unlock()
		return o, errors.New("operation_service_stopping")
	}
	select {
	case s.mcp.workerSlots <- struct{}{}:
	default:
		s.mcp.workerMu.Unlock()
		return o, errors.New("operation_workers_busy_retry_same_operation")
	}
	o, err = s.mcp.operations.Claim(id, call.principal.ID, digest, s.mcpHostIdentity(host))
	if err != nil {
		<-s.mcp.workerSlots
		s.mcp.workerMu.Unlock()
		return o, err
	}
	s.mcp.workers.Add(1)
	s.mcp.workerMu.Unlock()
	done := make(chan struct{})
	go func() {
		defer s.mcp.workers.Done()
		defer func() { <-s.mcp.workerSlots; close(done) }()
		// Admission and durable claim are complete. Client disconnects no longer
		// cancel an accepted job; shutdown and the worker deadline still do.
		workCtx, cancel := context.WithTimeout(s.mcp.workerCtx, 10*time.Minute)
		defer cancel()
		_, _ = s.finishMCPOperation(workCtx, call, o, prepared)
	}()
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-done:
		return s.mcp.operations.Get(o.ID)
	case <-timer.C:
		return o, nil
	case <-ctx.Done():
		return o, nil
	}
}

func (s *Server) finishMCPOperation(ctx context.Context, call mcpRequest, o mcpaccess.Operation, prepared managedRequest) (mcpaccess.Operation, error) {
	if !s.mcpCallActive(call) {
		return s.mcp.operations.Finish(o.ID, "failed", nil, "authorization_revoked_before_dispatch")
	}
	if o.HostID != "local" {
		finished, err := s.finishManagedRemote(ctx, call, o)
		if err == nil {
			err = s.auditMCP(call, "operation.execute", o.HostID, finished.State)
		}
		return finished, err
	}
	// Claim was persisted before submission. Any lost receipt is UNKNOWN.
	if err := s.mcpPreflight(ctx, o.Tool, o.Arguments, call.principal.Policy); err != nil {
		return s.mcp.operations.Finish(o.ID, "failed", nil, err.Error())
	}
	host, hostErr := s.cluster.Host(ctx, o.HostID)
	if hostErr != nil || !s.mcpCallActive(call) || !s.mcpCallAllowed(call, host) || o.HostIdentity != s.mcpHostIdentity(host) {
		return s.mcp.operations.Finish(o.ID, "failed", nil, "authorization_revoked_before_dispatch")
	}
	response, callErr := s.mcpLocalRequest(ctx, managedCatalog()[o.Tool], prepared, o.ID)
	state, code := "succeeded", ""
	var result json.RawMessage
	if callErr != nil {
		state, code = "unknown", "receipt_lost_check_host_before_retry"
	} else if response.StatusCode < 200 || response.StatusCode >= 300 {
		state, code = "failed", mcpAgentError(response)
	} else {
		if response.StatusCode == http.StatusAccepted {
			state = "submitted"
		}
		body := response.Body
		if response.StatusCode == http.StatusNoContent {
			body = []byte(`{"acknowledged":true}`)
		}
		output, projectionErr := mcpManagedOutput(body, 0, 20)
		if projectionErr != nil {
			state, code = "unknown", "receipt_invalid_check_host_before_retry"
		} else {
			if task := mcpReceiptTask(o.Tool, response); task != nil {
				output["task"] = task
				state = "submitted"
			} else if data, ok := output["data"].(map[string]any); ok && (data["status"] == "accepted" || response.StatusCode == http.StatusAccepted) {
				state, code = "unknown", "operation_accepted_verify_host_state"
			}
			if op := managedCatalog()[o.Tool]; op.Domain == "files" && mcpFileFailures(body) {
				state, code = "failed", "partial_failure_check_result_before_retry"
			}
			result, _ = json.Marshal(output)
			if len(result) > mcpaccess.MaxOperationResult {
				result = nil
				state, code = "unknown", "receipt_too_large_check_host_before_retry"
			}
		}
	}
	finished, err := s.mcp.operations.Finish(o.ID, state, result, code)
	if err != nil {
		return o, err
	}
	if err := s.auditMCP(call, "operation.execute", o.HostID, state); err != nil {
		return finished, errors.New("audit_unavailable_check_operation_status")
	}
	return finished, nil
}

func (s *Server) callMCPOperation(ctx context.Context, name string, raw json.RawMessage) *mcp.CallToolResult {
	ctx, cancel, call, ok := mcpBoundContext(ctx)
	defer cancel()
	if !ok || !s.mcp.access.Active(call.principal) || call.principal.Policy == nil {
		return mcpToolError("mcp_authentication_required")
	}
	var input struct {
		OperationID string `json:"operationId"`
		Digest      string `json:"digest"`
	}
	if decodeStrictToolArguments(raw, &input) != nil {
		return mcpToolError("invalid_arguments")
	}
	if name == "operations_list" {
		if input.OperationID != "" || input.Digest != "" {
			return mcpToolError("invalid_arguments")
		}
		items, err := s.mcp.operations.List(call.principal.ID)
		if err != nil {
			return mcpToolError(err.Error())
		}
		views := []map[string]any{}
		for _, o := range items {
			view := mcpOperationView(o)
			delete(view, "result")
			views = append(views, view)
		}
		return mcpManagedResult(map[string]any{"items": views})
	}
	o, err := s.mcp.operations.Get(input.OperationID)
	if err != nil || o.ClientID != call.principal.ID {
		return mcpToolError("operation_not_found")
	}
	if name == "operation_execute" {
		o, err = s.executeMCPOperation(ctx, call, o.ID, input.Digest)
		if err != nil {
			return mcpToolError(err.Error())
		}
	} else {
		o, err = s.observeMCPOperation(ctx, call, o)
		if err != nil {
			return mcpToolError(err.Error())
		}
	}
	if !s.mcp.access.Active(call.principal) {
		return mcpToolError("mcp_authentication_required")
	}
	return mcpManagedResult(mcpOperationView(o))
}

// Called only after handleMCPSettings has checked Session, Origin and CSRF.
func (s *Server) handleMCPOperations(w http.ResponseWriter, r *http.Request, actor string) {
	if r.Method == http.MethodGet && r.URL.Path == mcpSettingsPath+"/operations" {
		items, err := s.mcp.operations.List("")
		if err != nil {
			s.mcpSettingsError(w, r, err)
			return
		}
		views := []map[string]any{}
		for _, o := range items {
			view := mcpOperationView(o)
			view["clientId"] = o.ClientID
			view["arguments"] = safeArgumentSummary(o.Arguments)
			views = append(views, view)
		}
		s.writeJSON(w, 200, map[string]any{"items": views})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, mcpSettingsPath+"/operations/")
	if r.Method == http.MethodGet && mcpClientID(id) {
		o, err := s.mcp.operations.Get(id)
		if err != nil {
			s.mcpSettingsError(w, r, err)
			return
		}
		if principal, lookupErr := s.mcp.access.LookupClient(o.ClientID); lookupErr == nil {
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			o, err = s.observeMCPOperation(ctx, mcpRequest{principal: principal, request: r}, o)
			if err != nil {
				s.mcpSettingsError(w, r, err)
				return
			}
		}
		s.writeJSON(w, 200, mcpOperationView(o))
		return
	}
	if r.Method != http.MethodPost || !mcpClientID(id) {
		http.NotFound(w, r)
		return
	}
	var input struct {
		Digest  string `json:"digest"`
		Approve bool   `json:"approve"`
	}
	if s.decodeJSON(w, r, &input) != nil {
		return
	}
	prior, err := s.mcp.operations.Get(id)
	if err != nil {
		s.mcpSettingsError(w, r, err)
		return
	}
	principal, err := s.mcp.access.LookupClient(prior.ClientID)
	if err != nil {
		s.mcpSettingsError(w, r, mcpaccess.ErrConflict)
		return
	}
	if err := s.audit(r, actor, "mcp.operation.decide", "mcp_operation", id, "intent", nil); err != nil {
		s.writeProblem(w, r, 503, "audit_unavailable", "Audit unavailable", "")
		return
	}
	o, err := s.mcp.operations.Decide(id, input.Digest, actor, input.Approve)
	if err != nil {
		s.mcpSettingsError(w, r, err)
		return
	}
	_ = s.audit(r, actor, "mcp.operation.decide", "mcp_operation", id, o.State, nil)
	if input.Approve {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		o, err = s.executeMCPOperation(ctx, mcpRequest{principal: principal, request: r}, o.ID, o.Digest)
		if err != nil {
			s.mcpSettingsError(w, r, err)
			return
		}
	}
	s.writeJSON(w, 200, mcpOperationView(o))
}
