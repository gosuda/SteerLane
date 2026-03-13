package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gosuda/steerlane/internal/messenger"
)

const defaultBaseURL = "https://slack.com/api"

// Messenger implements messenger.Messenger using the Slack Web API.
type Messenger struct {
	httpClient *http.Client
	baseURL    string
	botToken   string
}

// Compile-time check.
var _ messenger.Messenger = (*Messenger)(nil)

// Option configures a Messenger.
type Option func(*Messenger)

// WithBaseURL overrides the Slack API base URL.
func WithBaseURL(baseURL string) Option {
	return func(m *Messenger) {
		trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
		if trimmed != "" {
			m.baseURL = trimmed
		}
	}
}

// WithHTTPClient overrides the HTTP client used for Slack API calls.
func WithHTTPClient(client *http.Client) Option {
	return func(m *Messenger) {
		if client != nil {
			m.httpClient = client
		}
	}
}

// NewMessenger creates a Slack messenger adapter.
func NewMessenger(botToken string, opts ...Option) *Messenger {
	m := &Messenger{
		baseURL:    defaultBaseURL,
		botToken:   botToken,
		httpClient: http.DefaultClient,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(m)
		}
	}

	return m
}

// Platform returns the messenger platform identifier.
func (m *Messenger) Platform() messenger.Platform {
	return messenger.PlatformSlack
}

// SendMessage posts a message to a Slack channel.
func (m *Messenger) SendMessage(ctx context.Context, params messenger.SendMessageParams) (messenger.MessageResult, error) {
	resp, err := m.postMessage(ctx, postMessageRequest{
		Channel: params.ChannelID,
		Text:    params.Text,
		Blocks:  json.RawMessage(params.Blocks),
	})
	if err != nil {
		return messenger.MessageResult{}, fmt.Errorf("slack send message: %w", err)
	}

	return messenger.MessageResult{MessageID: resp.TS, ThreadID: resp.TS}, nil
}

// CreateThread posts a threaded reply to an existing Slack message.
func (m *Messenger) CreateThread(ctx context.Context, params messenger.CreateThreadParams) (messenger.MessageResult, error) {
	blocks := json.RawMessage(params.Blocks)
	if len(blocks) == 0 && params.ActionID != "" && len(params.Options) != 0 {
		generated, err := buildQuestionBlocks(params.Text, params.ActionID, params.Options)
		if err != nil {
			return messenger.MessageResult{}, fmt.Errorf("slack create thread: build blocks: %w", err)
		}
		blocks = generated
	}

	resp, err := m.postMessage(ctx, postMessageRequest{
		Channel:  params.ChannelID,
		Text:     params.Text,
		ThreadTS: params.ParentMessageID,
		Blocks:   blocks,
	})
	if err != nil {
		return messenger.MessageResult{}, fmt.Errorf("slack create thread: %w", err)
	}

	threadID := params.ParentMessageID
	if threadID == "" {
		threadID = resp.TS
	}

	return messenger.MessageResult{MessageID: resp.TS, ThreadID: threadID}, nil
}

// UpdateMessage updates an existing Slack message.
func (m *Messenger) UpdateMessage(ctx context.Context, params messenger.UpdateMessageParams) error {
	_, err := m.doJSON(ctx, "chat.update", updateMessageRequest{
		Channel: params.ChannelID,
		TS:      params.MessageID,
		Text:    params.Text,
		Blocks:  json.RawMessage(params.Blocks),
	})
	if err != nil {
		return fmt.Errorf("slack update message: %w", err)
	}

	return nil
}

func buildQuestionBlocks(text, actionID string, options []messenger.ThreadOption) (json.RawMessage, error) {
	elements := make([]map[string]any, 0, min(len(options), 5))
	for idx, option := range options {
		if option.Label == "" || option.Value == "" {
			continue
		}
		elements = append(elements, map[string]any{
			"type":      "button",
			"action_id": fmt.Sprintf("%s#%d", actionID, idx),
			"text": map[string]any{
				"type": "plain_text",
				"text": option.Label,
			},
			"value": option.Value,
		})
		if len(elements) == 5 {
			break
		}
	}

	blocks := []map[string]any{
		{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": text,
			},
		},
	}
	if len(elements) != 0 {
		blocks = append(blocks, map[string]any{
			"type":     "actions",
			"elements": elements,
		})
	}

	data, err := json.Marshal(blocks)
	if err != nil {
		return nil, fmt.Errorf("marshal question blocks: %w", err)
	}
	return data, nil
}

// SendNotification opens a DM channel and posts the notification into it.
func (m *Messenger) SendNotification(ctx context.Context, params messenger.NotificationParams) error {
	openResp, err := m.openConversation(ctx, params.UserExternalID)
	if err != nil {
		return fmt.Errorf("slack send notification: open conversation: %w", err)
	}

	_, err = m.postMessage(ctx, postMessageRequest{
		Channel: openResp.channelID(),
		Text:    params.Text,
		Blocks:  json.RawMessage(params.Blocks),
	})
	if err != nil {
		return fmt.Errorf("slack send notification: post message: %w", err)
	}

	return nil
}

type postMessageRequest struct {
	Channel  string          `json:"channel"`
	Text     string          `json:"text"`
	ThreadTS string          `json:"thread_ts,omitempty"`
	Blocks   json.RawMessage `json:"blocks,omitempty"`
}

type updateMessageRequest struct {
	Channel string          `json:"channel"`
	TS      string          `json:"ts"`
	Text    string          `json:"text"`
	Blocks  json.RawMessage `json:"blocks,omitempty"`
}

type openConversationRequest struct {
	Users string `json:"users"`
}

type slackChannel struct {
	ID string
}

func (c *slackChannel) UnmarshalJSON(data []byte) error {
	var channelID string
	if err := json.Unmarshal(data, &channelID); err == nil {
		c.ID = channelID
		return nil
	}

	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("decode channel: %w", err)
	}

	c.ID = payload.ID
	return nil
}

type slackAPIResponse struct {
	Channel slackChannel `json:"channel"`
	Error   string       `json:"error,omitempty"`
	TS      string       `json:"ts,omitempty"`
	OK      bool         `json:"ok"`
}

func (r slackAPIResponse) channelID() string {
	return r.Channel.ID
}

func (m *Messenger) postMessage(ctx context.Context, req postMessageRequest) (slackAPIResponse, error) {
	resp, err := m.doJSON(ctx, "chat.postMessage", req)
	if err != nil {
		return slackAPIResponse{}, err
	}

	return resp, nil
}

func (m *Messenger) openConversation(ctx context.Context, userID string) (slackAPIResponse, error) {
	resp, err := m.doJSON(ctx, "conversations.open", openConversationRequest{Users: userID})
	if err != nil {
		return slackAPIResponse{}, err
	}

	return resp, nil
}

func (m *Messenger) doJSON(ctx context.Context, endpoint string, payload any) (slackAPIResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return slackAPIResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	endpointURL, err := url.JoinPath(m.baseURL, endpoint)
	if err != nil {
		return slackAPIResponse{}, fmt.Errorf("build request URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(body))
	if err != nil {
		return slackAPIResponse{}, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+m.botToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return slackAPIResponse{}, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	var apiResp slackAPIResponse
	if err = json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return slackAPIResponse{}, fmt.Errorf("decode response: %w", err)
	}

	if !apiResp.OK {
		return slackAPIResponse{}, fmt.Errorf("slack API %s: %s", endpoint, apiResp.Error)
	}

	return apiResp, nil
}
