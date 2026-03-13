package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
	agentdom "github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/domain/hitl"
	"github.com/gosuda/steerlane/internal/server/reqctx"
)

// ---------------------------------------------------------------------------
// Agent Session types
// ---------------------------------------------------------------------------

// AgentSessionResponse represents a single agent session.
type AgentSessionResponse struct {
	Body struct {
		CreatedAt   time.Time              `json:"created_at"`
		CompletedAt *time.Time             `json:"completed_at,omitempty"`
		StartedAt   *time.Time             `json:"started_at,omitempty"`
		BranchName  *string                `json:"branch_name,omitempty"`
		Error       *string                `json:"error,omitempty"`
		Status      agentdom.SessionStatus `json:"status"`
		AgentType   agentdom.AgentType     `json:"agent_type"`
		ID          domain.AgentSessionID  `json:"id"`
		ProjectID   domain.ProjectID       `json:"project_id"`
		TaskID      domain.TaskID          `json:"task_id"`
	}
}

// AgentSessionListResponse represents a session list response.
type AgentSessionListResponse struct {
	Body struct {
		Items []*AgentSessionResponse `json:"items"`
	}
}

// AgentSessionPathRequest targets a session by ID.
type AgentSessionPathRequest struct {
	ID domain.AgentSessionID `path:"id"`
}

// AgentSessionEventResponse represents a persisted replayable session event.
type AgentSessionEventResponse struct {
	Body struct { //nolint:govet // JSON response layout is chosen for API clarity over field packing.
		Timestamp time.Time       `json:"timestamp"`
		Payload   json.RawMessage `json:"payload"`
		Type      string          `json:"type"`
		ID        uuid.UUID       `json:"id"`
	}
}

// AgentSessionEventListResponse represents a paginated replay event list.
type AgentSessionEventListResponse struct {
	Body struct {
		NextCursor *uuid.UUID                   `json:"next_cursor,omitempty"`
		Items      []*AgentSessionEventResponse `json:"items"`
	}
}

// AgentSessionEventListRequest queries persisted events for a session.
type AgentSessionEventListRequest struct {
	PaginationRequest
	ID domain.AgentSessionID `path:"id"`
}

// AgentSessionListRequest queries sessions for a project.
type AgentSessionListRequest struct {
	PaginationRequest
	ProjectID domain.ProjectID `query:"project_id" required:"true"`
}

// TaskSessionsRequest queries sessions for a task.
type TaskSessionsRequest struct {
	PaginationRequest
	TaskID domain.TaskID `path:"task_id"`
}

// DispatchTaskRequest is the payload for dispatching a task to an agent.
type DispatchTaskRequest struct {
	Body struct {
		Prompt    string             `json:"prompt" required:"true" doc:"Instruction for the agent"`
		AgentType agentdom.AgentType `json:"agent_type" required:"true" doc:"Agent type (claude, codex, etc.)"`
		TaskID    domain.TaskID      `json:"task_id" required:"true" doc:"Task to dispatch"`
	}
}

// TaskDispatchRequest is the payload for dispatching a task-scoped route.
type TaskDispatchRequest struct {
	Body struct {
		Prompt    string             `json:"prompt" required:"true" doc:"Instruction for the agent"`
		AgentType agentdom.AgentType `json:"agent_type" required:"true" doc:"Agent type (claude, codex, etc.)"`
	}
	TaskID domain.TaskID `path:"task_id"`
}

// DispatchTaskResponse returns the created session ID.
type DispatchTaskResponse struct {
	Body struct {
		SessionID domain.AgentSessionID `json:"session_id"`
	}
}

// CancelSessionRequest cancels a running session.
type CancelSessionRequest struct {
	ID domain.AgentSessionID `path:"id"`
}

// ---------------------------------------------------------------------------
// HITL types
// ---------------------------------------------------------------------------

