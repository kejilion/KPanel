package panel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/ai"
	"github.com/kejilion/kejilion-panel/internal/auth"
)

// EnableAI initializes the optional AI module. An error disables only AI;
// callers must keep serving the rest of KPanel.
func (s *Server) EnableAI() error {
	service, err := ai.Open(s.config.DataDir, &panelAITools{server: s})
	if err != nil {
		s.aiError = err.Error()
		return err
	}
	s.ai = service
	s.aiError = ""
	return nil
}

func (s *Server) handleAI(w http.ResponseWriter, r *http.Request) {
	_, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if s.ai == nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "ai_unavailable", "AI module unavailable", s.aiError)
		return
	}
	if r.Method != http.MethodGet {
		if !s.checkOrigin(w, r) || !s.checkCSRF(w, r, session) {
			return
		}
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/ai"), "/")
	parts := []string{}
	if path != "" {
		parts = strings.Split(path, "/")
	}
	switch {
	case len(parts) == 1 && parts[0] == "providers":
		s.aiProviders(w, r)
	case len(parts) == 2 && parts[0] == "providers":
		s.aiProvider(w, r, parts[1])
	case len(parts) == 3 && parts[0] == "providers" && parts[2] == "test":
		s.aiProviderTest(w, r, parts[1])
	case len(parts) == 4 && parts[0] == "providers" && parts[2] == "models" && parts[3] == "sync":
		s.aiModelSync(w, r, parts[1])
	case len(parts) == 3 && parts[0] == "providers" && parts[2] == "models":
		s.aiModelAdd(w, r, parts[1])
	case len(parts) == 1 && parts[0] == "models":
		s.aiModels(w, r)
	case len(parts) == 1 && parts[0] == "sessions":
		s.aiSessions(w, r, session.User.ID)
	case len(parts) == 2 && parts[0] == "sessions":
		s.aiSession(w, r, session.User.ID, parts[1])
	case len(parts) == 3 && parts[0] == "sessions" && parts[2] == "messages":
		s.aiMessages(w, r, session.User.ID, parts[1])
	case len(parts) == 2 && parts[0] == "runs":
		s.aiRun(w, r, session.User.ID, parts[1])
	case len(parts) == 3 && parts[0] == "runs" && parts[2] == "events":
		s.aiRunEvents(w, r, session, parts[1])
	case len(parts) == 3 && parts[0] == "runs" && parts[2] == "decision":
		s.aiRunDecision(w, r, session.User.ID, parts[1])
	case len(parts) == 3 && parts[0] == "runs" && parts[2] == "cancel":
		s.aiRunCancel(w, r, session.User.ID, parts[1])
	case len(parts) == 3 && parts[0] == "runs" && parts[2] == "retry":
		s.aiRunRetry(w, r, session.User.ID, parts[1])
	case len(parts) == 3 && parts[0] == "runs" && parts[2] == "evolution":
		s.aiRunEvolution(w, r, session.User.ID, parts[1])
	case len(parts) == 2 && parts[0] == "evolution" && parts[1] == "proposals":
		s.aiProposals(w, r, session.User.ID)
	case len(parts) == 4 && parts[0] == "evolution" && parts[1] == "proposals" && (parts[3] == "approve" || parts[3] == "reject"):
		s.aiProposalDecision(w, r, session.User.ID, parts[2], parts[3] == "approve")
	case len(parts) == 1 && parts[0] == "memories":
		s.aiMemories(w, r, session.User.ID)
	case len(parts) == 2 && parts[0] == "memories":
		s.aiMemory(w, r, session.User.ID, parts[1])
	case len(parts) == 1 && parts[0] == "procedures":
		s.aiProcedures(w, r, session.User.ID)
	case len(parts) == 2 && parts[0] == "procedures":
		s.aiProcedure(w, r, session.User.ID, parts[1])
	default:
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
	}
}

