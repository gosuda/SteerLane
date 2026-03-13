package telegram

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

const maxRequestBodySize = 256 * 1024

type UpdateProcessor interface {
	HandleUpdate(ctx context.Context, update Update) error
}

type Handler struct {
	logger      *slog.Logger
	processor   UpdateProcessor
	secretToken string
}

func NewHandler(logger *slog.Logger, secretToken string, processor UpdateProcessor) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{logger: logger.With("component", "telegram.handler"), secretToken: secretToken, processor: processor}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.secretToken != "" && r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != h.secretToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodySize))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var update Update
	unmarshalErr := json.Unmarshal(body, &update)
	if unmarshalErr != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	if h.processor != nil {
		processErr := h.processor.HandleUpdate(r.Context(), update)
		if processErr != nil {
			h.logger.ErrorContext(r.Context(), "telegram update handler error", "error", processErr)
		}
	}
}