// HITLQuestionResponse represents a HITL question.
type HITLQuestionResponse struct {
	//nolint:govet // JSON response layout is chosen for API clarity over field packing.
	Body struct {
		Question       string                `json:"question"`
		Options        json.RawMessage       `json:"options,omitempty"`
		Answer         *string               `json:"answer,omitempty"`
		AnsweredBy     *domain.UserID        `json:"answered_by,omitempty"`
		AnsweredAt     *time.Time            `json:"answered_at,omitempty"`
		TimeoutAt      *time.Time            `json:"timeout_at,omitempty"`
		CreatedAt      time.Time             `json:"created_at"`
		Status         hitl.QuestionStatus   `json:"status"`
		ID             domain.HITLQuestionID `json:"id"`
		AgentSessionID domain.AgentSessionID `json:"agent_session_id"`
	}
}

// HITLQuestionListResponse represents a HITL question list response.
type HITLQuestionListResponse struct {
	Body struct {
		Items []*HITLQuestionResponse `json:"items"`
	}
}

// HITLListRequest lists questions for a session.
type HITLListRequest struct {
	SessionID domain.AgentSessionID `query:"session_id" required:"true"`
}

// SessionQuestionsRequest lists questions for a session via path-based routing.
type SessionQuestionsRequest struct {
	ID domain.AgentSessionID `path:"id"`
}

// HITLQuestionPathRequest targets a HITL question by ID.
type HITLQuestionPathRequest struct {
	ID domain.HITLQuestionID `path:"id"`
}

// HITLAnswerRequest answers a HITL question.
type HITLAnswerRequest struct {
	Body struct {
		Answer string `json:"answer" required:"true" doc:"Human answer to the question"`
	}
	ID domain.HITLQuestionID `path:"id"`
}

// ---------------------------------------------------------------------------
// Mappers
// ---------------------------------------------------------------------------

func mapSession(s *agentdom.Session) *AgentSessionResponse {
	resp := &AgentSessionResponse{}
	resp.Body.ID = s.ID
	resp.Body.ProjectID = s.ProjectID
	resp.Body.TaskID = s.TaskID
	resp.Body.Status = s.Status
	resp.Body.AgentType = s.AgentType
	resp.Body.CreatedAt = s.CreatedAt
	resp.Body.StartedAt = s.StartedAt
	resp.Body.CompletedAt = s.CompletedAt
	resp.Body.BranchName = s.BranchName
	resp.Body.Error = s.Error
	return resp
}

func mapQuestion(q *hitl.Question) *HITLQuestionResponse {
	resp := &HITLQuestionResponse{}
	resp.Body.ID = q.ID
	resp.Body.AgentSessionID = q.AgentSessionID
	resp.Body.Question = q.Question
	resp.Body.Status = q.Status
	resp.Body.Options = q.Options
	resp.Body.CreatedAt = q.CreatedAt
	resp.Body.AnsweredAt = q.AnsweredAt
	resp.Body.TimeoutAt = q.TimeoutAt
	resp.Body.Answer = q.Answer
	resp.Body.AnsweredBy = q.AnsweredBy
	return resp
}

func mapSessionEvent(event *agentdom.Event) *AgentSessionEventResponse {
	resp := &AgentSessionEventResponse{}
	resp.Body.ID = event.ID
	resp.Body.Type = event.Type
	resp.Body.Payload = event.Payload
	resp.Body.Timestamp = event.CreatedAt
	return resp
}

func mapSessionList(sessions []*agentdom.Session) *AgentSessionListResponse {
	resp := &AgentSessionListResponse{}
	for _, session := range sessions {
		resp.Body.Items = append(resp.Body.Items, mapSession(session))
	}
	return resp
}

func mapQuestionList(questions []*hitl.Question) *HITLQuestionListResponse {
	resp := &HITLQuestionListResponse{}
	for _, question := range questions {
		resp.Body.Items = append(resp.Body.Items, mapQuestion(question))
	}
	return resp
}

