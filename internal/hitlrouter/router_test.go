package hitlrouter

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
	agentdom "github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/domain/hitl"
	"github.com/gosuda/steerlane/internal/messenger"
	redispkg "github.com/gosuda/steerlane/internal/store/redis"
	"github.com/gosuda/steerlane/internal/testutil"
)

type recordedAgentEvent struct {
	eventType redispkg.EventType
	payload   any
	sessionID string
}

type fakePublisher struct {
	err    error
	events []recordedAgentEvent
}

func (f *fakePublisher) PublishAgentEvent(_ context.Context, sessionID string, eventType redispkg.EventType, payload any) error {
	f.events = append(f.events, recordedAgentEvent{
		sessionID: sessionID,
		eventType: eventType,
		payload:   payload,
	})
	return f.err
}

type fakeThreadCreator struct {
	result messenger.MessageResult
	err    error
	params []messenger.CreateThreadParams
}

func (f *fakeThreadCreator) CreateThread(_ context.Context, params messenger.CreateThreadParams) (messenger.MessageResult, error) {
	f.params = append(f.params, params)
	if f.err != nil {
		return messenger.MessageResult{}, f.err
	}
	return f.result, nil
}

func (f *fakeThreadCreator) Platform() messenger.Platform { return messenger.PlatformSlack }

func testRouterLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func mustMarshalAskHuman(t *testing.T, input AskHumanInput) json.RawMessage {
	t.Helper()

	data, err := json.Marshal(input)
	require.NoError(t, err)

	return data
}

func TestRouter_HandleAskHuman_Errors(t *testing.T) {
	t.Parallel()

	createErr := errors.New("create failed")
	updateErr := errors.New("status failed")

	//nolint:govet // test case shape prioritizes readability over field packing.
	tests := []struct {
		name            string
		input           json.RawMessage
		createErr       error
		updateErr       error
		wantErr         error
		wantContains    string
		wantCreateCalls int
		wantStatusCalls int
	}{
		{
			name:         "invalid json",
			input:        json.RawMessage(`{"question":`),
			wantContains: "unmarshal input",
		},
		{
			name: "missing question",
			input: mustMarshalAskHuman(t, AskHumanInput{
				Options: json.RawMessage(`["approve"]`),
			}),
			wantErr:      domain.ErrInvalidInput,
			wantContains: "question text required",
		},
		{
			name: "create fails",
			input: mustMarshalAskHuman(t, AskHumanInput{
				Question: "Ship it?",
			}),
			createErr:       createErr,
			wantContains:    "create question",
			wantCreateCalls: 1,
		},
		{
			name: "session status update fails",
			input: mustMarshalAskHuman(t, AskHumanInput{
				Question: "Ship it?",
			}),
			updateErr:       updateErr,
			wantContains:    "update session status",
			wantCreateCalls: 1,
			wantStatusCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			createCalls := 0
			statusCalls := 0
			publisher := &fakePublisher{}
			router := NewRouter(
				testRouterLogger(),
				&testutil.MockHITLRepo{
					CreateFn: func(_ context.Context, _ *hitl.Question) error {
						createCalls++
						return tt.createErr
					},
				},
				&testutil.MockAgentRepo{
					UpdateStatusFn: func(_ context.Context, _ domain.TenantID, _ domain.AgentSessionID, _ agentdom.SessionStatus) error {
						statusCalls++
						return tt.updateErr
					},
				},
				publisher,
			)

			got, err := router.HandleAskHuman(t.Context(), testutil.TestTenantID(), testutil.TestSessionID(), tt.input)

			require.Error(t, err)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			}
			assert.Nil(t, got)
			assert.Contains(t, err.Error(), tt.wantContains)
			assert.Equal(t, tt.wantCreateCalls, createCalls, "question create calls")
			assert.Equal(t, tt.wantStatusCalls, statusCalls, "session status update calls")
			assert.Empty(t, publisher.events, "no events should be published on error")
		})
	}
}

