package panel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const systemResourceAgentTimeout = 55 * time.Second

func isSystemResourcePublicPath(path string) bool {
	switch path {
	case "/api/v1/system/hosts", "/api/v1/system/cron",
		"/api/v1/system/network-interfaces", "/api/v1/system/firewall",
		"/api/v1/system/port-usage", "/api/v1/system/traffic-shutdown", "/api/v1/system/accounts",
		"/api/v1/system/disk-partitions":
		return true
	default:
		return false
	}
}

func (s *Server) handleSystemResourceAction(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	var input contract.SystemResourceActionRequest
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if field, detail := contract.ValidateSystemResourceAction(&input); field != "" {
		s.writeValidationProblem(w, r, field, detail)
		return
	}
	body, err := json.Marshal(input)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	action := "system.resource." + input.Action
	change := systemResourceAuditChange(input)
	if err := s.audit(r, session.User.ID, action, "system-resource", input.Action, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	// The Agent finishes a privileged resource transaction after it acquires its
	// writer lock, even if the browser disconnects. Keep the Panel side alive for
	// the same bounded operation so the outcome audit reflects the final receipt.
	agentContext, cancelAgent := context.WithTimeout(context.WithoutCancel(r.Context()), systemResourceAgentTimeout)
	defer cancelAgent()
	response, err := s.hostOps.Do(
		agentContext, http.MethodPost, "/v1/system/resource-actions", "", requestID(r), body,
	)
	if err != nil {
		_ = s.audit(r, session.User.ID, action, "system-resource", input.Action, "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	result := "failure"
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		result = "success"
	}
	_ = s.audit(r, session.User.ID, action, "system-resource", input.Action, result, change)
	s.writeAgentResponse(w, r, response)
}

func systemResourceAuditChange(input contract.SystemResourceActionRequest) map[string]any {
	change := map[string]any{"action": input.Action}
	switch input.Action {
	case "hosts-add":
		payload := input.Address + "\x00" + strings.Join(input.Hostnames, "\x00") + "\x00" + input.Comment
		change["payloadHash"], change["payloadLength"] = auditValueMetadata(payload)
	case "hosts-delete", "cron-delete":
		change["line"] = input.Line
	case "cron-add":
		change["schedule"] = input.Expression
		change["commandHash"], change["commandLength"] = auditValueMetadata(input.Command)
	case "cron-update":
		change["line"] = input.Line
		change["schedule"] = input.Expression
		change["commandHash"], change["commandLength"] = auditValueMetadata(input.Command)
	case "network-interface-state":
		change["interfaceName"] = input.InterfaceName
		change["enabled"] = input.Enabled != nil && *input.Enabled
	case "firewall-open-port", "firewall-close-port":
		change["port"] = input.Port
	case "firewall-allow-ip", "firewall-block-ip", "firewall-remove-ip":
		change["addressHash"], change["addressLength"] = auditValueMetadata(input.Address)
	case "firewall-allow-country", "firewall-block-country", "firewall-remove-country":
		change["countryCode"] = input.CountryCode
	case "firewall-open-all", "firewall-close-all", "firewall-enable-ping",
		"firewall-disable-ping", "firewall-enable-ddos", "firewall-disable-ddos":
		change["requested"] = true
	}
	return change
}

func auditValueMetadata(value string) (string, int) {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:]), len(value)
}