func (s *Server) aiRunRetry(w http.ResponseWriter, r *http.Request, userID, id string) {
	if r.Method != http.MethodPost {
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	run, err := s.ai.Retry(r.Context(), userID, id)
	s.aiJSON(w, r, map[string]string{"runId": run.ID}, err, http.StatusAccepted)
}

func (s *Server) aiRunEvolution(w http.ResponseWriter, r *http.Request, userID, id string) {
	if r.Method != http.MethodPost {
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	err := s.ai.Propose(r.Context(), userID, id)
	s.aiJSON(w, r, map[string]bool{"created": err == nil}, err, http.StatusCreated)
}

func (s *Server) aiProposals(w http.ResponseWriter, r *http.Request, userID string) {
	if r.Method != http.MethodGet {
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
		return
	}
	items, err := s.ai.Store.Proposals(r.Context(), userID, ai.EvolutionPending)
	s.aiJSON(w, r, items, err, http.StatusOK)
}

func (s *Server) aiProposalDecision(w http.ResponseWriter, r *http.Request, userID, id string, approve bool) {
	if r.Method != http.MethodPost {
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
		return
	}
	valid := map[string]bool{}
	tools := &panelAITools{server: s}
	for _, definition := range tools.Definitions() {
		valid[definition.Name] = true
	}
	if approve {
		items, err := s.ai.Store.Proposals(r.Context(), userID, ai.EvolutionPending)
		if err != nil {
			s.aiJSON(w, r, nil, err, 0)
			return
		}
		for _, item := range items {
			if item.ID != id || item.Type != ai.EvolutionProcedure {
				continue
			}
			var body struct {
				Steps []ai.ProcedureStep `json:"steps"`
			}
			if json.Unmarshal(item.Payload, &body) != nil {
				s.aiJSON(w, r, nil, ai.ErrConflict, 0)
				return
			}
			for _, step := range body.Steps {
				if err := tools.DryRun(step.Tool, step.Arguments); err != nil {
					s.aiJSON(w, r, nil, fmt.Errorf("procedure dry-run failed: %w", err), 0)
					return
				}
			}
		}
	}
	err := s.ai.Store.DecideProposal(r.Context(), userID, id, approve, valid)
	s.aiJSON(w, r, map[string]bool{"approved": approve && err == nil, "rejected": !approve && err == nil}, err, http.StatusOK)
}

func (s *Server) aiMemories(w http.ResponseWriter, r *http.Request, userID string) {
	if r.Method != http.MethodGet {
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
		return
	}
	items, err := s.ai.Store.Memories(r.Context(), userID)
	s.aiJSON(w, r, items, err, http.StatusOK)
}
func (s *Server) aiMemory(w http.ResponseWriter, r *http.Request, userID, id string) {
	switch r.Method {
	case http.MethodPatch:
		var input struct {
			Enabled *bool `json:"enabled"`
		}
		if s.decodeJSON(w, r, &input) != nil {
			return
		}
		item, err := s.ai.Store.UpdateMemory(r.Context(), userID, id, input.Enabled)
		s.aiJSON(w, r, item, err, http.StatusOK)
	case http.MethodDelete:
		err := s.ai.Store.DeleteMemory(r.Context(), userID, id)
		s.aiJSON(w, r, map[string]bool{"deleted": err == nil}, err, http.StatusOK)
	default:
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
	}
}

func (s *Server) decodeAIMessageJSON(w http.ResponseWriter, r *http.Request, target any) error {
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		s.writeProblem(w, r, http.StatusUnsupportedMediaType, "json_required", "JSON request body required", "")
		return errors.New("invalid content type")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 12<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			s.writeProblem(w, r, http.StatusRequestEntityTooLarge, "request_too_large", "AI message or attachments are too large", "")
			return err
		}
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid JSON", "")
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid JSON", "")
		return errors.New("multiple JSON values")
	}
	return nil
}

