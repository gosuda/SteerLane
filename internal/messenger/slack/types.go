// Package slack implements the Slack messenger adapter for SteerLane.
//
// This package handles Slack Events API webhooks, interactive component
// callbacks, and signature verification. Command parsing and HITL thread
// routing are out of scope for this slice (see SteerLane-7em).
package slack

import "encoding/json"

// -----------------------------------------------------------------------
// Slack Events API types
// -----------------------------------------------------------------------

// EventRequest is the top-level JSON payload from the Slack Events API.
// Slack sends three envelope types: url_verification, event_callback,
// and app_rate_limited.
type EventRequest struct {
	Token     string          `json:"token"`
	Type      string          `json:"type"`
	Challenge string          `json:"challenge,omitempty"`
	TeamID    string          `json:"team_id,omitempty"`
	EventID   string          `json:"event_id,omitempty"`
	Event     json.RawMessage `json:"event,omitempty"`
}

// InnerEvent holds the common fields present in all Slack event payloads.
type InnerEvent struct {
	Type    string `json:"type"`
	SubType string `json:"subtype,omitempty"`
	TeamID  string `json:"team_id,omitempty"`

	// User is the Slack user ID that triggered the event.
	User string `json:"user,omitempty"`

	// Channel is the channel or DM where the event occurred.
	Channel string `json:"channel,omitempty"`

	// Text is the message text (for message events).
	Text string `json:"text,omitempty"`

	// ThreadTS is the thread timestamp (non-empty for threaded replies).
	ThreadTS string `json:"thread_ts,omitempty"`

	// TS is the message timestamp, used as the message identifier.
	TS string `json:"ts,omitempty"`
}

// ChallengeResponse is the response body for Slack URL verification challenges.
type ChallengeResponse struct {
	Challenge string `json:"challenge"`
}

// -----------------------------------------------------------------------
// Slack Interactive Components types
// -----------------------------------------------------------------------

// InteractionPayload is the top-level JSON payload from Slack interactive
// component callbacks (buttons, menus, modals, shortcuts).
type InteractionPayload struct {
	Channel     *InteractionChannel `json:"channel,omitempty"`
	Message     *InteractionMessage `json:"message,omitempty"`
	User        InteractionUser     `json:"user"`
	Team        InteractionTeam     `json:"team"`
	Type        string              `json:"type"`
	CallbackID  string              `json:"callback_id,omitempty"`
	TriggerID   string              `json:"trigger_id,omitempty"`
	ResponseURL string              `json:"response_url,omitempty"`
	Actions     []InteractionAction `json:"actions,omitempty"`
}

// InteractionUser identifies the user in an interaction payload.
type InteractionUser struct {
	ID       string `json:"id"`
	Username string `json:"username,omitempty"`
	TeamID   string `json:"team_id,omitempty"`
}

// InteractionChannel identifies the channel in an interaction payload.
type InteractionChannel struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// InteractionTeam identifies the workspace in an interaction payload.
type InteractionTeam struct {
	ID     string `json:"id"`
	Domain string `json:"domain,omitempty"`
}

// InteractionAction represents a single user action (button click, menu select).
type InteractionAction struct {
	// SelectedOption is populated for menu/radio selections.
	SelectedOption *ActionOption `json:"selected_option,omitempty"`

	ActionID string `json:"action_id"`
	BlockID  string `json:"block_id,omitempty"`
	Type     string `json:"type"`
	Value    string `json:"value,omitempty"`
}

// ActionOption represents a selected option in a menu or radio button group.
type ActionOption struct {
	Text  ActionText `json:"text"`
	Value string     `json:"value"`
}

// ActionText is a Slack text object used in action options.
type ActionText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// InteractionMessage is the original message containing interactive components.
type InteractionMessage struct {
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts,omitempty"`
	Text     string `json:"text,omitempty"`
}