func TestRouter_HandleAskHuman_Success(t *testing.T) {
	t.Parallel()

	var captured *hitl.Question
	var statusUpdates []agentdom.SessionStatus
	publisher := &fakePublisher{}
	router := NewRouter(
		testRouterLogger(),
		&testutil.MockHITLRepo{
			CreateFn: func(_ context.Context, question *hitl.Question) error {
				captured = question
				return nil
			},
		},
		&testutil.MockAgentRepo{
			UpdateStatusFn: func(_ context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID, status agentdom.SessionStatus) error {
				assert.Equal(t, testutil.TestTenantID(), tenantID)
				assert.Equal(t, testutil.TestSessionID(), sessionID)
				statusUpdates = append(statusUpdates, status)
				return nil
			},
		},
		publisher,
	)

	before := time.Now()
	got, err := router.HandleAskHuman(t.Context(), testutil.TestTenantID(), testutil.TestSessionID(), mustMarshalAskHuman(t, AskHumanInput{
		Question: "Should we enable retries?",
		Options:  json.RawMessage(`["yes","no"]`),
	}))
	after := time.Now()

	require.NoError(t, err)
	require.Same(t, captured, got)
	require.NotNil(t, got.TimeoutAt)

	assert.Equal(t, testutil.TestTenantID(), got.TenantID)
	assert.Equal(t, testutil.TestSessionID(), got.AgentSessionID)
	assert.Equal(t, "Should we enable retries?", got.Question)
	assert.Equal(t, hitl.StatusPending, got.Status)
	assert.JSONEq(t, `["yes","no"]`, string(got.Options))
	assert.False(t, got.CreatedAt.Before(before), "created_at should be set during the call")
	assert.False(t, got.CreatedAt.After(after), "created_at should be set during the call")
	assert.WithinDuration(t, got.CreatedAt.Add(DefaultTimeout), *got.TimeoutAt, time.Second)
	assert.Equal(t, []agentdom.SessionStatus{agentdom.StatusWaitingHITL}, statusUpdates)

	require.Len(t, publisher.events, 1)
	event := publisher.events[0]
	assert.Equal(t, testutil.TestSessionID().String(), event.sessionID)
	assert.Equal(t, redispkg.EventAgentStatus, event.eventType)
	payload, ok := event.payload.(map[string]any)
	require.True(t, ok, "payload should be a map")
	assert.Equal(t, testutil.TestSessionID(), payload["session_id"])
	assert.Equal(t, "waiting_hitl", payload["status"])
	assert.Equal(t, got.ID, payload["question_id"])
}

func TestRouter_HandleAskHuman_CreatesMessengerThread(t *testing.T) {
	t.Parallel()

	publisher := &fakePublisher{}
	threader := &fakeThreadCreator{result: messenger.MessageResult{MessageID: "1710000000.000200", ThreadID: "1710000000.000100"}}
	contexts := messenger.NewSessionContextRegistry()
	contexts.Put(testutil.TestTenantID(), testutil.TestSessionID(), messenger.SessionContext{
		Platform:        messenger.PlatformSlack,
		ChannelID:       "C123",
		ParentMessageID: "1710000000.000100",
	})

	router := NewRouter(
		testRouterLogger(),
		&testutil.MockHITLRepo{CreateFn: func(_ context.Context, _ *hitl.Question) error { return nil }},
		&testutil.MockAgentRepo{UpdateStatusFn: func(_ context.Context, _ domain.TenantID, _ domain.AgentSessionID, _ agentdom.SessionStatus) error {
			return nil
		}},
		publisher,
	)
	router.ConfigureThreading(threader, contexts)

	got, err := router.HandleAskHuman(t.Context(), testutil.TestTenantID(), testutil.TestSessionID(), mustMarshalAskHuman(t, AskHumanInput{
		Question: "Should we enable retries?",
		Options:  json.RawMessage(`["yes","no"]`),
	}))

	require.NoError(t, err)
	require.Len(t, threader.params, 1)
	require.Equal(t, "C123", threader.params[0].ChannelID)
	require.Equal(t, "1710000000.000100", threader.params[0].ParentMessageID)
	require.Equal(t, "Should we enable retries?", threader.params[0].Text)
	require.Equal(t, "1710000000.000100", *got.MessengerThreadID)
	require.Equal(t, string(messenger.PlatformSlack), *got.MessengerPlatform)
}

func TestRouter_AnswerQuestion_Errors(t *testing.T) {
	t.Parallel()

	loadErr := errors.New("load failed")
	answerErr := errors.New("answer failed")

	tests := []struct {
		name            string
		question        *hitl.Question
		getErr          error
		answerErr       error
		wantErr         error
		wantContains    string
		wantAnswerCalls int
	}{
		{
			name:         "question load fails",
			getErr:       loadErr,
			wantContains: "get question",
		},
		{
			name: "question is not pending",
			question: &hitl.Question{
				ID:             domain.NewID(),
				TenantID:       testutil.TestTenantID(),
				AgentSessionID: testutil.TestSessionID(),
				Question:       "Ship it?",
				Status:         hitl.StatusAnswered,
			},
			wantErr:      domain.ErrInvalidInput,
			wantContains: "question not in pending status",
		},
		{
			name: "answer persistence fails",
			question: &hitl.Question{
				ID:             domain.NewID(),
				TenantID:       testutil.TestTenantID(),
				AgentSessionID: testutil.TestSessionID(),
				Question:       "Ship it?",
				Status:         hitl.StatusPending,
			},
			answerErr:       answerErr,
			wantContains:    "answer question",
			wantAnswerCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			answerCalls := 0
			publisher := &fakePublisher{}
			router := NewRouter(
				testRouterLogger(),
				&testutil.MockHITLRepo{
					GetByIDFn: func(_ context.Context, _ domain.TenantID, _ domain.HITLQuestionID) (*hitl.Question, error) {
						if tt.getErr != nil {
							return nil, tt.getErr
						}
						return tt.question, nil
					},
					AnswerFn: func(_ context.Context, _ domain.TenantID, _ domain.HITLQuestionID, _ string, _ domain.UserID) error {
						answerCalls++
						return tt.answerErr
					},
				},
				&testutil.MockAgentRepo{
					UpdateStatusFn: func(_ context.Context, _ domain.TenantID, _ domain.AgentSessionID, _ agentdom.SessionStatus) error {
						return nil
					},
				},
				publisher,
			)

			err := router.AnswerQuestion(t.Context(), testutil.TestTenantID(), domain.NewID(), "yes", testutil.TestUserID())

			require.Error(t, err)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			}
			assert.Contains(t, err.Error(), tt.wantContains)
			assert.Equal(t, tt.wantAnswerCalls, answerCalls, "answer calls")
			assert.Empty(t, publisher.events, "no events should be published on error")
		})
	}
}

