package ai

import (
	"encoding/json"
	"time"
)

type ProviderProtocol string

const (
	ProtocolOpenAICompatible ProviderProtocol = "openai_compatible"
	ProtocolAnthropic        ProviderProtocol = "anthropic"
	ProtocolGemini           ProviderProtocol = "gemini"
)

type OpenAIAPIMode string

const (
	OpenAIChatCompletions OpenAIAPIMode = "chat_completions"
	OpenAIResponses       OpenAIAPIMode = "responses"
)

type EndpointScope string

const (
	EndpointPublic  EndpointScope = "public"
	EndpointPrivate EndpointScope = "private"
)

type Provider struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Protocol      ProviderProtocol `json:"protocol"`
	APIMode       OpenAIAPIMode    `json:"apiMode,omitempty"`
	BaseURL       string           `json:"baseUrl"`
	EndpointScope EndpointScope    `json:"endpointScope"`
	Enabled       bool             `json:"enabled"`
	APIKeySet     bool             `json:"apiKeySet"`
	APIKeyHint    string           `json:"apiKeyHint,omitempty"`
	EncryptedKey  []byte           `json:"-"`
	Version       int64            `json:"version"`
	CreatedAt     time.Time        `json:"createdAt"`
	UpdatedAt     time.Time        `json:"updatedAt"`
}

