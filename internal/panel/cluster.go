package panel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/auth"
	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	lightEnrollEndpoint = "/api/v3/federation/light/enroll"
	lightReportEndpoint = "/api/v3/federation/light/report"
)

type clusterTelemetrySource struct {
	agent agentAPI
}

func (s clusterTelemetrySource) Telemetry(ctx context.Context) (contract.HostTelemetry, error) {
	response, err := s.agent.Get(ctx, "/v1/system/telemetry", "", newRequestID())
	if err != nil {
		return contract.HostTelemetry{}, err
	}
	if response.StatusCode != http.StatusOK {
		return contract.HostTelemetry{}, errors.New("Agent telemetry is unavailable")
	}
	decoder := json.NewDecoder(bytes.NewReader(response.Body))
	decoder.DisallowUnknownFields()
	var telemetry contract.HostTelemetry
	if err := decoder.Decode(&telemetry); err != nil {
		return contract.HostTelemetry{}, errors.New("Agent telemetry response is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return contract.HostTelemetry{}, errors.New("Agent telemetry response has multiple JSON values")
	}
	return telemetry, nil
}

func (s *Server) StartBackground(ctx context.Context) {
	s.cluster.Start(ctx)
}

func (s *Server) Close() error {
	s.closeTerminalSessions()
	s.closeFileShareStreams()
	var aiErr error
	if s.ai != nil {
		aiErr = s.ai.Close()
	}
	var clusterErr error
	if s.cluster != nil {
		clusterErr = s.cluster.Close()
	}
	return errors.Join(aiErr, clusterErr)
}

func (s *Server) handleCluster(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_cluster_request", "Invalid cluster request", "")
		return
	}
	switch {
	case r.URL.Path == "/api/v1/cluster/hosts" && r.Method == http.MethodGet:
		if _, _, ok := s.requireSession(w, r); !ok {
			return
		}
		s.writeJSON(w, http.StatusOK, s.cluster.Hosts(r.Context()))
	case r.URL.Path == "/api/v1/cluster/hosts" && r.Method == http.MethodPost:
		s.handleClusterHostAdd(w, r)
	case r.URL.Path == "/api/v1/cluster/pairing-codes/v2" && r.Method == http.MethodPost:
		s.handleClusterPairingCodeV2(w, r)
	case r.URL.Path == "/api/v1/cluster/light-enrollments" && r.Method == http.MethodPost:
		s.handleLightEnrollmentCreate(w, r)
	case r.URL.Path == "/api/v1/cluster/pairing-codes" && r.Method == http.MethodPost:
		s.handleClusterPairingCode(w, r)
	case r.URL.Path == "/api/v1/cluster/controllers" && r.Method == http.MethodGet:
		if _, _, ok := s.requireSession(w, r); !ok {
			return
		}
		items := s.cluster.Controllers()
		s.writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
	case strings.HasPrefix(r.URL.Path, "/api/v1/cluster/controllers/") &&
		r.Method == http.MethodDelete:
		s.handleClusterControllerDelete(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/cluster/hosts/"):
		s.handleClusterHost(w, r)
	default:
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
	}
}

func (s *Server) handleClusterHostAdd(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	var input cluster.AddHostInput
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	input.ControllerOrigin = s.clusterFileCallbackOrigin(r)
	change := map[string]any{
		"name": strings.TrimSpace(input.Name), "origin": strings.TrimSpace(input.Origin),
	}
	if err := s.audit(r, session.User.ID, "cluster.host.add", "cluster-host", "", "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	host, err := s.cluster.AddHost(r.Context(), input)
	if err != nil {
		_ = s.audit(r, session.User.ID, "cluster.host.add", "cluster-host", "", "failure", change)
		s.writeClusterError(w, r, err)
		return
	}
	_ = s.audit(r, session.User.ID, "cluster.host.add", "cluster-host", host.ID, "success", change)
	s.writeJSON(w, http.StatusCreated, host)
}

func (s *Server) handleClusterHost(w http.ResponseWriter, r *http.Request) {
	const prefix = "/api/v1/cluster/hosts/"
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	if rest == r.URL.Path || rest == "" {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	if strings.HasSuffix(rest, "/refresh") {
		id := strings.TrimSuffix(rest, "/refresh")
		if strings.Contains(id, "/") || r.Method != http.MethodPost {
			s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
			return
		}
		s.handleClusterHostRefresh(w, r, id)
		return
	}
	if strings.HasSuffix(rest, "/mutual-files") {
		id := strings.TrimSuffix(rest, "/mutual-files")
		if strings.Contains(id, "/") || r.Method != http.MethodPost {
			s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
			return
		}
		s.handleClusterHostMutualFiles(w, r, id)
		return
	}
	if strings.Contains(rest, "/") {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	switch r.Method {
	case http.MethodGet:
		if _, _, ok := s.requireSession(w, r); !ok {
			return
		}
		host, err := s.cluster.Host(r.Context(), rest)
		if err != nil {
			s.writeClusterError(w, r, err)
			return
		}
		s.writeJSON(w, http.StatusOK, host)
	case http.MethodPatch:
		s.handleClusterHostRename(w, r, rest)
	case http.MethodDelete:
		s.handleClusterHostDelete(w, r, rest)
	default:
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
	}
}

func (s *Server) handleClusterHostMutualFiles(w http.ResponseWriter, r *http.Request, id string) {
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		s.writeProblem(w, r, http.StatusBadRequest, "request_body_not_allowed", "Request body not allowed", "")
		return
	}
	if err := s.audit(r, session.User.ID, "cluster.host.mutual-files.enable", "cluster-host", id, "intent", nil); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	host, err := s.cluster.EnableMutualFileTransfer(r.Context(), id, s.clusterFileCallbackOrigin(r))
	if err != nil {
		_ = s.audit(r, session.User.ID, "cluster.host.mutual-files.enable", "cluster-host", id, "failure", nil)
		s.writeClusterError(w, r, err)
		return
	}
	_ = s.audit(r, session.User.ID, "cluster.host.mutual-files.enable", "cluster-host", id, "success", nil)
	s.writeJSON(w, http.StatusOK, host)
}

func (s *Server) handleClusterHostRename(w http.ResponseWriter, r *http.Request, id string) {
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	var input cluster.UpdateHostInput
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	change := map[string]any{
		"name":                    strings.TrimSpace(input.Name),
		"expectedResourceVersion": input.ExpectedResourceVersion,
	}
	if err := s.audit(r, session.User.ID, "cluster.host.rename", "cluster-host", id, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	host, err := s.cluster.RenameHost(id, input)
	if err != nil {
		_ = s.audit(r, session.User.ID, "cluster.host.rename", "cluster-host", id, "failure", change)
		s.writeClusterError(w, r, err)
		return
	}
	_ = s.audit(r, session.User.ID, "cluster.host.rename", "cluster-host", id, "success", change)
	s.writeJSON(w, http.StatusOK, host)
}

func (s *Server) handleClusterHostDelete(w http.ResponseWriter, r *http.Request, id string) {
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	var input cluster.DeleteHostInput
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	change := map[string]any{"expectedResourceVersion": input.ExpectedResourceVersion}
	if err := s.audit(r, session.User.ID, "cluster.host.delete", "cluster-host", id, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	result, err := s.cluster.DeleteHost(r.Context(), id, input)
	if err != nil {
		_ = s.audit(r, session.User.ID, "cluster.host.delete", "cluster-host", id, "failure", change)
		s.writeClusterError(w, r, err)
		return
	}
	change["remoteRevoked"] = result.RemoteRevoked
	change["credentialRemoved"] = result.CredentialRemoved
	_ = s.audit(r, session.User.ID, "cluster.host.delete", "cluster-host", id, "success", change)
	s.writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleClusterHostRefresh(w http.ResponseWriter, r *http.Request, id string) {
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	if err := s.audit(r, session.User.ID, "cluster.host.refresh", "cluster-host", id, "intent", nil); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	host, err := s.cluster.Refresh(r.Context(), id)
	if err != nil {
		_ = s.audit(r, session.User.ID, "cluster.host.refresh", "cluster-host", id, "failure", nil)
		s.writeClusterError(w, r, err)
		return
	}
	_ = s.audit(r, session.User.ID, "cluster.host.refresh", "cluster-host", id, "success", nil)
	s.writeJSON(w, http.StatusAccepted, host)
}

func (s *Server) handleClusterPairingCode(w http.ResponseWriter, r *http.Request) {
	s.handleClusterPairingCodeProtocol(w, r, false)
}

func (s *Server) handleClusterPairingCodeV2(w http.ResponseWriter, r *http.Request) {
	s.handleClusterPairingCodeProtocol(w, r, true)
}

func (s *Server) handleLightEnrollmentCreate(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	if r.ContentLength > 0 {
		var input struct{}
		if err := s.decodeJSON(w, r, &input); err != nil {
			return
		}
	}
	if err := s.audit(
		r, session.User.ID, "cluster.light-enrollment.create",
		"cluster-node", s.cluster.NodeID(), "intent", nil,
	); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	origin, ok := s.lightEnrollmentOrigin(r)
	if !ok {
		_ = s.audit(r, session.User.ID, "cluster.light-enrollment.create", "cluster-node", s.cluster.NodeID(), "failure", nil)
		s.writeClusterError(w, r, cluster.ErrLightHTTPSOrigin)
		return
	}
	enrollment, err := s.cluster.CreateLightEnrollmentForOrigin(origin)
	if err != nil {
		_ = s.audit(r, session.User.ID, "cluster.light-enrollment.create", "cluster-node", s.cluster.NodeID(), "failure", nil)
		s.writeClusterError(w, r, err)
		return
	}
	_ = s.audit(r, session.User.ID, "cluster.light-enrollment.create", "cluster-node", s.cluster.NodeID(), "success", map[string]any{
		"expiresAt": enrollment.ExpiresAt,
		"protocol":  cluster.LightNodeProtocol,
	})
	s.writeJSON(w, http.StatusCreated, enrollment)
}

func (s *Server) handleClusterPairingCodeProtocol(
	w http.ResponseWriter,
	r *http.Request,
	v2 bool,
) {
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	if r.ContentLength > 0 {
		var input struct{}
		if err := s.decodeJSON(w, r, &input); err != nil {
			return
		}
	}
	if err := s.audit(
		r, session.User.ID, "cluster.pairing-code.create",
		"cluster-node", s.cluster.NodeID(), "intent", nil,
	); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	var code cluster.PairingCode
	var err error
	if v2 {
		code, err = s.cluster.CreatePairingCodeV2()
	} else {
		code, err = s.cluster.CreatePairingCode()
	}
	if err != nil {
		_ = s.audit(r, session.User.ID, "cluster.pairing-code.create", "cluster-node", s.cluster.NodeID(), "failure", nil)
		s.writeClusterError(w, r, err)
		return
	}
	_ = s.audit(r, session.User.ID, "cluster.pairing-code.create", "cluster-node", s.cluster.NodeID(), "success", nil)
	s.writeJSON(w, http.StatusCreated, code)
}

func (s *Server) handleClusterControllerDelete(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/cluster/controllers/")
	if id == "" || strings.Contains(id, "/") {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	if err := s.audit(r, session.User.ID, "cluster.controller.revoke", "cluster-controller", id, "intent", nil); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	if err := s.cluster.DeleteController(id); err != nil {
		_ = s.audit(r, session.User.ID, "cluster.controller.revoke", "cluster-controller", id, "failure", nil)
		s.writeClusterError(w, r, err)
		return
	}
	_ = s.audit(r, session.User.ID, "cluster.controller.revoke", "cluster-controller", id, "success", nil)
	s.writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) handleFederationPair(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_federation_request", "Invalid federation request", "")
		return
	}
	var input cluster.PairRequest
	if err := decodeLimitedJSON(w, r, cluster.MaxPairBytes, &input); err != nil {
		return
	}
	response, err := s.cluster.AcceptPair(s.remoteIP(r), input)
	if err != nil {
		s.auditAuthFailure(r, "cluster.federation.pair")
		s.writeClusterError(w, r, err)
		return
	}
	if err := s.audit(
		r, "", "cluster.federation.pair",
		"cluster-controller", input.ControllerID, "intent", nil,
	); err != nil {
		_ = s.cluster.DeleteController(input.ControllerID)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	_ = s.audit(r, "", "cluster.federation.pair", "cluster-controller", input.ControllerID, "success", nil)
	s.writeJSON(w, http.StatusCreated, response)
}

func (s *Server) handleFederationSummary(w http.ResponseWriter, r *http.Request) {
	response, err := s.cluster.SignedSummary(r.Context(), r)
	if err != nil {
		s.writeClusterError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleFederationRevoke(w http.ResponseWriter, r *http.Request) {
	controllerID := strings.TrimSpace(r.Header.Get("X-KPanel-Controller-ID"))
	if err := s.cluster.SignedRevoke(r); err != nil {
		s.writeClusterError(w, r, err)
		return
	}
	_ = s.audit(r, "", "cluster.federation.revoke", "cluster-controller", controllerID, "success", nil)
	s.writeJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}

func (s *Server) handleFederationV2(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_federation_request", "Invalid federation request", "")
		return
	}
	var envelope cluster.FederationEnvelopeV2
	if err := decodeLimitedJSON(
		w,
		r,
		cluster.MaxFederationV2Bytes,
		&envelope,
	); err != nil {
		return
	}
	if r.URL.Path == "/api/v2/federation/files/open" ||
		r.URL.Path == "/api/v2/federation/files/open-linked" {
		s.handleFederationFileOpenV2(w, r, envelope)
		return
	}
	response, err := s.cluster.HandleFederationV2(
		r.Context(),
		s.remoteIP(r),
		r.URL.Path,
		r.Header.Get(cluster.FederationCapabilitiesHeader),
		envelope,
	)
	if err != nil {
		s.auditAuthFailure(r, "cluster.federation.v2")
		s.writeClusterError(w, r, err)
		return
	}
	action := ""
	status := http.StatusOK
	switch r.URL.Path {
	case "/api/v2/federation/pair":
		action = "cluster.federation.v2.pair"
		status = http.StatusCreated
	case "/api/v2/federation/commit":
		action = "cluster.federation.v2.commit"
	case "/api/v2/federation/revoke":
		action = "cluster.federation.v2.revoke"
	case "/api/v2/federation/files/link":
		action = "cluster.federation.v2.files.link"
	case "/api/v2/federation/terminal/open":
		action = "cluster.federation.v2.terminal.open"
	case "/api/v2/federation/terminal/close":
		action = "cluster.federation.v2.terminal.close"
	}
	if action != "" {
		_ = s.audit(
			r,
			"",
			action,
			"cluster-controller",
			envelope.ControllerID,
			"success",
			map[string]any{"protocol": cluster.FederationProtocolV2},
		)
	}
	s.writeJSON(w, status, response)
}

func (s *Server) handleFederationFileOpenV2(
	w http.ResponseWriter,
	r *http.Request,
	envelope cluster.FederationEnvelopeV2,
) {
	linked := r.URL.Path == "/api/v2/federation/files/open-linked"
	var input cluster.FederationFileOpenRequest
	var authorization *cluster.FederationFileAuthorization
	var err error
	if linked {
		input, authorization, err = s.cluster.AuthorizeLinkedFederationFileV2(s.remoteIP(r), envelope)
	} else {
		input, authorization, err = s.cluster.AuthorizeFederationFileV2(s.remoteIP(r), envelope)
	}
	if err != nil {
		action := "cluster.federation.v2.files.open"
		if linked {
			action = "cluster.federation.v2.files.open-linked"
		}
		s.auditAuthFailure(r, action)
		s.writeClusterError(w, r, err)
		return
	}
	defer authorization.Close()
	streamer, ok := s.agent.(agentStreamAPI)
	if !ok {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_stream_unavailable", "Agent 文件流不可用", "")
		return
	}
	query := url.Values{
		"path":            []string{input.Path},
		"resourceVersion": []string{input.ResourceVersion},
	}
	response, err := streamer.OpenStream(
		r.Context(), http.MethodGet, "/v1/files/transfer/export", query.Encode(),
		requestID(r), http.NoBody, nil, 0,
	)
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		s.writeAgentResponse(w, r, AgentResponse{
			StatusCode:  response.StatusCode,
			ContentType: response.Header.Get("Content-Type"),
			Body:        mustReadLimited(response.Body, 1<<20),
		})
		return
	}
	rawMetadata, err := base64.RawURLEncoding.DecodeString(response.Header.Get("X-KPanel-File-Metadata"))
	if err != nil || len(rawMetadata) == 0 || len(rawMetadata) > cluster.MaxSummaryBytes {
		s.writeProblem(w, r, http.StatusBadGateway, "agent_response_invalid", "Agent 文件元数据无效", "")
		return
	}
	var metadata contract.FileTransferMetadata
	decoder := json.NewDecoder(bytes.NewReader(rawMetadata))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&metadata); err != nil {
		s.writeProblem(w, r, http.StatusBadGateway, "agent_response_invalid", "Agent 文件元数据无效", "")
		return
	}
	sealed, cipher, err := authorization.SealMetadata(metadata)
	if err != nil {
		s.writeClusterError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/x-kpanel-noise-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if err := cluster.WriteFederationFileHeader(w, sealed); err != nil {
		return
	}
	writer := cluster.NewFederationFileWriter(w, cipher)
	_, copyErr := io.CopyBuffer(writer, response.Body, make([]byte, 60<<10))
	if copyErr == nil && response.Trailer.Get("X-KPanel-Transfer-Result") != "ok" {
		copyErr = errors.New("Agent transfer failed")
	}
	_ = writer.Finish(copyErr)
	result := "success"
	if copyErr != nil {
		result = "failure"
	}
	action := "cluster.federation.v2.files.open"
	if linked {
		action = "cluster.federation.v2.files.open-linked"
	}
	_ = s.audit(r, "", action, "cluster-controller", envelope.ControllerID, result, map[string]any{
		"kind": metadata.Kind, "bytes": metadata.SizeBytes,
	})
}

func mustReadLimited(input io.Reader, limit int64) []byte {
	content, _ := io.ReadAll(io.LimitReader(input, limit))
	return content
}

func isLightNodeRequest(r *http.Request) bool {
	return r.Method == http.MethodPost &&
		(r.URL.Path == lightEnrollEndpoint || r.URL.Path == lightReportEndpoint)
}

func (s *Server) handleLightNodeFederation(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.Path == "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_federation_request", "Invalid federation request", "")
		return
	}
	switch r.URL.Path {
	case lightEnrollEndpoint:
		var input cluster.LightEnrollRequest
		if err := decodeLimitedJSON(w, r, cluster.MaxPairBytes, &input); err != nil {
			return
		}
		origin, ok := s.requestHTTPSOrigin(r)
		if !ok {
			s.auditAuthFailure(r, "cluster.light-node.enroll")
			s.writeClusterError(w, r, cluster.ErrLightHTTPSOrigin)
			return
		}
		response, err := s.cluster.EnrollLightNodeAtOrigin(s.remoteIP(r), origin, input)
		if err != nil {
			s.auditAuthFailure(r, "cluster.light-node.enroll")
			s.writeClusterError(w, r, err)
			return
		}
		_ = s.audit(r, "", "cluster.light-node.enroll", "cluster-host", response.NodeID, "success", map[string]any{
			"protocol": cluster.LightNodeProtocol,
		})
		s.writeJSON(w, http.StatusCreated, response)
	case lightReportEndpoint:
		rawBody, err := readLimitedJSONBody(w, r, cluster.MaxSummaryBytes)
		if err != nil {
			return
		}
		var input cluster.LightReportRequest
		decoder := json.NewDecoder(bytes.NewReader(rawBody))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeProblemJSON(w, contract.Problem{Type: "about:blank", Title: "Invalid JSON", Status: http.StatusBadRequest, Code: "invalid_json"})
			return
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			writeProblemJSON(w, contract.Problem{Type: "about:blank", Title: "Invalid JSON", Status: http.StatusBadRequest, Code: "invalid_json"})
			return
		}
		response, err := s.cluster.AcceptLightReport(cluster.LightReportAuth{
			Source:    s.remoteIP(r),
			NodeID:    strings.TrimSpace(r.Header.Get("X-KPanel-Light-Node-ID")),
			Timestamp: strings.TrimSpace(r.Header.Get("X-KPanel-Timestamp")),
			RequestID: strings.TrimSpace(r.Header.Get("X-KPanel-Request-ID")),
			Signature: strings.TrimSpace(r.Header.Get("X-KPanel-Signature")),
		}, rawBody, input)
		if err != nil {
			s.auditAuthFailure(r, "cluster.light-node.report")
			s.writeClusterError(w, r, err)
			return
		}
		s.writeJSON(w, http.StatusOK, response)
	default:
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
	}
}

func readLimitedJSONBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, error) {
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		writeProblemJSON(w, contract.Problem{Type: "about:blank", Title: "JSON request body required", Status: http.StatusUnsupportedMediaType, Code: "json_required"})
		return nil, errors.New("invalid content type")
	}
	if r.ContentLength > limit {
		writeProblemJSON(w, contract.Problem{Type: "about:blank", Title: "Request body too large", Status: http.StatusRequestEntityTooLarge, Code: "request_too_large"})
		return nil, errors.New("request body too large")
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	content, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeProblemJSON(w, contract.Problem{Type: "about:blank", Title: "Request body too large", Status: http.StatusRequestEntityTooLarge, Code: "request_too_large"})
		} else {
			writeProblemJSON(w, contract.Problem{Type: "about:blank", Title: "Invalid JSON", Status: http.StatusBadRequest, Code: "invalid_json"})
		}
		return nil, err
	}
	if len(content) == 0 {
		writeProblemJSON(w, contract.Problem{Type: "about:blank", Title: "Invalid JSON", Status: http.StatusBadRequest, Code: "invalid_json"})
		return nil, errors.New("request body is empty")
	}
	return content, nil
}

func (s *Server) requireClusterMutation(w http.ResponseWriter, r *http.Request) (auth.Session, bool) {
	if !s.checkOrigin(w, r) {
		return auth.Session{}, false
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return auth.Session{}, false
	}
	return session, true
}

// clusterFileCallbackOrigin is called only after requireClusterMutation has
// authenticated the session, Origin and CSRF token. It never trusts a
// browser-supplied callback value from JSON.
func (s *Server) clusterFileCallbackOrigin(r *http.Request) string {
	candidates := make([]string, 0, 3)
	if origin, ok := s.requestHTTPSOrigin(r); ok {
		candidates = append(candidates, origin)
	}
	if origin, ok := directIPOrigin(r); ok {
		candidates = append(candidates, origin)
	}
	if configured := strings.TrimRight(strings.TrimSpace(s.config.PublicURL), "/"); configured != "" {
		candidates = append(candidates, configured)
	}
	for _, candidate := range candidates {
		normalized, err := cluster.NormalizeV2Origin(candidate)
		if err == nil && normalized == candidate {
			return normalized
		}
	}
	return ""
}

func (s *Server) writeClusterError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	code := "cluster_operation_failed"
	title := "Cluster operation failed"
	switch {
	case errors.Is(err, cluster.ErrNotFound):
		status, code, title = http.StatusNotFound, "cluster_not_found", "Cluster resource not found"
	case errors.Is(err, cluster.ErrConflict):
		status, code, title = http.StatusConflict, "cluster_resource_changed", "Cluster resource changed"
	case errors.Is(err, cluster.ErrDuplicate):
		status, code, title = http.StatusConflict, "cluster_duplicate", "Cluster host already exists"
	case errors.Is(err, cluster.ErrHostLimit):
		status, code, title = http.StatusConflict, "cluster_host_limit", "Cluster host limit reached"
	case errors.Is(err, cluster.ErrLocalHost):
		status, code, title = http.StatusConflict, "cluster_local_host", "Local cluster host cannot be modified"
	case errors.Is(err, cluster.ErrInvalidOrigin):
		status, code, title = http.StatusUnprocessableEntity, "cluster_origin_invalid", "Cluster origin is invalid"
	case errors.Is(err, cluster.ErrLightHTTPSOrigin):
		status, code, title = http.StatusUnprocessableEntity, "cluster_light_https_required", "Light node HTTPS origin is required"
	case errors.Is(err, cluster.ErrPrivateOrigin):
		status, code, title = http.StatusUnprocessableEntity, "cluster_origin_blocked", "Cluster origin is blocked"
	case errors.Is(err, cluster.ErrPairingCode):
		status, code, title = http.StatusUnauthorized, "cluster_pairing_failed", "Cluster pairing failed"
	case errors.Is(err, cluster.ErrAuthentication):
		status, code, title = http.StatusUnauthorized, "federation_authentication_failed", "Federation authentication failed"
	case errors.Is(err, cluster.ErrReplay):
		status, code, title = http.StatusConflict, "federation_replay_rejected", "Federation request rejected"
	case errors.Is(err, cluster.ErrRateLimited):
		status, code, title = http.StatusTooManyRequests, "federation_rate_limited", "Federation request rate limited"
	case errors.Is(err, cluster.ErrMutualFilesUnsupported):
		status, code, title = http.StatusUpgradeRequired, "cluster_mutual_files_unsupported", "Mutual file transfer unsupported"
	case errors.Is(err, cluster.ErrProtocolMismatch):
		status, code, title = http.StatusUpgradeRequired, "federation_incompatible", "Federation protocol incompatible"
	case errors.Is(err, cluster.ErrIdentityMismatch):
		status, code, title = http.StatusConflict, "federation_identity_changed", "Federation identity changed"
	default:
		var remote *cluster.RemoteError
		if errors.As(err, &remote) {
			status = http.StatusBadGateway
			code = "cluster_remote_" + remote.Code
			title = "Remote KPanel request failed"
			if remote.Code == "authentication_failed" {
				status = http.StatusUnauthorized
			}
		}
	}
	if status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "60")
	}
	s.writeProblem(w, r, status, code, title, "")
}

func decodeLimitedJSON(w http.ResponseWriter, r *http.Request, limit int64, target any) error {
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		writeProblemJSON(w, contract.Problem{
			Type: "about:blank", Title: "JSON request body required",
			Status: http.StatusUnsupportedMediaType, Code: "json_required",
		})
		return errors.New("invalid content type")
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		status, code, title := http.StatusBadRequest, "invalid_json", "Invalid JSON"
		if errors.As(err, &tooLarge) {
			status, code, title = http.StatusRequestEntityTooLarge, "request_too_large", "Request body too large"
		}
		writeProblemJSON(w, contract.Problem{
			Type: "about:blank", Title: title, Status: status, Code: code,
		})
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeProblemJSON(w, contract.Problem{
			Type: "about:blank", Title: "Invalid JSON",
			Status: http.StatusBadRequest, Code: "invalid_json",
		})
		return errors.New("multiple JSON values")
	}
	return nil
}
