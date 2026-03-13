package discord

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

const defaultBaseURL = "https://discord.com/api/v10"

type Messenger struct {
	httpClient *http.Client
	baseURL    string
	botToken   string
}

var _ messenger.Messenger = (*Messenger)(nil)

type Option func(*Messenger)

func WithBaseURL(baseURL string) Option {
	return func(m *Messenger) {
		trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
		if trimmed != "" {
			m.baseURL = trimmed
		}
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(m *Messenger) {
		if client != nil {
			m.httpClient = client
		}
	}
}

func NewMessenger(botToken string, opts ...Option) *Messenger {
	m := &Messenger{baseURL: defaultBaseURL, botToken: botToken, httpClient: http.DefaultClient}
	for _, opt := range opts {
		if opt != nil {
			opt(m)
		}
	}
	return m
}

func (m *Messenger) Platform() messenger.Platform { return messenger.PlatformDiscord }

func (m *Messenger) SendMessage(ctx context.Context, params messenger.SendMessageParams) (messenger.MessageResult, error) {
	resp, err := m.createMessage(ctx, params.ChannelID, createMessageRequest{Content: params.Text})
	if err != nil {
		return messenger.MessageResult{}, fmt.Errorf("discord send message: %w", err)
	}
	return messenger.MessageResult{MessageID: resp.ID, ThreadID: resp.ID}, nil
}

func (m *Messenger) CreateThread(ctx context.Context, params messenger.CreateThreadParams) (messenger.MessageResult, error) {
	req := createMessageRequest{Content: params.Text}
	if params.ParentMessageID != "" {
		req.MessageReference = &messageReference{MessageID: params.ParentMessageID}
	}
	if len(params.Options) != 0 && params.ActionID != "" {
		req.Components = buildComponents(params.ActionID, params.Options)
	}
	resp, err := m.createMessage(ctx, params.ChannelID, req)
	if err != nil {
		return messenger.MessageResult{}, fmt.Errorf("discord create thread: %w", err)
	}
	threadID := params.ParentMessageID
	if threadID == "" {
		threadID = resp.ID
	}
	return messenger.MessageResult{MessageID: resp.ID, ThreadID: threadID}, nil
}

func (m *Messenger) UpdateMessage(ctx context.Context, params messenger.UpdateMessageParams) error {
	endpoint, err := url.JoinPath(m.baseURL, "channels", params.ChannelID, "messages", params.MessageID)
	if err != nil {
		return fmt.Errorf("discord update message: %w", err)
	}
	return m.doJSON(ctx, http.MethodPatch, endpoint, createMessageRequest{Content: params.Text}, nil)
}

func (m *Messenger) SendNotification(ctx context.Context, params messenger.NotificationParams) error {
	var dm struct {
		ID string `json:"id"`
	}
	endpoint, err := url.JoinPath(m.baseURL, "users", "@me", "channels")
	if err != nil {
		return fmt.Errorf("discord send notification: %w", err)
	}
	dmErr := m.doJSON(ctx, http.MethodPost, endpoint, map[string]string{"recipient_id": params.UserExternalID}, &dm)
	if dmErr != nil {
		return fmt.Errorf("discord send notification: open dm: %w", dmErr)
	}
	_, err = m.SendMessage(ctx, messenger.SendMessageParams{ChannelID: dm.ID, Text: params.Text})
	if err != nil {
		return fmt.Errorf("discord send notification: %w", err)
	}
	return nil
}

type createMessageRequest struct { //nolint:govet // readability over field packing
	Content          string            `json:"content"`
	Components       []component       `json:"components,omitempty"`
	MessageReference *messageReference `json:"message_reference,omitempty"`
}

type component struct { //nolint:govet // readability over field packing
	Type       int               `json:"type"`
	Components []componentButton `json:"components"`
}

type componentButton struct { //nolint:govet // readability over field packing
	Type     int    `json:"type"`
	Style    int    `json:"style"`
	CustomID string `json:"custom_id"`
	Label    string `json:"label"`
}

type messageReference struct {
	MessageID string `json:"message_id"`
}

type discordMessage struct {
	ID string `json:"id"`
}

func (m *Messenger) createMessage(ctx context.Context, channelID string, payload createMessageRequest) (discordMessage, error) {
	endpoint, err := url.JoinPath(m.baseURL, "channels", channelID, "messages")
	if err != nil {
		return discordMessage{}, err
	}
	var out discordMessage
	requestErr := m.doJSON(ctx, http.MethodPost, endpoint, payload, &out)
	if requestErr != nil {
		return discordMessage{}, requestErr
	}
	return out, nil
}

func (m *Messenger) doJSON(ctx context.Context, method, endpoint string, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bot "+m.botToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	decodeErr := json.NewDecoder(resp.Body).Decode(out)
	if decodeErr != nil {
		return fmt.Errorf("decode response: %w", decodeErr)
	}
	return nil
}

func buildComponents(actionID string, options []messenger.ThreadOption) []component {
	buttons := make([]componentButton, 0, min(len(options), 5))
	for _, option := range options {
		if option.Label == "" || option.Value == "" {
			continue
		}
		buttons = append(buttons, componentButton{Type: 2, Style: 1, Label: option.Label, CustomID: actionID + "|" + url.QueryEscape(option.Value)})
		if len(buttons) == 5 {
			break
		}
	}
	if len(buttons) == 0 {
		return nil
	}
	return []component{{Type: 1, Components: buttons}}
}
