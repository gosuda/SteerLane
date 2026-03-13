// Package messenger defines the common interface for messenger platform adapters.
//
// Every messenger platform (Slack, Discord, Telegram) implements the Messenger
// interface. The orchestrator and HITL router depend only on this interface,
// keeping platform-specific details confined to adapter packages.
package messenger

import "context"

// Platform identifies a messenger platform.
type Platform string

const (
	PlatformSlack    Platform = "slack"
	PlatformDiscord  Platform = "discord"
	PlatformTelegram Platform = "telegram"
)

// SendMessageParams holds the parameters for sending a message to a channel.
type SendMessageParams struct {
	ChannelID string
	Text      string
	// Blocks is optional platform-specific rich content (e.g., Slack Block Kit JSON).
	Blocks []byte
}

// CreateThreadParams holds the parameters for creating a threaded reply.
type CreateThreadParams struct {
	ChannelID       string
	ParentMessageID string
	Text            string
	// ActionID carries optional interaction routing metadata for platforms that
	// support structured thread actions.
	ActionID string
	// Options are structured choices for HITL question threads.
	// Empty means free-text only.
	Options []ThreadOption
	// Blocks is optional platform-specific rich content.
	Blocks []byte
}

// ThreadOption represents a selectable option in a HITL question thread.
type ThreadOption struct {
	Label string
	Value string
}

// UpdateMessageParams holds the parameters for editing an existing message.
type UpdateMessageParams struct {
	ChannelID string
	MessageID string
	Text      string
	// Blocks is optional platform-specific rich content.
	Blocks []byte
}

// NotificationParams holds the parameters for sending a direct notification.
type NotificationParams struct {
	UserExternalID string
	Text           string
	// Blocks is optional platform-specific rich content (e.g., Slack Block Kit JSON).
	Blocks []byte
}

// MessageResult is returned after a successful send or thread creation.
type MessageResult struct {
	MessageID string
	ThreadID  string
}

// Messenger is the common interface every platform adapter must implement.
// See SPEC.md section 9.1 for the authoritative contract.
type Messenger interface {
	// SendMessage sends a text message to a channel. Returns a message identifier.
	SendMessage(ctx context.Context, params SendMessageParams) (MessageResult, error)

	// CreateThread creates a threaded reply under an existing message,
	// optionally with structured question options. Returns a thread identifier.
	CreateThread(ctx context.Context, params CreateThreadParams) (MessageResult, error)

	// UpdateMessage edits an existing message (for status updates on task cards).
	UpdateMessage(ctx context.Context, params UpdateMessageParams) error

	// SendNotification sends a direct/push notification to a specific user.
	SendNotification(ctx context.Context, params NotificationParams) error

	// Platform returns the platform identifier string.
	Platform() Platform
}
