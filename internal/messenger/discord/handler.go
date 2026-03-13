package discord

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

const maxRequestBodySize = 256 * 1024

type InteractionProcessor interface {
	HandleInteraction(ctx context.Context, interaction Interaction) error
}

type Handler struct { //nolint:govet // readability over field packing
	logger    *slog.Logger
	publicKey ed25519.PublicKey
	processor InteractionProcessor
}

func NewHandler(logger *slog.Logger, publicKeyHex string, processor InteractionProcessor) (*Handler, error) {
	if logger == nil {
		logger = slog.Default()
	}
	publicKey, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return nil, fmt.Errorf("discord: decode public key: %w", err)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, errors.New("discord: invalid public key length")
	}

	return &Handler{logger: logger.With("component", "discord.handler"), publicKey: ed25519.PublicKey(publicKey), processor: processor}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodySize))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !h.verify(r.Header.Get("X-Signature-Ed25519"), r.Header.Get("X-Signature-Timestamp"), body) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var interaction Interaction
	unmarshalErr := json.Unmarshal(body, &interaction)
	if unmarshalErr != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if interaction.Type == interactionTypePing {
		_ = json.NewEncoder(w).Encode(interactionResponse{Type: interactionResponsePong})
		return
	}

	_ = json.NewEncoder(w).Encode(interactionResponse{Type: interactionResponseDeferred})
	if h.processor != nil {
		processErr := h.processor.HandleInteraction(r.Context(), interaction)
		if processErr != nil {
			h.logger.ErrorContext(r.Context(), "discord interaction handler error", "error", processErr)
		}
	}
}

func (h *Handler) verify(signatureHex, timestamp string, body []byte) bool {
	if signatureHex == "" || timestamp == "" {
		return false
	}
	signature, err := hex.DecodeString(signatureHex)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return false
	}
	msg := append([]byte(timestamp), body...)
	return ed25519.Verify(h.publicKey, msg, signature)
}
