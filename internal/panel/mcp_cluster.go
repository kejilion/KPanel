package panel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/mcpaccess"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *Server) callMCPHostCapabilities(ctx context.Context, raw json.RawMessage) *mcp.CallToolResult {
	ctx, cancel, call, ok := mcpBoundContext(ctx)
	defer cancel()
	if !ok || !s.mcpCallActive(call) || call.principal.Policy == nil {
		return mcpToolError("mcp_authentication_required")
	}
	var input struct {
		HostID string `json:"hostId"`
	}
	if decodeStrictToolArguments(raw, &input) != nil || input.HostID == "" {
		return mcpToolError("invalid_arguments")
	}
	host, err := s.cluster.Host(ctx, input.HostID)
	if err != nil || !s.mcpCallAllowed(call, host) {
		return mcpToolError("host_not_authorized")
	}
	if err := s.auditMCP(call, "tool.host_capabilities", host.ID, "intent"); err != nil {
		return mcpToolError("audit_unavailable")
	}
	operations := []string{}
	supported, authorized, code := host.IsLocal, host.IsLocal, ""
	var remote *cluster.ManagedPolicy
	if !host.IsLocal {
		response, err := s.cluster.ManagedRemote(ctx, host.ID, cluster.ManagedRequest{Version: 1, Mode: "capabilities"})
		if err != nil {
			code = "remote_operations_unsupported_or_unreachable"
		} else {
			supported, authorized, remote = response.Supported, response.Authorized, response.Policy
		}
	}
	for name, op := range managedCatalog() {
		if op.Domain == "files" && !host.IsLocal && (remote == nil || len(intersectMCPFileRoots(remote.FileRoots, call.principal.Policy.FileRoots)) == 0) {
			continue
		}
		if op.allowed(call.principal.Policy) && (host.IsLocal || (supported && authorized && remote != nil && remote.OperationVersions[name] == op.signature() && (op.ReadOnly || remote.Write))) {
			operations = append(operations, name)
		}
	}
	slices.Sort(operations)
	current, err := s.cluster.Host(ctx, host.ID)
	if err != nil || !s.mcpCallAllowed(call, current) {
		return mcpToolError("host_not_authorized")
	}
	if err := s.auditMCP(call, "tool.host_capabilities", host.ID, "success"); err != nil {
		return mcpToolError("audit_unavailable")
	}
	return mcpManagedResult(map[string]any{"hostId": host.ID, "supported": supported, "authorized": authorized, "operations": operations, "error": code, "fileRoots": call.principal.Policy.FileRoots, "targetFileRoots": func() []string {
		if remote != nil {
			return remote.FileRoots
		}
		return nil
	}(), "observedAt": time.Now().UTC()})
}

func (s *Server) mcpCallActive(call mcpRequest) bool {
	if call.authorize != nil {
		return call.authorize()
	}
	return s.mcp.access.Active(call.principal) && (call.credentialActive == nil || call.credentialActive())
}

func (s *Server) mcpCallAllowed(call mcpRequest, host cluster.Host) bool {
	if call.authorize == nil {
		return s.mcpAllowed(call.principal, host)
	}
	if !host.IsLocal || !s.mcpCallActive(call) {
		return false
	}
	for _, grant := range call.principal.Hosts {
		if grant.ID == host.ID && grant.Identity == s.mcpHostIdentity(host) {
			return true
		}
	}
	return false
}

func managedRemoteError(code string) error {
	return &cluster.ManagedOperationError{Code: code}
}