func (a *API) listSessionsByProject(ctx context.Context, projectID domain.ProjectID) (*AgentSessionListResponse, error) {
	if a.deps.AgentSessions == nil {
		return nil, huma.Error501NotImplemented("agent sessions not configured")
	}

	tenantID := reqctx.TenantIDFrom(ctx)
	sessions, err := a.deps.AgentSessions.ListByProject(ctx, tenantID, projectID)
	if err != nil {
		status, model := MapDomainError(err)
		return nil, huma.NewError(status, model.Detail, err)
	}

	return mapSessionList(sessions), nil
}

func (a *API) listSessionsByTask(ctx context.Context, taskID domain.TaskID) (*AgentSessionListResponse, error) {
	if a.deps.AgentSessions == nil {
		return nil, huma.Error501NotImplemented("agent sessions not configured")
	}

	tenantID := reqctx.TenantIDFrom(ctx)
	sessions, err := a.deps.AgentSessions.ListByTask(ctx, tenantID, taskID)
	if err != nil {
		status, model := MapDomainError(err)
		return nil, huma.NewError(status, model.Detail, err)
	}

	return mapSessionList(sessions), nil
}

func (a *API) dispatchTask(ctx context.Context, taskID domain.TaskID, agentType agentdom.AgentType, prompt string) (*DispatchTaskResponse, error) {
	if a.deps.Orchestrator == nil {
		return nil, huma.Error501NotImplemented("orchestrator not configured")
	}

	tenantID := reqctx.TenantIDFrom(ctx)
	sessionID, err := a.deps.Orchestrator.DispatchTask(ctx, tenantID, taskID, agentType, prompt)
	if err != nil {
		status, model := MapDomainError(err)
		return nil, huma.NewError(status, model.Detail, err)
	}

	resp := &DispatchTaskResponse{}
	resp.Body.SessionID = sessionID
	return resp, nil
}

func (a *API) cancelSession(ctx context.Context, sessionID domain.AgentSessionID) (*EmptyResponse, error) {
	if a.deps.Orchestrator == nil {
		return nil, huma.Error501NotImplemented("orchestrator not configured")
	}

	tenantID := reqctx.TenantIDFrom(ctx)
	if err := a.deps.Orchestrator.CancelSession(ctx, tenantID, sessionID); err != nil {
		status, model := MapDomainError(err)
		return nil, huma.NewError(status, model.Detail, err)
	}
	return &EmptyResponse{}, nil
}

func (a *API) listSessionQuestions(ctx context.Context, sessionID domain.AgentSessionID) (*HITLQuestionListResponse, error) {
	if a.deps.HITLRouter == nil {
		return nil, huma.Error501NotImplemented("HITL router not configured")
	}

	tenantID := reqctx.TenantIDFrom(ctx)
	questions, err := a.deps.HITLRouter.ListBySession(ctx, tenantID, sessionID)
	if err != nil {
		status, model := MapDomainError(err)
		return nil, huma.NewError(status, model.Detail, err)
	}

	return mapQuestionList(questions), nil
}

func (a *API) getQuestion(ctx context.Context, questionID domain.HITLQuestionID) (*HITLQuestionResponse, error) {
	if a.deps.HITLRouter == nil {
		return nil, huma.Error501NotImplemented("HITL router not configured")
	}

	tenantID := reqctx.TenantIDFrom(ctx)
	question, err := a.deps.HITLRouter.GetQuestion(ctx, tenantID, questionID)
	if err != nil {
		status, model := MapDomainError(err)
		return nil, huma.NewError(status, model.Detail, err)
	}
	return mapQuestion(question), nil
}

