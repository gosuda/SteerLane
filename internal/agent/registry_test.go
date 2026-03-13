package agent

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainagent "github.com/gosuda/steerlane/internal/domain/agent"
)

type stubBackend struct{}

func (stubBackend) StartSession(context.Context, SessionOpts) error { return nil }
func (stubBackend) SendPrompt(context.Context, string) error        { return nil }
func (stubBackend) Cancel(context.Context) error                    { return nil }
func (stubBackend) OnMessage(MessageHandler)                        {}
func (stubBackend) Dispose() error                                  { return nil }

func testRegistryLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRegistry_Create(t *testing.T) {
	t.Parallel()

	factoryErr := errors.New("factory failed")

	//nolint:govet // test case shape prioritizes readability over field packing.
	tests := []struct {
		name         string
		agentType    domainagent.AgentType
		register     bool
		factoryErr   error
		wantErr      error
		wantContains string
	}{
		{
			name:      "registered backend is created",
			agentType: domainagent.TypeClaude,
			register:  true,
		},
		{
			name:         "missing backend returns error",
			agentType:    domainagent.TypeCodex,
			wantContains: "no backend registered",
		},
		{
			name:         "factory error is returned",
			agentType:    domainagent.TypeGemini,
			register:     true,
			factoryErr:   factoryErr,
			wantErr:      factoryErr,
			wantContains: "factory failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			registry := NewRegistry()
			if tt.register {
				registry.Register(tt.agentType, func(_ *slog.Logger) (Backend, error) {
					if tt.factoryErr != nil {
						return nil, tt.factoryErr
					}
					return stubBackend{}, nil
				})
			}

			got, err := registry.Create(tt.agentType, testRegistryLogger())

			if tt.wantContains != "" {
				require.Error(t, err)
				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)
				}
				assert.Nil(t, got)
				assert.Contains(t, err.Error(), tt.wantContains)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, got)
		})
	}
}

func TestRegistry_RegisterDuplicatePanics(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	registry.Register(domainagent.TypeClaude, func(_ *slog.Logger) (Backend, error) {
		return stubBackend{}, nil
	})

	require.PanicsWithValue(t, `agent: factory already registered for type "claude"`, func() {
		registry.Register(domainagent.TypeClaude, func(_ *slog.Logger) (Backend, error) {
			return stubBackend{}, nil
		})
	})
}

func TestRegistry_TypesAndHas(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	registry.Register(domainagent.TypeClaude, func(_ *slog.Logger) (Backend, error) {
		return stubBackend{}, nil
	})
	registry.Register(domainagent.TypeGemini, func(_ *slog.Logger) (Backend, error) {
		return stubBackend{}, nil
	})

	assert.True(t, registry.Has(domainagent.TypeClaude))
	assert.True(t, registry.Has(domainagent.TypeGemini))
	assert.False(t, registry.Has(domainagent.TypeOpenCode))
	assert.ElementsMatch(t, []domainagent.AgentType{domainagent.TypeClaude, domainagent.TypeGemini}, registry.Types())
}