type aiMessageInput struct {
	Content     string `json:"content"`
	Attachments []struct {
		Name     string `json:"name"`
		MimeType string `json:"mimeType"`
		Data     string `json:"data"`
	} `json:"attachments"`
}

func (s *Server) decodeAIMessage(w http.ResponseWriter, r *http.Request) (string, []ai.Attachment, error) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		s.writeProblem(w, r, http.StatusUnsupportedMediaType, "message_media_type_required", "Use JSON or multipart form data", "")
		return "", nil, err
	}
	if mediaType == "application/json" {
		var input aiMessageInput
		if err := s.decodeAIMessageJSON(w, r, &input); err != nil {
			return "", nil, err
		}
		attachments := make([]ai.Attachment, 0, len(input.Attachments))
		for index, item := range input.Attachments {
			data, decodeErr := base64.StdEncoding.DecodeString(item.Data)
			if decodeErr != nil {
				s.writeValidationProblem(w, r, fmt.Sprintf("attachments.%d.data", index), "attachment data must be valid base64")
				return "", nil, decodeErr
			}
			attachments = append(attachments, ai.Attachment{Name: item.Name, MimeType: item.MimeType, Data: data})
		}
		return input.Content, attachments, nil
	}
	if mediaType != "multipart/form-data" {
		s.writeProblem(w, r, http.StatusUnsupportedMediaType, "message_media_type_required", "Use JSON or multipart form data", "")
		return "", nil, errors.New("unsupported AI message content type")
	}

	r.Body = http.MaxBytesReader(w, r.Body, 12<<20)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			s.writeProblem(w, r, http.StatusRequestEntityTooLarge, "request_too_large", "AI message or attachments are too large", "")
		} else {
			s.writeProblem(w, r, http.StatusBadRequest, "invalid_multipart", "Invalid multipart form data", "")
		}
		return "", nil, err
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
		for name := range r.MultipartForm.Value {
			if name != "content" {
				s.writeValidationProblem(w, r, name, "unknown message field")
				return "", nil, errors.New("unknown multipart value field")
			}
		}
		for name := range r.MultipartForm.File {
			if name != "attachments" {
				s.writeValidationProblem(w, r, name, "unknown attachment field")
				return "", nil, errors.New("unknown multipart file field")
			}
		}
		if len(r.MultipartForm.Value["content"]) > 1 {
			s.writeValidationProblem(w, r, "content", "content must be provided at most once")
			return "", nil, errors.New("duplicate multipart content")
		}
	}

	files := r.MultipartForm.File["attachments"]
	attachments := make([]ai.Attachment, 0, len(files))
	for index, header := range files {
		file, openErr := header.Open()
		if openErr != nil {
			s.writeValidationProblem(w, r, fmt.Sprintf("attachments.%d", index), "attachment could not be read")
			return "", nil, openErr
		}
		data, readErr := io.ReadAll(io.LimitReader(file, (4<<20)+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			if readErr == nil {
				readErr = closeErr
			}
			s.writeValidationProblem(w, r, fmt.Sprintf("attachments.%d", index), "attachment could not be read")
			return "", nil, readErr
		}
		attachments = append(attachments, ai.Attachment{Name: header.Filename, MimeType: header.Header.Get("Content-Type"), Data: data})
	}
	return r.FormValue("content"), attachments, nil
}
func (s *Server) aiProcedures(w http.ResponseWriter, r *http.Request, userID string) {
	if r.Method != http.MethodGet {
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
		return
	}
	items, err := s.ai.Store.Procedures(r.Context(), userID)
	s.aiJSON(w, r, items, err, http.StatusOK)
}
func (s *Server) aiProcedure(w http.ResponseWriter, r *http.Request, userID, id string) {
	switch r.Method {
	case http.MethodPatch:
		var input struct {
			Enabled           *bool           `json:"enabled"`
			Title             string          `json:"title"`
			Condition         string          `json:"condition"`
			Steps             json.RawMessage `json:"steps"`
			RollbackToVersion int             `json:"rollbackToVersion"`
		}
		if s.decodeJSON(w, r, &input) != nil {
			return
		}
		var item ai.Procedure
		var err error
		if input.Title != "" || input.Condition != "" || len(input.Steps) > 0 || input.RollbackToVersion > 0 {
			if len(input.Steps) > 0 {
				var steps []ai.ProcedureStep
				if json.Unmarshal(input.Steps, &steps) != nil {
					s.writeValidationProblem(w, r, "steps", "steps must be a valid array")
					return
				}
				tools := &panelAITools{server: s}
				for _, step := range steps {
					if err := tools.DryRun(step.Tool, step.Arguments); err != nil {
						s.writeValidationProblem(w, r, "steps", err.Error())
						return
					}
				}
			}
			item, err = s.ai.Store.ReviseProcedure(r.Context(), userID, id, input.Title, input.Condition, input.Steps, input.RollbackToVersion)
		} else {
			item, err = s.ai.Store.UpdateProcedure(r.Context(), userID, id, input.Enabled)
		}
		s.aiJSON(w, r, item, err, http.StatusOK)
	case http.MethodDelete:
		err := s.ai.Store.DeleteProcedure(r.Context(), userID, id)
		s.aiJSON(w, r, map[string]bool{"deleted": err == nil}, err, http.StatusOK)
	default:
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
	}
}

