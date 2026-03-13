package adrengine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/adr"
	"github.com/gosuda/steerlane/internal/testutil"
)

func testADRLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func mustMarshalADRInput(t *testing.T, input CreateADRInput) json.RawMessage {
	t.Helper()

	data, err := json.Marshal(input)
	require.NoError(t, err)

	return data
}

func TestEngine_HandleCreateADR_Validation(t *testing.T) {
	t.Parallel()

	//nolint:govet // test case shape prioritizes readability over field packing.
	tests := []struct {
		name         string
		input        json.RawMessage
		wantErr      error
		wantContains string
	}{
		{
			name:         "invalid json",
			input:        json.RawMessage(`{"title":`),
			wantContains: "parse input",
		},
		{
			name: "missing title",
			input: mustMarshalADRInput(t, CreateADRInput{
				Context:  "Need a pubsub layer",
				Decision: "Use Redis",
			}),
			wantErr:      domain.ErrInvalidInput,
			wantContains: "title is required",
		},
		{
			name: "missing context",
			input: mustMarshalADRInput(t, CreateADRInput{
				Title:    "Use Redis pubsub",
				Decision: "Use Redis",
			}),
			wantErr:      domain.ErrInvalidInput,
			wantContains: "context is required",
		},
		{
			name: "missing decision",
			input: mustMarshalADRInput(t, CreateADRInput{
				Title:   "Use Redis pubsub",
				Context: "Need realtime events",
			}),
			wantErr:      domain.ErrInvalidInput,
			wantContains: "decision is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			called := false
			engine := NewEngine(testADRLogger(), &testutil.MockADRRepo{
				CreateWithNextSequenceFn: func(_ context.Context, _ *adr.ADR) error {
					called = true
					return nil
				},
			}, nil)

			got, err := engine.HandleCreateADR(t.Context(), testutil.TestTenantID(), testutil.TestProjectID(), testutil.TestSessionID(), tt.input)

			require.Error(t, err)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			}
			assert.Nil(t, got)
			assert.Contains(t, err.Error(), tt.wantContains)
			assert.False(t, called, "repository should not be called for invalid input")
		})
	}
}

func TestEngine_HandleCreateADR_CreateError(t *testing.T) {
	t.Parallel()

	boom := errors.New("insert failed")
	engine := NewEngine(testADRLogger(), &testutil.MockADRRepo{
		CreateWithNextSequenceFn: func(_ context.Context, _ *adr.ADR) error {
			return boom
		},
	}, nil)

	got, err := engine.HandleCreateADR(
		t.Context(),
		testutil.TestTenantID(),
		testutil.TestProjectID(),
		testutil.TestSessionID(),
		mustMarshalADRInput(t, CreateADRInput{
			Title:    "Use Redis pubsub",
			Context:  "Need realtime updates",
			Decision: "Use Redis",
		}),
	)

	require.Error(t, err)
	require.ErrorIs(t, err, boom)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "adrengine.HandleCreateADR")
}

func TestEngine_HandleCreateADR_Success(t *testing.T) {
	t.Parallel()

	var captured *adr.ADR
	engine := NewEngine(testADRLogger(), &testutil.MockADRRepo{
		CreateWithNextSequenceFn: func(_ context.Context, record *adr.ADR) error {
			captured = record
			record.Sequence = 7
			return nil
		},
	}, nil)

	input := CreateADRInput{
		Title:    "Use Redis pubsub",
		Context:  "The board needs realtime updates across replicas.",
		Decision: "Adopt Redis pubsub for fan-out.",
		Consequences: adr.Consequences{
			Good:    []string{"Cross-node fan-out"},
			Bad:     []string{"Operational dependency"},
			Neutral: []string{"Eventual delivery semantics"},
		},
		Options: json.RawMessage(`[{"name":"postgres listen/notify"}]`),
		Drivers: []string{"horizontal scaling", "websocket fan-out"},
	}

	before := time.Now()
	got, err := engine.HandleCreateADR(
		t.Context(),
		testutil.TestTenantID(),
		testutil.TestProjectID(),
		testutil.TestSessionID(),
		mustMarshalADRInput(t, input),
	)
	after := time.Now()

	require.NoError(t, err)
	require.Same(t, captured, got)
	require.NotNil(t, got.AgentSessionID)

	assert.Equal(t, testutil.TestTenantID(), got.TenantID)
	assert.Equal(t, testutil.TestProjectID(), got.ProjectID)
	assert.Equal(t, testutil.TestSessionID(), *got.AgentSessionID)
	assert.Equal(t, input.Title, got.Title)
	assert.Equal(t, input.Context, got.Context)
	assert.Equal(t, input.Decision, got.Decision)
	assert.Equal(t, adr.StatusProposed, got.Status)
	assert.Equal(t, input.Consequences, got.Consequences)
	assert.Equal(t, input.Drivers, got.Drivers)
	assert.JSONEq(t, string(input.Options), string(got.Options))
	assert.Equal(t, 7, got.Sequence)
	assert.WithinDuration(t, got.CreatedAt, got.UpdatedAt, time.Millisecond)
	assert.False(t, got.CreatedAt.Before(before), "created_at should be set during the call")
	assert.False(t, got.CreatedAt.After(after), "created_at should be set during the call")
	require.NoError(t, got.Validate())
}
