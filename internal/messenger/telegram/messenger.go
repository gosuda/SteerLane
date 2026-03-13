package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gosuda/steerlane/internal/messenger"
)

const defaultBaseURL = "https://api.telegram.org"

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

func (m *Messenger) Platform() messenger.Platform { return messenger.PlatformTelegram }

func (m *Messenger) SendMessage(ctx context.Context, params messenger.SendMessageParams) (messenger.MessageResult, error) {
	var out sendMessageResponse
	if err := m.doJSON(ctx, "sendMessage", sendMessageRequest{ChatID: params.ChannelID, Text: params.Text}, &out); err != nil {
		return messenger.MessageResult{}, fmt.Errorf("telegram send message: %w", err)
	}
	messageID := strconv.FormatInt(out.Result.MessageID, 10)
	return messenger.MessageResult{MessageID: messageID, ThreadID: messageID}, nil
}

func (m *Messenger) CreateThread(ctx context.Context, params messenger.CreateThreadParams) (messenger.MessageResult, error) {
	var out sendMessageResponse
	req := sendMessageRequest{ChatID: params.ChannelID, Text: params.Text}
	if params.ParentMessageID != "" {
		if id, err := strconv.ParseInt(params.ParentMessageID, 10, 64); err == nil {
			req.ReplyToMessageID = id
		}
	}
	if len(params.Options) != 0 && params.ActionID != "" {
		req.ReplyMarkup = buildInlineKeyboard(params.ActionID, params.Options)
	}
	if err := m.doJSON(ctx, "sendMessage", req, &out); err != nil {
		return messenger.MessageResult{}, fmt.Errorf("telegram create thread: %w", err)
	}
	messageID := strconv.FormatInt(out.Result.MessageID, 10)
	threadID := params.ParentMessageID
	if threadID == "" {
		threadID = messageID
	}
	return messenger.MessageResult{MessageID: messageID, ThreadID: threadID}, nil
}

func (m *Messenger) UpdateMessage(ctx context.Context, params messenger.UpdateMessageParams) error {
	messageID, err := strconv.ParseInt(params.MessageID, 10, 64)
	if err != nil {
		return fmt.Errorf("telegram update message: %w", err)
	}
	return m.doJSON(ctx, "editMessageText", map[string]any{"chat_id": params.ChannelID, "message_id": messageID, "text": params.Text}, nil)
}

func (m *Messenger) SendNotification(ctx context.Context, params messenger.NotificationParams) error {
	_, err := m.SendMessage(ctx, messenger.SendMessageParams{ChannelID: params.UserExternalID, Text: params.Text})
	return err
}

type sendMessageRequest struct { //nolint:govet // readability over field packing
	ChatID           string `json:"chat_id"`
	ReplyMarkup      any    `json:"reply_markup,omitempty"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
	Text             string `json:"text"`
}

type sendMessageResponse struct {
	OK     bool `json:"ok"`
	Result struct {
		MessageID int64 `json:"message_id"`
	} `json:"result"`
}

func (m *Messenger) doJSON(ctx context.Context, method string, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	endpoint, err := url.JoinPath(m.baseURL, "bot"+m.botToken, method)
	if err != nil {
		return fmt.Errorf("build request URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
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

func buildInlineKeyboard(actionID string, options []messenger.ThreadOption) map[string]any {
	buttons := make([]map[string]any, 0, min(len(options), 5))
	for _, option := range options {
		if option.Label == "" || option.Value == "" {
			continue
		}
		buttons = append(buttons, map[string]any{"text": option.Label, "callback_data": actionID + "|" + url.QueryEscape(option.Value)})
		if len(buttons) == 5 {
			break
		}
	}
	if len(buttons) == 0 {
		return nil
	}
	return map[string]any{"inline_keyboard": [][]map[string]any{buttons}}
}