func (s *Server) aiProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.ai.Store.ListProviders(r.Context())
		s.aiJSON(w, r, items, err, http.StatusOK)
	case http.MethodPost:
		var input ai.ProviderInput
		if s.decodeJSON(w, r, &input) != nil {
			return
		}
		item, err := s.ai.Providers.Save(r.Context(), "", input)
		s.aiJSON(w, r, item, err, http.StatusCreated)
	default:
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
	}
}

func (s *Server) aiProvider(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodPatch:
		var input ai.ProviderInput
		if s.decodeJSON(w, r, &input) != nil {
			return
		}
		item, err := s.ai.Providers.Save(r.Context(), id, input)
		s.aiJSON(w, r, item, err, http.StatusOK)
	case http.MethodDelete:
		err := s.ai.DeleteProvider(r.Context(), id)
		s.aiJSON(w, r, map[string]bool{"deleted": err == nil}, err, http.StatusOK)
	default:
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
	}
}

func (s *Server) aiProviderTest(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
	defer cancel()
	err := s.ai.TestProvider(ctx, id)
	s.aiJSON(w, r, map[string]bool{"ok": err == nil}, err, http.StatusOK)
}

func (s *Server) aiModelSync(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
	defer cancel()
	items, err := s.ai.SyncModels(ctx, id)
	s.aiJSON(w, r, items, err, http.StatusOK)
}

func (s *Server) aiModelAdd(w http.ResponseWriter, r *http.Request, providerID string) {
	if r.Method != http.MethodPost {
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
		return
	}
	var input struct {
		ModelID       string `json:"modelId"`
		DisplayName   string `json:"displayName"`
		ContextWindow int    `json:"contextWindow"`
		ToolCalling   bool   `json:"toolCalling"`
		Vision        *bool  `json:"vision"`
		Reasoning     bool   `json:"reasoning"`
		Enabled       bool   `json:"enabled"`
		IsDefault     bool   `json:"isDefault"`
	}
	if s.decodeJSON(w, r, &input) != nil {
		return
	}
	if strings.TrimSpace(input.ModelID) == "" {
		s.writeValidationProblem(w, r, "modelId", "modelId is required")
		return
	}
	if input.DisplayName == "" {
		input.DisplayName = input.ModelID
	}
	if input.ContextWindow <= 0 {
		input.ContextWindow = 32000
	}
	vision := true
	if input.Vision != nil {
		vision = *input.Vision
	}
	model := ai.Model{ProviderID: providerID, ModelID: input.ModelID, DisplayName: input.DisplayName,
		ContextWindow: input.ContextWindow, ToolCalling: input.ToolCalling, Vision: vision,
		Reasoning: input.Reasoning, Enabled: input.Enabled, IsDefault: input.IsDefault}
	err := s.ai.Store.SaveModels(r.Context(), providerID, []ai.Model{model})
	items, _ := s.ai.Store.ListModels(r.Context(), providerID)
	s.aiJSON(w, r, items, err, http.StatusCreated)
}