func (a *API) answerQuestion(ctx context.Context, questionID domain.HITLQuestionID, answer string) (*EmptyResponse, error) {
	if a.deps.HITLRouter == nil {
		return nil, huma.Error501NotImplemented("HITL router not configured")
	}

	tenantID := reqctx.TenantIDFrom(ctx)
	userID := reqctx.UserIDFrom(ctx)

	if err := a.deps.HITLRouter.AnswerQuestion(ctx, tenantID, questionID, answer, userID); err != nil {
		status, model := MapDomainError(err)
		return nil, huma.NewError(status, model.Detail, err)
	}

	if a.deps.Orchestrator == nil {
		return &EmptyResponse{}, nil
	}

	resumeCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	question, questionErr := a.deps.HITLRouter.GetQuestion(resumeCtx, tenantID, questionID)
	if questionErr != nil {
		rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 15*time.Second)
		if rollbackErr := a.deps.HITLRouter.ResetQuestion(rollbackCtx, tenantID, questionID); rollbackErr != nil {
			rollbackCancel()
			return nil, huma.Error500InternalServerError("failed to roll back HITL answer after question reload failure")
		}
		rollbackCancel()
		status, model := MapDomainError(questionErr)
		return nil, huma.NewError(status, model.Detail, questionErr)
	}
	if sendErr := a.deps.Orchestrator.SendHITLAnswer(resumeCtx, tenantID, question.AgentSessionID, answer); sendErr != nil {
		if errors.Is(sendErr, domain.ErrSessionUnavailable) {
			return &EmptyResponse{}, nil
		}

		rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer rollbackCancel()
		if rollbackErr := a.deps.HITLRouter.ResetQuestion(rollbackCtx, tenantID, questionID); rollbackErr != nil {
			return nil, huma.Error500InternalServerError("failed to resume agent after HITL answer")
		}
		return nil, huma.Error500InternalServerError("failed to resume agent after HITL answer")
	}

	return &EmptyResponse{}, nil
}

// ---------------------------------------------------------------------------
// Route registration
// ---------------------------------------------------------------------------

