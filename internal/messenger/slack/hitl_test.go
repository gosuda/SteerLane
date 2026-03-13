package slack

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/hitl"
	"github.com/gosuda/steerlane/internal/testutil"
)

type mockHITLAnswerer struct {
	answerFn func(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID, answer string, answeredBy domain.UserID) error
	getFn    func(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID) (*hitl.Question, error)
	resetFn  func(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID) error
}

func (m *mockHITLAnswerer) AnswerQuestion(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID, answer string, answeredBy domain.UserID) error {
	if m.answerFn != nil {
		return m.answerFn(ctx, tenantID, questionID, answer, answeredBy)
	}
	return nil
}

func (m *mockHITLAnswerer) GetQuestion(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID) (*hitl.Question, error) {
	if m.getFn != nil {
		return m.getFn(ctx, tenantID, questionID)
	}
	return &hitl.Question{ID: questionID, AgentSessionID: testutil.TestSessionID()}, nil
}

func (m *mockHITLAnswerer) ResetQuestion(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID) error {
	if m.resetFn != nil {
		return m.resetFn(ctx, tenantID, questionID)
	}
	return nil
}

func TestParseHITLAction(t *testing.T) {
	t.Parallel()

	t.Run("parses valid action", func(t *testing.T) {
		t.Parallel()

		actionID := FormatActionID(testutil.TestTaskID(), testutil.TestSessionID())
		got, err := ParseHITLAction(actionID, "Ship it")
		require.NoError(t, err)
		require.Equal(t, testutil.TestTaskID(), got.QuestionID)
		require.Equal(t, testutil.TestSessionID(), got.SessionID)
		require.Equal(t, "Ship it", got.Answer)
	})

	t.Run("rejects invalid prefix", func(t *testing.T) {
		t.Parallel()

		_, err := ParseHITLAction("other:hitl:bad:value", "Yes")
		require.Error(t, err)
	})

	t.Run("rejects empty answer", func(t *testing.T) {
		t.Parallel()

		actionID := FormatActionID(testutil.TestTaskID(), testutil.TestSessionID())
		_, err := ParseHITLAction(actionID, "")
		require.Error(t, err)
	})
}

func TestHITLInteractionHandler_RollsBackWhenResumeFails(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolved := ResolvedContext{TenantID: testutil.TestTenantID(), UserID: testutil.TestUserID()}
	questionID := testutil.TestTaskID()
	sessionID := testutil.TestSessionID()
	resetCalls := 0
	handler := NewHITLInteractionHandler(
		logger,
		&mockHITLAnswerer{
			answerFn: func(_ context.Context, tenantID domain.TenantID, gotQuestionID domain.HITLQuestionID, answer string, answeredBy domain.UserID) error {
				require.Equal(t, resolved.TenantID, tenantID)
				require.Equal(t, questionID, gotQuestionID)
				require.Equal(t, "Ship it", answer)
				require.Equal(t, resolved.UserID, answeredBy)
				return nil
			},
			resetFn: func(_ context.Context, tenantID domain.TenantID, gotQuestionID domain.HITLQuestionID) error {
				require.Equal(t, resolved.TenantID, tenantID)
				require.Equal(t, questionID, gotQuestionID)
				resetCalls++
				return nil
			},
		},
		&mockResumer{resumeFn: func(context.Context, domain.TenantID, domain.AgentSessionID, string) error {
			return errors.New("resume failed")
		}},
		&mockContextResolver{resolveContextFn: func(context.Context, string, string, string) (ResolvedContext, error) {
			return resolved, nil
		}},
		&mockSender{},
	)

	err := handler.HandleInteraction(t.Context(), InteractionPayload{
		Team:    InteractionTeam{ID: "T123"},
		Channel: &InteractionChannel{ID: "C123"},
		User:    InteractionUser{ID: "U123"},
		Actions: []InteractionAction{{
			ActionID: FormatActionID(questionID, sessionID),
			Value:    "Ship it",
		}},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "resume session")
	require.Equal(t, 1, resetCalls)
}
