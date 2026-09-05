package panel

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/ai"
	"github.com/kejilion/kejilion-panel/internal/auth"
)

type aiSnapshotRecorder struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (w *aiSnapshotRecorder) Flush() {
	w.ResponseRecorder.Flush()
	w.cancel()
}

func TestAIHistoryAndStreamAttachmentMetadata(t *testing.T) {
	server, _ := newTestServer(t)
	if err := server.EnableAI(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	session, err := server.ai.Store.CreateSession(ctx, ai.Session{UserID: "admin", ProviderID: "provider", ModelID: "model"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := server.ai.Store.CreateRun(ctx, ai.Run{SessionID: session.ID, UserID: "admin", ProviderID: "provider", ModelID: "model"})
	if err != nil {
		t.Fatal(err)
	}
	message, err := server.ai.Store.AddMessage(ctx, ai.Message{SessionID: session.ID, RunID: run.ID, Role: ai.RoleUser, Content: "read this",
		Attachments: []ai.Attachment{{Name: "note.txt", MimeType: "text/plain", Kind: "text", Data: []byte("hello")}}})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.aiMessages(response, httptest.NewRequest(http.MethodGet, "/", nil), "admin", session.ID)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"size":5`) || strings.Contains(response.Body.String(), "aGVsbG8=") {
		t.Fatalf("history=%d %s", response.Code, response.Body.String())
	}
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := &aiSnapshotRecorder{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
	server.aiRunEvents(stream, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(streamCtx), auth.Session{User: auth.PublicUser{ID: "admin"}}, run.ID)
	if stream.Code != http.StatusOK || !strings.Contains(stream.Body.String(), "event: run.snapshot") || !strings.Contains(stream.Body.String(), `"size":5`) || strings.Contains(stream.Body.String(), "aGVsbG8=") {
		t.Fatalf("stream=%d %s", stream.Code, stream.Body.String())
	}
	db, err := sql.Open("sqlite", filepath.Join(server.config.DataDir, "ai.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "UPDATE messages SET attachments_json=? WHERE id=?", []byte("{"), message.ID); err != nil {
		t.Fatal(err)
	}
	for _, streamRequest := range []bool{false, true} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		if streamRequest {
			server.aiRunEvents(response, request, auth.Session{User: auth.PublicUser{ID: "admin"}}, run.ID)
		} else {
			server.aiMessages(response, request, "admin", session.ID)
		}
		if response.Code != http.StatusUnprocessableEntity || strings.Contains(response.Body.String(), "run.snapshot") || !strings.Contains(response.Body.String(), "ai_request_failed") {
			t.Fatalf("corrupt stream=%v: %d %s", streamRequest, response.Code, response.Body.String())
		}
	}
}

func TestAIMessageAcceptsMultipartAttachmentsWithoutBase64JSON(t *testing.T) {
	server, _ := newTestServer(t)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("content", "分析图片"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("attachments", "screen.png")
	if err != nil {
		t.Fatal(err)
	}
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	if _, err := part.Write(png); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/ai/sessions/test/messages", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	content, attachments, err := server.decodeAIMessage(response, request)
	if err != nil || response.Code != http.StatusOK {
		t.Fatalf("multipart decode status=%d body=%s err=%v", response.Code, response.Body.String(), err)
	}
	if content != "分析图片" || len(attachments) != 1 || attachments[0].Name != "screen.png" || !bytes.Equal(attachments[0].Data, png) {
		t.Fatalf("content=%q attachments=%#v", content, attachments)
	}
}

func TestAIProviderModelSessionCRUD(t *testing.T) {
	server, tokenPath := newTestServer(t)
	if err := server.EnableAI(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value}
	create := authenticatedRequest(server, http.MethodPost, "/api/v1/ai/providers", []byte(`{"name":"Mock","protocol":"openai_compatible","baseUrl":"http://127.0.0.1:11434/v1","endpointScope":"private","privateConfirmed":true,"apiKey":"secret-1234","enabled":true}`), sessionCookie, csrfCookie, headers)
	if create.Code != http.StatusCreated {
		t.Fatalf("provider create=%d %s", create.Code, create.Body.String())
	}
	var provider ai.Provider
	if err := json.Unmarshal(create.Body.Bytes(), &provider); err != nil {
		t.Fatal(err)
	}
	if provider.ID == "" || !provider.APIKeySet || provider.APIKeyHint != "1234" {
		t.Fatalf("provider=%#v", provider)
	}
	if strings.Contains(create.Body.String(), "secret-1234") {
		t.Fatal("response exposed API key")
	}
	modelBody := []byte(`{"modelId":"mock-model","displayName":"Mock Model","contextWindow":8192,"toolCalling":true,"enabled":true}`)
	modelsResponse := authenticatedRequest(server, http.MethodPost, "/api/v1/ai/providers/"+provider.ID+"/models", modelBody, sessionCookie, csrfCookie, headers)
	if modelsResponse.Code != http.StatusCreated {
		t.Fatalf("model add=%d %s", modelsResponse.Code, modelsResponse.Body.String())
	}
	var models []ai.Model
	if err := json.Unmarshal(modelsResponse.Body.Bytes(), &models); err != nil || len(models) != 1 {
		t.Fatalf("models=%#v err=%v", models, err)
	}
	if !models[0].Vision {
		t.Fatalf("manually added model did not default to vision: %#v", models[0])
	}
	sessionResponse := authenticatedRequest(server, http.MethodPost, "/api/v1/ai/sessions", []byte(`{"providerId":"`+provider.ID+`","modelId":"`+models[0].ID+`"}`), sessionCookie, csrfCookie, headers)
	if sessionResponse.Code != http.StatusCreated {
		t.Fatalf("session create=%d %s", sessionResponse.Code, sessionResponse.Body.String())
	}
	var session ai.Session
	if err := json.Unmarshal(sessionResponse.Body.Bytes(), &session); err != nil || session.ApprovalMode != ai.ApprovalManual {
		t.Fatalf("session approval default=%#v err=%v", session, err)
	}
	modeResponse := authenticatedRequest(server, http.MethodPatch, "/api/v1/ai/sessions/"+session.ID, []byte(`{"approvalMode":"auto"}`), sessionCookie, csrfCookie, headers)
	if modeResponse.Code != http.StatusOK || json.Unmarshal(modeResponse.Body.Bytes(), &session) != nil || session.ApprovalMode != ai.ApprovalAuto {
		t.Fatalf("session approval update=%d %s", modeResponse.Code, modeResponse.Body.String())
	}
	list := authenticatedRequest(server, http.MethodGet, "/api/v1/ai/sessions", nil, sessionCookie, csrfCookie, nil)
	if list.Code != http.StatusOK {
		t.Fatalf("session list=%d %s", list.Code, list.Body.String())
	}
	run, err := server.ai.Store.CreateRun(context.Background(), ai.Run{SessionID: session.ID, UserID: session.UserID, ProviderID: provider.ID, ProviderName: provider.Name, ModelID: models[0].ID, ModelName: models[0].DisplayName})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.ai.Store.SaveToolCall(context.Background(), ai.ToolCall{ID: "persisted-call", RunID: run.ID, SessionID: session.ID, Name: "host_system_summary", Arguments: json.RawMessage(`{}`), Status: ai.ToolCompleted}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.ai.Store.AddMessage(context.Background(), ai.Message{SessionID: session.ID, RunID: run.ID, Role: ai.RoleAssistant, Content: "healthy"}); err != nil {
		t.Fatal(err)
	}
	messagesResponse := authenticatedRequest(server, http.MethodGet, "/api/v1/ai/sessions/"+session.ID+"/messages", nil, sessionCookie, csrfCookie, nil)
	var messagePage struct {
		Items     []ai.Message  `json:"items"`
		ToolCalls []ai.ToolCall `json:"toolCalls"`
	}
	if messagesResponse.Code != http.StatusOK || json.Unmarshal(messagesResponse.Body.Bytes(), &messagePage) != nil || len(messagePage.Items) != 1 || len(messagePage.ToolCalls) != 1 || messagePage.ToolCalls[0].ID != "persisted-call" {
		t.Fatalf("message process history=%d %s", messagesResponse.Code, messagesResponse.Body.String())
	}
	deleteResponse := authenticatedRequest(server, http.MethodDelete, "/api/v1/ai/sessions/"+session.ID, nil, sessionCookie, csrfCookie, headers)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("active session delete=%d %s", deleteResponse.Code, deleteResponse.Body.String())
	}
	if _, err := server.ai.Store.Session(context.Background(), session.UserID, session.ID); !errors.Is(err, ai.ErrNotFound) {
		t.Fatalf("deleted active session lookup error=%v", err)
	}
}

func TestAIToolMapsAgentVersionConflictForReplanning(t *testing.T) {
	server, _ := newTestServer(t)
	server.agent = &stubAgent{response: AgentResponse{StatusCode: http.StatusConflict, ContentType: "application/json", Body: []byte(`{"code":"resource_version_conflict"}`)}}
	tools := &panelAITools{server: server}
	execution := ai.ToolExecutionContext{UserID: "admin", SessionID: "ses", RunID: "run", ToolCallID: "call"}
	_, err := tools.Execute(context.Background(), execution, "host_system_summary", json.RawMessage(`{}`))
	if !errors.Is(err, ai.ErrToolConflict) {
		t.Fatalf("Agent conflict was not typed for replanning: %v", err)
	}
}

func TestAIToolMapsSafeAgentBusinessRejectionForReplanning(t *testing.T) {
	server, _ := newTestServer(t)
	server.agent = &stubAgent{response: AgentResponse{StatusCode: http.StatusUnprocessableEntity, ContentType: "application/json", Body: []byte(`{"code":"file_symlink_rejected","detail":"sensitive server detail","requestId":"req-safe"}`)}}
	tools := &panelAITools{server: server}
	execution := ai.ToolExecutionContext{UserID: "admin", SessionID: "ses", RunID: "run", ToolCallID: "call"}
	_, err := tools.Execute(context.Background(), execution, "host_system_summary", json.RawMessage(`{}`))
	if !errors.Is(err, ai.ErrToolRejected) {
		t.Fatalf("Agent business rejection was not typed for replanning: %v", err)
	}
	var rejection *ai.ToolRejectedError
	if !errors.As(err, &rejection) || rejection.StatusCode != http.StatusUnprocessableEntity || rejection.Code != "file_symlink_rejected" || rejection.RequestID != "req-safe" {
		t.Fatalf("typed rejection=%#v err=%v", rejection, err)
	}
	if strings.Contains(err.Error(), "sensitive server detail") {
		t.Fatalf("Agent detail leaked through rejection: %v", err)
	}

	server.agent = &stubAgent{response: AgentResponse{StatusCode: http.StatusUnauthorized, ContentType: "application/json", Body: []byte(`{"code":"unauthorized"}`)}}
	_, err = tools.Execute(context.Background(), execution, "host_system_summary", json.RawMessage(`{}`))
	if errors.Is(err, ai.ErrToolRejected) {
		t.Fatalf("Agent authentication failure was treated as model-recoverable: %v", err)
	}
}

func TestAIToolUsesFixedAgentPathAndAudits(t *testing.T) {
	server, _ := newTestServer(t)
	agent := &stubAgent{response: AgentResponse{StatusCode: 200, ContentType: "application/json", Body: []byte(`{"ok":true}`)}}
	server.agent = agent
	tools := &panelAITools{server: server}
	execution := ai.ToolExecutionContext{UserID: "admin", SessionID: "ses-1", RunID: "run-1", ToolCallID: "call-1"}
	result, err := tools.Execute(context.Background(), execution, "host_system_summary", json.RawMessage(`{"reason":"inspect resources"}`))
	if err != nil || result != "{\"ok\":true}" {
		t.Fatalf("result=%q err=%v", result, err)
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].path != "/v1/system/summary" || calls[0].method != http.MethodGet {
		t.Fatalf("calls=%#v", calls)
	}
	if _, err := tools.Execute(context.Background(), execution, "host_system_summary", json.RawMessage(`{"unknown":true}`)); !errors.Is(err, ai.ErrToolArguments) || len(agent.snapshotCalls()) != 1 {
		t.Fatalf("unknown read arguments were not rejected before Agent: %v", err)
	}
	audits, _ := server.store.ListAudit(10, "")
	if len(audits) == 0 || audits[0].Change["sessionId"] != "ses-1" || audits[0].Change["runId"] != "run-1" || audits[0].Change["toolCallId"] != "call-1" {
		t.Fatalf("AI audit correlation missing: %#v", audits)
	}
	_, err = tools.Execute(context.Background(), execution, "host_docker_container_action", json.RawMessage(`{"containerId":"bad","action":"remove","resourceVersion":"bad"}`))
	if err == nil || len(agent.snapshotCalls()) != 1 {
		t.Fatal("invalid write reached Agent")
	}
}

func TestAIToolReasonMetadataIsStrippedBeforeNginxReloadAndAudit(t *testing.T) {
	server, _ := newTestServer(t)
	agent := &stubAgent{response: AgentResponse{StatusCode: 200, ContentType: "application/json", Body: []byte(`{"ok":true}`)}}
	server.agent = agent
	tools := &panelAITools{server: server}
	execution := ai.ToolExecutionContext{UserID: "admin", SessionID: "ses", RunID: "run", ToolCallID: "call"}
	if _, err := tools.Execute(context.Background(), execution, "host_nginx_reload", json.RawMessage(`{"reason":"configuration validated"}`)); err != nil {
		t.Fatal(err)
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].path != "/v1/nginx/reload" || calls[0].method != http.MethodPost || string(calls[0].body) != `{}` {
		t.Fatalf("nginx reload Agent call=%#v", calls)
	}
	audits, _ := server.store.ListAudit(10, "")
	encoded, _ := json.Marshal(audits)
	if strings.Contains(string(encoded), "configuration validated") || strings.Contains(string(encoded), `"reason"`) {
		t.Fatalf("reason metadata reached audit: %s", encoded)
	}
}

func TestAIFileToolsUseBoundedAgentRoutesAndRedactContent(t *testing.T) {
	server, _ := newTestServer(t)
	agent := &stubAgent{response: AgentResponse{StatusCode: 200, ContentType: "application/json", Body: []byte(`{"ok":true}`)}}
	server.agent = agent
	tools := &panelAITools{server: server}
	execution := ai.ToolExecutionContext{UserID: "admin", SessionID: "ses", RunID: "run", ToolCallID: "call"}
	if _, err := tools.Execute(context.Background(), execution, "host_file_read", json.RawMessage(`{"path":"/tmp/kpanel-ai-test.txt"}`)); err != nil {
		t.Fatal(err)
	}
	version := "sha256:" + strings.Repeat("a", 64)
	arguments := json.RawMessage(`{"path":"/tmp/kpanel-ai-test.txt","content":"sensitive-value","expectedResourceVersion":"` + version + `"}`)
	if _, err := tools.Execute(context.Background(), execution, "host_file_write", arguments); err != nil {
		t.Fatal(err)
	}
	calls := agent.snapshotCalls()
	if len(calls) != 2 || calls[0].path != "/v1/files/text" || calls[0].rawQuery != "path=%2Ftmp%2Fkpanel-ai-test.txt" ||
		calls[1].path != "/v1/files/content" || calls[1].method != http.MethodPut || calls[1].rawQuery != "path=%2Ftmp%2Fkpanel-ai-test.txt" {
		t.Fatalf("file tool calls=%#v", calls)
	}
	audits, _ := server.store.ListAudit(20, "")
	for _, event := range audits {
		encoded, _ := json.Marshal(event.Change)
		if strings.Contains(string(encoded), "sensitive-value") {
			t.Fatalf("file content leaked into audit: %s", encoded)
		}
	}
}

func TestAIToolAgentProblemIsSafeAndTraceable(t *testing.T) {
	server, _ := newTestServer(t)
	server.agent = &stubAgent{response: AgentResponse{StatusCode: http.StatusBadRequest, ContentType: "application/problem+json", Body: []byte(`{"title":"invalid","status":400,"code":"invalid_request","detail":"secret payload","requestId":"req-123"}`)}}
	tools := &panelAITools{server: server}
	_, err := tools.Execute(context.Background(), ai.ToolExecutionContext{UserID: "admin"}, "host_system_summary", json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "invalid_request") || !strings.Contains(err.Error(), "req-123") || strings.Contains(err.Error(), "secret payload") || strings.Contains(err.Error(), "{\"") {
		t.Fatalf("unsafe or untraceable Agent error: %v", err)
	}
}