type Model struct {
	ID            string    `json:"id"`
	ProviderID    string    `json:"providerId"`
	ModelID       string    `json:"modelId"`
	DisplayName   string    `json:"displayName"`
	ContextWindow int       `json:"contextWindow"`
	ToolCalling   bool      `json:"toolCalling"`
	Vision        bool      `json:"vision"`
	Reasoning     bool      `json:"reasoning"`
	Enabled       bool      `json:"enabled"`
	IsDefault     bool      `json:"isDefault"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type Session struct {
	ID             string        `json:"id"`
	UserID         string        `json:"userId"`
	Title          string        `json:"title"`
	ProviderID     string        `json:"providerId"`
	ModelID        string        `json:"modelId"`
	ProviderName   string        `json:"providerName,omitempty"`
	ModelName      string        `json:"modelName,omitempty"`
	Summary        string        `json:"summary,omitempty"`
	ApprovalMode   ApprovalMode  `json:"approvalMode"`
	ThinkingLevel  ThinkingLevel `json:"thinkingLevel"`
	Pinned         bool          `json:"pinned"`
	Archived       bool          `json:"archived"`
	CreatedAt      time.Time     `json:"createdAt"`
	UpdatedAt      time.Time     `json:"updatedAt"`
	LastMessageAt  time.Time     `json:"lastMessageAt"`
	ModelAvailable bool          `json:"modelAvailable"`
	Running        bool          `json:"running"`
	ActiveRunID    string        `json:"activeRunId,omitempty"`
	LastRunID      string        `json:"lastRunId,omitempty"`
	LastRunStatus  RunStatus     `json:"lastRunStatus,omitempty"`
}

type ApprovalMode string

const (
	ApprovalManual ApprovalMode = "manual"
	ApprovalAuto   ApprovalMode = "auto"
)

func (mode ApprovalMode) Valid() bool {
	return mode == ApprovalManual || mode == ApprovalAuto
}

type ThinkingLevel string

const (
	ThinkingLow    ThinkingLevel = "low"
	ThinkingMedium ThinkingLevel = "medium"
	ThinkingHigh   ThinkingLevel = "high"
)

func (level ThinkingLevel) Valid() bool {
	return level == ThinkingLow || level == ThinkingMedium || level == ThinkingHigh
}

type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

type Message struct {
	RequiredAttachments bool `json:"-"`

	ID           string       `json:"id"`
	SessionID    string       `json:"sessionId"`
	RunID        string       `json:"runId,omitempty"`
	Role         MessageRole  `json:"role"`
	Content      string       `json:"content"`
	ProviderID   string       `json:"providerId,omitempty"`
	ProviderName string       `json:"providerName,omitempty"`
	ModelID      string       `json:"modelId,omitempty"`
	ModelName    string       `json:"modelName,omitempty"`
	ToolCallID   string       `json:"toolCallId,omitempty"`
	Attachments  []Attachment `json:"attachments,omitempty"`
	CreatedAt    time.Time    `json:"createdAt"`
}

type Attachment struct {
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	Size     int    `json:"size"`
	Kind     string `json:"kind"`
	Data     []byte `json:"-"`
}

type RunStatus string

const (
	RunQueued          RunStatus = "queued"
	RunRunning         RunStatus = "running"
	RunPendingApproval RunStatus = "pending_approval"
	RunCompleted       RunStatus = "completed"
	RunFailed          RunStatus = "failed"
	RunCancelled       RunStatus = "cancelled"
	RunInterrupted     RunStatus = "interrupted"
)

type Usage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	TotalTokens  int `json:"totalTokens"`
}

type Run struct {
	// nil is legacy/unknown, empty is an independent input, otherwise the retry parent.
	RetryOf       *string       `json:"-"`
	ID            string        `json:"id"`
	SessionID     string        `json:"sessionId"`
	UserID        string        `json:"userId"`
	ProviderID    string        `json:"providerId"`
	ProviderName  string        `json:"providerName"`
	ModelID       string        `json:"modelId"`
	ModelName     string        `json:"modelName"`
	ApprovalMode  ApprovalMode  `json:"approvalMode"`
	ThinkingLevel ThinkingLevel `json:"thinkingLevel"`
	Status        RunStatus     `json:"status"`
	Step          int           `json:"step"`
	Usage         Usage         `json:"usage"`
	ErrorCode     string        `json:"errorCode,omitempty"`
	ErrorMessage  string        `json:"errorMessage,omitempty"`
	CreatedAt     time.Time     `json:"createdAt"`
	UpdatedAt     time.Time     `json:"updatedAt"`
	FinishedAt    time.Time     `json:"finishedAt,omitempty"`
}

type ToolCallStatus string

const (
	ToolPendingApproval ToolCallStatus = "pending_approval"
	ToolRunning         ToolCallStatus = "running"
	ToolCompleted       ToolCallStatus = "completed"
	ToolRejected        ToolCallStatus = "rejected"
	ToolFailed          ToolCallStatus = "failed"
)

type ToolCall struct {
	ID               string          `json:"id"`
	RunID            string          `json:"runId"`
	SessionID        string          `json:"sessionId"`
	Name             string          `json:"name"`
	Arguments        json.RawMessage `json:"arguments,omitempty"`
	ProviderData     json.RawMessage `json:"-"`
	ArgumentsPreview string          `json:"argumentsPreview,omitempty"`
	ResultPreview    string          `json:"resultPreview,omitempty"`
	Status           ToolCallStatus  `json:"status"`
	RequiresApproval bool            `json:"requiresApproval"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}

type EvolutionType string

const (
	EvolutionMemory    EvolutionType = "memory"
	EvolutionProcedure EvolutionType = "procedure"
)

type EvolutionStatus string

const (
	EvolutionPending  EvolutionStatus = "pending"
	EvolutionActive   EvolutionStatus = "active"
	EvolutionRejected EvolutionStatus = "rejected"
	EvolutionRetired  EvolutionStatus = "retired"
)

type EvolutionProposal struct {
	ID        string          `json:"id"`
	UserID    string          `json:"userId"`
	SessionID string          `json:"sessionId,omitempty"`
	RunID     string          `json:"runId,omitempty"`
	Type      EvolutionType   `json:"type"`
	Title     string          `json:"title"`
	Content   string          `json:"content"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Status    EvolutionStatus `json:"status"`
	Version   int             `json:"version"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type Memory struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Enabled   bool      `json:"enabled"`
	Retired   bool      `json:"retired"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Procedure struct {
	ID        string          `json:"id"`
	UserID    string          `json:"userId"`
	Title     string          `json:"title"`
	Condition string          `json:"condition"`
	Steps     json.RawMessage `json:"steps"`
	Enabled   bool            `json:"enabled"`
	Retired   bool            `json:"retired"`
	Version   int             `json:"version"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}
