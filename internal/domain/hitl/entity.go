package hitl

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gosuda/steerlane/internal/domain"
)

// QuestionStatus represents the lifecycle state of a HITL question.
type QuestionStatus string

const (
	StatusPending   QuestionStatus = "pending"
	StatusEscalated QuestionStatus = "escalated"
	StatusAnswered  QuestionStatus = "answered"
	StatusTimeout   QuestionStatus = "timeout"
	StatusCancelled QuestionStatus = "cancelled"
)

// Question represents a human-in-the-loop question posed by an agent.
type Question struct {
	CreatedAt         time.Time
	Answer            *string
	AnsweredAt        *time.Time
	TimeoutAt         *time.Time
	AnsweredBy        *domain.UserID
	MessengerThreadID *string
	MessengerPlatform *string
	Question          string
	Status            QuestionStatus
	Options           json.RawMessage
	ID                domain.HITLQuestionID
	AgentSessionID    domain.AgentSessionID
	TenantID          domain.TenantID
}

// Validate checks that the question's fields are well-formed.
func (q *Question) Validate() error {
	if q.Question == "" {
		return fmt.Errorf("hitl question text: %w", domain.ErrInvalidInput)
	}
	switch q.Status {
	case StatusPending, StatusEscalated, StatusAnswered, StatusTimeout, StatusCancelled:
	default:
		return fmt.Errorf("hitl question status %q: %w", q.Status, domain.ErrInvalidInput)
	}
	return nil
}