func TestRouter_AnswerQuestion_Success(t *testing.T) {
	t.Parallel()

	questionID := domain.NewID()
	question := &hitl.Question{
		ID:             questionID,
		TenantID:       testutil.TestTenantID(),
		AgentSessionID: testutil.TestSessionID(),
		Question:       "Ship it?",
		Status:         hitl.StatusPending,
	}

	var answered struct {
		answer     string
		answeredBy domain.UserID
		questionID domain.HITLQuestionID
	}
	publisher := &fakePublisher{}
	router := NewRouter(
		testRouterLogger(),
		&testutil.MockHITLRepo{
			GetByIDFn: func(_ context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) (*hitl.Question, error) {
				assert.Equal(t, testutil.TestTenantID(), tenantID)
				assert.Equal(t, questionID, id)
				return question, nil
			},
			AnswerFn: func(_ context.Context, tenantID domain.TenantID, id domain.HITLQuestionID, answer string, answeredBy domain.UserID) error {
				assert.Equal(t, testutil.TestTenantID(), tenantID)
				answered.answer = answer
				answered.answeredBy = answeredBy
				answered.questionID = id
				return nil
			},
		},
		&testutil.MockAgentRepo{
			UpdateStatusFn: func(_ context.Context, _ domain.TenantID, _ domain.AgentSessionID, _ agentdom.SessionStatus) error {
				return nil
			},
		},
		publisher,
	)

	err := router.AnswerQuestion(t.Context(), testutil.TestTenantID(), questionID, "yes", testutil.TestUserID())

	require.NoError(t, err)
	assert.Equal(t, "yes", answered.answer)
	assert.Equal(t, testutil.TestUserID(), answered.answeredBy)
	assert.Equal(t, questionID, answered.questionID)
	assert.Empty(t, publisher.events)
}

func TestRouter_AnswerByThread(t *testing.T) {
	t.Parallel()

	questionID := domain.NewID()
	router := NewRouter(
		testRouterLogger(),
		&testutil.MockHITLRepo{
			GetByThreadFn: func(_ context.Context, tenantID domain.TenantID, platform, threadID string) (*hitl.Question, error) {
				assert.Equal(t, testutil.TestTenantID(), tenantID)
				assert.Equal(t, string(messenger.PlatformSlack), platform)
				assert.Equal(t, "1710000000.000100", threadID)
				return &hitl.Question{
					ID:             questionID,
					TenantID:       tenantID,
					AgentSessionID: testutil.TestSessionID(),
					Status:         hitl.StatusPending,
					Question:       "Ship it?",
				}, nil
			},
			GetByIDFn: func(_ context.Context, _ domain.TenantID, id domain.HITLQuestionID) (*hitl.Question, error) {
				return &hitl.Question{ID: id, TenantID: testutil.TestTenantID(), AgentSessionID: testutil.TestSessionID(), Status: hitl.StatusPending, Question: "Ship it?"}, nil
			},
			AnswerFn: func(_ context.Context, _ domain.TenantID, id domain.HITLQuestionID, answer string, answeredBy domain.UserID) error {
				assert.Equal(t, questionID, id)
				assert.Equal(t, "yes", answer)
				assert.Equal(t, testutil.TestUserID(), answeredBy)
				return nil
			},
		},
		&testutil.MockAgentRepo{UpdateStatusFn: func(_ context.Context, _ domain.TenantID, _ domain.AgentSessionID, _ agentdom.SessionStatus) error {
			return nil
		}},
		&fakePublisher{},
	)

	question, err := router.AnswerByThread(t.Context(), testutil.TestTenantID(), string(messenger.PlatformSlack), "1710000000.000100", "yes", testutil.TestUserID())
	require.NoError(t, err)
	require.Equal(t, questionID, question.ID)
	require.NotNil(t, question.Answer)
	require.Equal(t, "yes", *question.Answer)
}