func (s *Server) handleManagedClusterOperation(ctx context.Context, controllerID string, grant cluster.ManagedGrant, input cluster.ManagedRequest, active func() bool) (json.RawMessage, error) {
	if !active() {
		return nil, managedRemoteError("remote_operations_not_authorized")
	}
	host, err := s.cluster.Host(ctx, cluster.LocalHostID)
	if err != nil {
		return nil, managedRemoteError("host_unavailable")
	}
	// Regranting a controller creates a new namespace. Old approvals cannot be
	// resurrected even if the controller and client IDs are reused.
	digest := sha256.Sum256([]byte(controllerID + "\x00" + input.ClientID + "\x00" + grant.Revision))
	clientID := hex.EncodeToString(digest[:16])
	policy := &mcpaccess.Policy{Write: grant.Policy.Write, OperationVersions: map[string]string{}, FileRoots: intersectMCPFileRoots(grant.Policy.FileRoots, input.FileRoots)}
	for name, signature := range grant.Policy.OperationVersions {
		op, ok := managedCatalog()[name]
		if !ok || signature != op.signature() || (op.Domain == "files" && len(policy.FileRoots) == 0) {
			continue
		}
		policy.Operations = append(policy.Operations, name)
		policy.OperationVersions[name] = signature
		if !slices.Contains(policy.Domains, op.Domain) {
			policy.Domains = append(policy.Domains, op.Domain)
		}
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://cluster/managed-operation", nil)
	call := mcpRequest{principal: mcpaccess.Principal{Client: mcpaccess.Client{ID: clientID, Name: "cluster-controller", Policy: policy, CreatedAt: grant.CreatedAt, ExpiresAt: grant.ExpiresAt, Hosts: []mcpaccess.HostGrant{{ID: host.ID, Identity: s.mcpHostIdentity(host)}}}}, request: request, authorize: active, controllerID: controllerID}
	if input.Mode == "status" {
		o, err := s.mcp.operations.ByKey(clientID, input.OperationID)
		if err != nil {
			return nil, managedRemoteError("operation_not_found")
		}
		o, err = s.observeMCPOperation(ctx, call, o)
		if err != nil {
			return nil, managedRemoteError(err.Error())
		}
		return json.Marshal(mcpOperationView(o))
	}
	op, ok := managedCatalog()[input.Tool]
	if !ok || input.Signature != op.signature() {
		return nil, managedRemoteError("operation_version_changed")
	}
	prepared, err := op.prepare(input.Arguments, policy)
	if err != nil {
		return nil, managedRemoteError("operation_not_authorized_or_invalid")
	}
	if err := s.auditMCP(call, "cluster.tool."+op.Name, host.ID, "intent"); err != nil {
		return nil, managedRemoteError("audit_unavailable")
	}
	if input.Mode == "read" {
		if !op.ReadOnly {
			return nil, managedRemoteError("operation_requires_write_grant")
		}
		if err := s.mcpPreflight(ctx, op.Name, input.Arguments, policy); err != nil {
			return nil, managedRemoteError(err.Error())
		}
		response, err := s.mcpLocalRequest(ctx, op, prepared, newRequestID())
		if err != nil {
			return nil, managedRemoteError("agent_unavailable")
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, managedRemoteError(mcpAgentError(response))
		}
		limit := input.Limit
		if limit == 0 {
			limit = 20
		}
		body, projectionErr := mcpFileReadProjection(op.Name, response.Body, policy)
		if projectionErr != nil {
			return nil, managedRemoteError(projectionErr.Error())
		}
		output, err := mcpManagedOutput(body, input.Offset, limit)
		if err != nil {
			return nil, managedRemoteError("agent_response_invalid")
		}
		if !active() {
			return nil, managedRemoteError("remote_operations_not_authorized")
		}
		if err := s.auditMCP(call, "cluster.tool."+op.Name, host.ID, "success"); err != nil {
			return nil, managedRemoteError("audit_unavailable")
		}
		return json.Marshal(output)
	}
	if input.Mode != "execute" || op.ReadOnly || !policy.Write {
		return nil, managedRemoteError("operation_requires_write_grant")
	}
	// The target administrator explicitly delegated these operation IDs to the
	// authenticated controller. The controller separately enforces MCP approval.
	o, err := s.mcp.operations.Plan(clientID, input.OperationID, host.ID, s.mcpHostIdentity(host), op.Name, input.Arguments, true)
	if err != nil {
		return nil, managedRemoteError("operation_conflict_or_store_unavailable")
	}
	if o.State == "approved" {
		o, err = s.executeMCPOperation(ctx, call, o.ID, o.Digest)
		if err != nil {
			// A durable, unclaimed approval is safe to retry using the same ID.
			// Return its actual state even when local capacity/audit is unavailable.
			current, readErr := s.mcp.operations.ByKey(clientID, input.OperationID)
			if readErr != nil {
				return nil, managedRemoteError("operation_could_not_start")
			}
			return json.Marshal(mcpOperationView(current))
		}
	}
	return json.Marshal(mcpOperationView(o))
}

func (s *Server) mcpRemoteCapabilities(ctx context.Context, hostID string, op managedOperation) error {
	result, err := s.cluster.ManagedRemote(ctx, hostID, cluster.ManagedRequest{Version: 1, Mode: "capabilities"})
	if err != nil || !result.Supported {
		return errors.New("remote_operations_unsupported_or_unreachable")
	}
	if !result.Authorized || result.Policy == nil || result.Policy.OperationVersions[op.Name] != op.signature() || (!op.ReadOnly && !result.Policy.Write) {
		return errors.New("remote_operations_not_authorized")
	}
	return nil
}

func (s *Server) readManagedRemote(ctx context.Context, call mcpRequest, hostID string, op managedOperation, args json.RawMessage, offset, limit int) (map[string]any, error) {
	if len(args) > cluster.MaxManagedPayload-2048 {
		return nil, errors.New("remote_arguments_too_large")
	}
	response, err := s.cluster.ManagedRemote(ctx, hostID, cluster.ManagedRequest{Version: 1, Mode: "read", FileRoots: call.principal.Policy.FileRoots, ClientID: call.principal.ID, Tool: op.Name, Signature: op.signature(), Arguments: args, Offset: offset, Limit: limit})
	if err != nil {
		return nil, errors.New("remote_operation_unavailable")
	}
	if response.ErrorCode != "" {
		return nil, errors.New(response.ErrorCode)
	}
	var output map[string]any
	if !response.Authorized || json.Unmarshal(response.Data, &output) != nil || output == nil {
		return nil, errors.New("remote_response_invalid")
	}
	return output, nil
}

func (s *Server) finishManagedRemote(ctx context.Context, call mcpRequest, o mcpaccess.Operation) (mcpaccess.Operation, error) {
	op := managedCatalog()[o.Tool]
	response, err := s.cluster.ManagedRemote(ctx, o.HostID, cluster.ManagedRequest{Version: 1, Mode: "execute", FileRoots: call.principal.Policy.FileRoots, ClientID: call.principal.ID, OperationID: o.ID, Tool: o.Tool, Signature: op.signature(), Arguments: o.Arguments})
	if err != nil {
		return s.mcp.operations.Finish(o.ID, "unknown", nil, "remote_receipt_lost_check_operation_status")
	}
	if response.ErrorCode != "" {
		return s.mcp.operations.Finish(o.ID, "unknown", nil, response.ErrorCode)
	}
	state, result, err := mcpRemoteOperationResult(response)
	if err != nil {
		return s.mcp.operations.Finish(o.ID, "unknown", nil, "remote_receipt_invalid")
	}
	return s.mcp.operations.Finish(o.ID, state, result, "")
}

func mcpRemoteOperationResult(response cluster.ManagedResponse) (string, json.RawMessage, error) {
	var output struct {
		State string `json:"state"`
		ID    string `json:"operationId"`
	}
	if !response.Authorized || !response.Supported || json.Unmarshal(response.Data, &output) != nil || !mcpClientID(output.ID) {
		return "", nil, errors.New("remote_response_invalid")
	}
	state := output.State
	if state == "executing" {
		state = "submitted"
	}
	if state == "expired" || state == "rejected" {
		state = "failed"
	}
	if !slices.Contains([]string{"approved", "submitted", "succeeded", "failed", "unknown"}, state) {
		return "", nil, errors.New("remote_response_invalid")
	}
	result, _ := json.Marshal(map[string]any{"remoteOperation": json.RawMessage(response.Data), "observedAt": time.Now().UTC()})
	return state, result, nil
}

// The settings router has already required a Panel session and, for writes,
// same-origin + CSRF. Pairing itself remains separate from this explicit grant.
func (s *Server) handleMCPClusterGrants(w http.ResponseWriter, r *http.Request, actor string) {
	view := func() map[string]any {
		return map[string]any{"grants": s.cluster.ManagedGrants(), "controllers": s.cluster.ManagedControllers()}
	}
	if r.Method == http.MethodGet && r.URL.Path == mcpSettingsPath+"/cluster-grants" {
		s.writeJSON(w, 200, view())
		return
	}
	if r.URL.Path != mcpSettingsPath+"/cluster-grants" || (r.Method != http.MethodPost && r.Method != http.MethodDelete) {
		http.NotFound(w, r)
		return
	}
	if !s.mcpTransportAllowed(r) {
		s.writeProblem(w, r, 403, "mcp_https_required", "Use HTTPS or loopback", "")
		return
	}
	var input struct {
		ControllerID            string   `json:"controllerId"`
		ExpectedResourceVersion string   `json:"expectedResourceVersion"`
		Domains                 []string `json:"domains"`
		Write                   bool     `json:"write"`
		FileRoots               []string `json:"fileRoots"`
		ExpiresInDays           int      `json:"expiresInDays"`
	}
	if s.decodeJSON(w, r, &input) != nil {
		return
	}
	if !resourceVersionPattern.MatchString(input.ExpectedResourceVersion) {
		s.mcpSettingsError(w, r, mcpaccess.ErrInvalid)
		return
	}
	action := "mcp.cluster.grant"
	var policy *mcpaccess.Policy
	var err error
	if r.Method == http.MethodPost {
		if input.ExpiresInDays < 1 || input.ExpiresInDays > 90 {
			s.mcpSettingsError(w, r, mcpaccess.ErrInvalid)
			return
		}
		policy, err = managedPolicy(input.Domains, input.Write, false, input.FileRoots)
		if err != nil {
			s.mcpSettingsError(w, r, mcpaccess.ErrInvalid)
			return
		}
	} else {
		action = "mcp.cluster.revoke"
	}
	if err := s.audit(r, actor, action, "cluster_controller", input.ControllerID, "intent", nil); err != nil {
		s.writeProblem(w, r, 503, "audit_unavailable", "Audit unavailable", "")
		return
	}
	if r.Method == http.MethodPost {
		err = s.cluster.SetManagedGrant(input.ControllerID, cluster.ManagedPolicy{OperationVersions: policy.OperationVersions, Write: policy.Write, FileRoots: policy.FileRoots}, time.Duration(input.ExpiresInDays)*24*time.Hour, input.ExpectedResourceVersion)
	} else {
		err = s.cluster.RevokeManagedGrant(input.ControllerID, input.ExpectedResourceVersion)
	}
	if err != nil {
		if errors.Is(err, cluster.ErrConflict) {
			s.mcpSettingsError(w, r, mcpaccess.ErrConflict)
		} else {
			s.mcpSettingsError(w, r, mcpaccess.ErrInvalid)
		}
		return
	}
	_ = s.audit(r, actor, action, "cluster_controller", input.ControllerID, "success", nil)
	s.writeJSON(w, 200, view())
}