func (s *Server) aiModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
		return
	}
	items, err := s.ai.Store.ListModels(r.Context(), r.URL.Query().Get("providerId"))
	s.aiJSON(w, r, items, err, http.StatusOK)
}

func (s *Server) aiSessions(w http.ResponseWriter, r *http.Request, userID string) {
	switch r.Method {
	case http.MethodGet:
		archived, _ := strconv.ParseBool(r.URL.Query().Get("archived"))
		items, err := s.ai.Store.Sessions(r.Context(), userID, r.URL.Query().Get("search"), archived, 50)
		s.aiJSON(w, r, items, err, http.StatusOK)
	case http.MethodPost:
		var input struct{ ProviderID, ModelID, Title string }
		if s.decodeJSON(w, r, &input) != nil {
			return
		}
		item, err := s.ai.CreateSession(r.Context(), userID, input.ProviderID, input.ModelID, input.Title)
		s.aiJSON(w, r, item, err, http.StatusCreated)
	default:
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
	}
}

func (s *Server) aiSession(w http.ResponseWriter, r *http.Request, userID, id string) {
	switch r.Method {
	case http.MethodGet:
		item, err := s.ai.Store.Session(r.Context(), userID, id)
		s.aiJSON(w, r, item, err, http.StatusOK)
	case http.MethodPatch:
		var input struct {
			Title, ProviderID, ModelID string
			Pinned, Archived           *bool
			ApprovalMode               *ai.ApprovalMode
			ThinkingLevel              *ai.ThinkingLevel
		}
		if s.decodeJSON(w, r, &input) != nil {
			return
		}
		providerName, modelName := "", ""
		if input.ProviderID != "" || input.ModelID != "" {
			provider, err := s.ai.Store.Provider(r.Context(), input.ProviderID)
			if err != nil {
				s.aiJSON(w, r, nil, err, 0)
				return
			}
			model, err := s.ai.Store.Model(r.Context(), input.ModelID)
			if err != nil || model.ProviderID != provider.ID {
				s.aiJSON(w, r, nil, ai.ErrNotFound, 0)
				return
			}
			providerName, modelName = provider.Name, model.DisplayName
		}
		item, err := s.ai.Store.UpdateSession(r.Context(), userID, id, strings.TrimSpace(input.Title), input.ProviderID, input.ModelID, providerName, modelName, input.Pinned, input.Archived, input.ApprovalMode, input.ThinkingLevel)
		s.aiJSON(w, r, item, err, http.StatusOK)
	case http.MethodDelete:
		err := s.ai.DeleteSession(r.Context(), userID, id)
		s.aiJSON(w, r, map[string]bool{"deleted": err == nil}, err, http.StatusOK)
	default:
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
	}
}

func (s *Server) aiMessages(w http.ResponseWriter, r *http.Request, userID, sessionID string) {
	session, err := s.ai.Store.Session(r.Context(), userID, sessionID)
	if err != nil {
		s.aiJSON(w, r, nil, err, 0)
		return
	}
	switch r.Method {
	case http.MethodGet:
		cursor := r.URL.Query().Get("cursor")
		page, err := s.ai.Store.ConversationMessageMetadataPage(r.Context(), sessionID, 50, cursor)
		if err != nil {
			s.aiJSON(w, r, nil, err, 0)
			return
		}
		calls := []ai.ToolCall{}
		if cursor == "" && session.LastRunID != "" {
			calls, err = s.ai.Store.ToolCalls(r.Context(), session.LastRunID)
			if err != nil {
				s.aiJSON(w, r, nil, err, 0)
				return
			}
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"items": page.Items, "nextCursor": page.NextCursor, "toolCalls": calls})
	case http.MethodPost:
		content, attachments, decodeErr := s.decodeAIMessage(w, r)
		if decodeErr != nil {
			return
		}
		run, err := s.ai.SendWithAttachments(r.Context(), userID, sessionID, content, attachments)
		s.aiJSON(w, r, map[string]string{"runId": run.ID}, err, http.StatusAccepted)
	default:
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
	}
}

