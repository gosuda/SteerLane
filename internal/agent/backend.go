package agent

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
)

// SessionOpts configures an agent session.
type SessionOpts struct {
	Env        map[string]string
	Prompt     string
	RepoPath   string
	BranchName string
	SessionID  domain.AgentSessionID
	ProjectID  domain.ProjectID
	TaskID     domain.TaskID
	TenantID   domain.TenantID
}

// MessageType classifies agent output messages.
type MessageType string

const (
	// MessageText is a regular text output line from the agent.
	MessageText MessageType = "text"
	// MessageToolCall indicates the agent wants to invoke a tool.
	MessageToolCall MessageType = "tool_call"
	// MessageToolResult is the result returned to the agent from a tool.
	MessageToolResult MessageType = "tool_result"
	// MessageStatus is a lifecycle status change notification.
	MessageStatus MessageType = "status"
	// MessageTokenUsage reports token consumption.
	MessageTokenUsage MessageType = "token_usage"
	// MessageError is an error message from the agent.
	MessageError MessageType = "error"
)

// ToolCall represents an agent's request to invoke a tool.
type ToolCall struct {
	// ID uniquely identifies this tool call within the session.
	ID string `json:"id"`
	// Name is the tool being invoked (e.g., "create_adr", "ask_human").
	Name string `json:"name"`
	// Input is the JSON-encoded arguments for the tool.
	Input json.RawMessage `json:"input"`
}

// ToolResult is sent back to the agent after a tool call completes.
type ToolResult struct {
	// ToolCallID matches the ID from the original ToolCall.
	ToolCallID string `json:"tool_call_id"`
	// Output is the result content.
	Output string `json:"output"`
	// IsError indicates whether the tool execution failed.
	IsError bool `json:"is_error,omitempty"`
}

// TokenUsage reports token consumption for a message or session.
type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Message is an agent output envelope.
type Message struct {
	ToolCall   *ToolCall   `json:"tool_call,omitempty"`
	ToolResult *ToolResult `json:"tool_result,omitempty"`
	TokenUsage *TokenUsage `json:"token_usage,omitempty"`
	Type       MessageType `json:"type"`
	Content    string      `json:"content,omitempty"`
	SessionID  uuid.UUID   `json:"session_id"`
}

// MessageHandler processes agent messages. Implementations must be safe for
// concurrent calls from the backend's output goroutine.
type MessageHandler func(Message)

// Backend defines the interface every coding agent integration must implement.
// Each Backend instance manages a single agent session.
type Backend interface {
	// StartSession initializes and begins agent execution with the given options.
	// This is a non-blocking call; output is delivered via the registered handler.
	StartSession(ctx context.Context, opts SessionOpts) error

	// SendPrompt sends a follow-up message (e.g., HITL answer) to the running agent.
	SendPrompt(ctx context.Context, prompt string) error

	// Cancel gracefully stops the running agent session.
	Cancel(ctx context.Context) error

	// OnMessage registers a handler for agent output. Must be called before StartSession.
	OnMessage(handler MessageHandler)

	// Dispose releases all resources (processes, connections). Must be called after
	// the session ends (completed, failed, or cancelled). Safe to call multiple times.
	Dispose() error
}