func (a *API) registerAgents(api huma.API) {
	// -----------------------------------------------------------------------
	// Agent Sessions
	// -----------------------------------------------------------------------

	huma.Register(api, huma.Operation{
		OperationID: "agent-session-list",
		Method:      http.MethodGet,
		Path:        "/api/v1/agent-sessions",
		Summary:     "List agent sessions for a project",
		Tags:        []string{"Agent"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *AgentSessionListRequest) (*AgentSessionListResponse, error) {
		return a.listSessionsByProject(ctx, req.ProjectID)
	})

	huma.Register(api, huma.Operation{
		OperationID: "task-sessions-list",
		Method:      http.MethodGet,
		Path:        "/api/v1/tasks/{task_id}/sessions",
		Summary:     "List agent sessions for a task",
		Tags:        []string{"Agent"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *TaskSessionsRequest) (*AgentSessionListResponse, error) {
		return a.listSessionsByTask(ctx, req.TaskID)
	})

	huma.Register(api, huma.Operation{
		OperationID: "agent-session-get",
		Method:      http.MethodGet,
		Path:        "/api/v1/agent-sessions/{id}",
		Summary:     "Get agent session details",
		Tags:        []string{"Agent"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *AgentSessionPathRequest) (*AgentSessionResponse, error) {
		if a.deps.AgentSessions == nil {
			return nil, huma.Error501NotImplemented("agent sessions not configured")
		}

		tenantID := reqctx.TenantIDFrom(ctx)
		session, err := a.deps.AgentSessions.GetByID(ctx, tenantID, req.ID)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}
		return mapSession(session), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "agent-session-events",
		Method:      http.MethodGet,
		Path:        "/api/v1/agent-sessions/{id}/events",
		Summary:     "List persisted replay events for an agent session",
		Tags:        []string{"Agent"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *AgentSessionEventListRequest) (*AgentSessionEventListResponse, error) {
		if a.deps.AgentEvents == nil {
			return nil, huma.Error501NotImplemented("agent session event replay not configured")
		}

		tenantID := reqctx.TenantIDFrom(ctx)
		events, err := a.deps.AgentEvents.ListBySession(ctx, tenantID, req.ID, req.Limit, req.Cursor)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		resp := &AgentSessionEventListResponse{}
		for _, event := range events {
			resp.Body.Items = append(resp.Body.Items, mapSessionEvent(event))
		}
		if len(events) == req.Limit && len(events) != 0 {
			last := events[len(events)-1].ID
			resp.Body.NextCursor = &last
		}
		return resp, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "agent-session-dispatch",
		Method:      http.MethodPost,
		Path:        "/api/v1/agent-sessions",
		Summary:     "Dispatch a task to an agent",
		Tags:        []string{"Agent"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *DispatchTaskRequest) (*DispatchTaskResponse, error) {
		return a.dispatchTask(ctx, req.Body.TaskID, req.Body.AgentType, req.Body.Prompt)
	})

	huma.Register(api, huma.Operation{
		OperationID: "task-dispatch",
		Method:      http.MethodPost,
		Path:        "/api/v1/tasks/{task_id}/dispatch",
		Summary:     "Dispatch a task to an agent",
		Tags:        []string{"Agent"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *TaskDispatchRequest) (*DispatchTaskResponse, error) {
		return a.dispatchTask(ctx, req.TaskID, req.Body.AgentType, req.Body.Prompt)
	})

	huma.Register(api, huma.Operation{
		OperationID: "agent-session-cancel",
		Method:      http.MethodPost,
		Path:        "/api/v1/agent-sessions/{id}/cancel",
		Summary:     "Cancel a running agent session",
		Tags:        []string{"Agent"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *CancelSessionRequest) (*EmptyResponse, error) {
		return a.cancelSession(ctx, req.ID)
	})

	huma.Register(api, huma.Operation{
		OperationID: "session-cancel",
		Method:      http.MethodPost,
		Path:        "/api/v1/sessions/{id}/cancel",
		Summary:     "Cancel a running agent session",
		Tags:        []string{"Agent"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *CancelSessionRequest) (*EmptyResponse, error) {
		return a.cancelSession(ctx, req.ID)
	})

	// -----------------------------------------------------------------------
	// HITL (Human-in-the-Loop)
	// -----------------------------------------------------------------------

	huma.Register(api, huma.Operation{
		OperationID: "hitl-list",
		Method:      http.MethodGet,
		Path:        "/api/v1/hitl",
		Summary:     "List HITL questions for a session",
		Tags:        []string{"HITL"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *HITLListRequest) (*HITLQuestionListResponse, error) {
		return a.listSessionQuestions(ctx, req.SessionID)
	})

	huma.Register(api, huma.Operation{
		OperationID: "session-questions-list",
		Method:      http.MethodGet,
		Path:        "/api/v1/sessions/{id}/questions",
		Summary:     "List HITL questions for a session",
		Tags:        []string{"HITL"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *SessionQuestionsRequest) (*HITLQuestionListResponse, error) {
		return a.listSessionQuestions(ctx, req.ID)
	})

	huma.Register(api, huma.Operation{
		OperationID: "hitl-get",
		Method:      http.MethodGet,
		Path:        "/api/v1/hitl/{id}",
		Summary:     "Get a HITL question",
		Tags:        []string{"HITL"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *HITLQuestionPathRequest) (*HITLQuestionResponse, error) {
		return a.getQuestion(ctx, req.ID)
	})

	huma.Register(api, huma.Operation{
		OperationID: "hitl-answer",
		Method:      http.MethodPost,
		Path:        "/api/v1/hitl/{id}/answer",
		Summary:     "Answer a HITL question",
		Tags:        []string{"HITL"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *HITLAnswerRequest) (*EmptyResponse, error) {
		return a.answerQuestion(ctx, req.ID, req.Body.Answer)
	})

	huma.Register(api, huma.Operation{
		OperationID: "question-answer",
		Method:      http.MethodPost,
		Path:        "/api/v1/questions/{id}/answer",
		Summary:     "Answer a HITL question",
		Tags:        []string{"HITL"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *HITLAnswerRequest) (*EmptyResponse, error) {
		return a.answerQuestion(ctx, req.ID, req.Body.Answer)
	})
}