func (s *Server) aiRun(w http.ResponseWriter, r *http.Request, userID, id string) {
	if r.Method != http.MethodGet {
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
		return
	}
	run, err := s.ai.Store.Run(r.Context(), userID, id)
	s.aiJSON(w, r, run, err, http.StatusOK)
}

func (s *Server) aiRunDecision(w http.ResponseWriter, r *http.Request, userID, id string) {
	if r.Method != http.MethodPost {
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
		return
	}
	if _, err := s.ai.Store.Run(r.Context(), userID, id); err != nil {
		s.aiJSON(w, r, nil, err, 0)
		return
	}
	var input ai.Decision
	if s.decodeJSON(w, r, &input) != nil {
		return
	}
	s.ai.Resume(id, input)
	s.writeJSON(w, http.StatusAccepted, map[string]bool{"accepted": true})
}

func (s *Server) aiRunCancel(w http.ResponseWriter, r *http.Request, userID, id string) {
	if r.Method != http.MethodPost {
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
		return
	}
	if _, err := s.ai.Store.Run(r.Context(), userID, id); err != nil {
		s.aiJSON(w, r, nil, err, 0)
		return
	}
	err := s.ai.Runtime.Cancel(r.Context(), id)
	s.aiJSON(w, r, map[string]bool{"cancelled": err == nil}, err, http.StatusOK)
}

func (s *Server) aiRunEvents(w http.ResponseWriter, r *http.Request, session auth.Session, id string) {
	if r.Method != http.MethodGet {
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
		return
	}
	run, err := s.ai.Store.Run(r.Context(), session.User.ID, id)
	if err != nil {
		s.aiJSON(w, r, nil, err, 0)
		return
	}
	page, err := s.ai.Store.ConversationMessageMetadataPage(r.Context(), run.SessionID, 50, "")
	if err != nil {
		s.aiJSON(w, r, nil, err, 0)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeProblem(w, r, 500, "stream_unavailable", "Streaming unavailable", "")
		return
	}
	write := func(event string, value any) bool {
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(20 * time.Second))
		data, _ := json.Marshal(value)
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	calls, _ := s.ai.Store.ToolCalls(r.Context(), id)
	if !write("run.snapshot", map[string]any{"run": run, "toolCalls": calls, "messages": page.Items}) {
		return
	}
	channel, unsubscribe := s.ai.Events.Subscribe(id)
	defer unsubscribe()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-channel:
			if !write(event.Type, event.Data) {
				return
			}
		case <-heartbeat.C:
			if time.Now().After(session.ExpiresAt) {
				write("auth.expired", map[string]string{"message": "Session expired"})
				return
			}
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) aiJSON(w http.ResponseWriter, r *http.Request, value any, err error, status int) {
	if err == nil {
		s.writeJSON(w, status, value)
		return
	}
	code, title, httpStatus := "ai_request_failed", "AI request failed", http.StatusUnprocessableEntity
	switch {
	case errors.Is(err, ai.ErrNotFound):
		code, title, httpStatus = "not_found", "Not found", 404
	case errors.Is(err, ai.ErrConflict):
		code, title, httpStatus = "conflict", "Record changed", 409
	case errors.Is(err, ai.ErrBusy):
		code, title, httpStatus = "run_active", "Run is active", 409
	}
	s.writeProblem(w, r, httpStatus, code, title, ai.PublicError(err))
}
