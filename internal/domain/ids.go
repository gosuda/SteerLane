package domain

import "github.com/google/uuid"

// TenantID uniquely identifies a tenant.
type TenantID = uuid.UUID

// UserID uniquely identifies a user.
type UserID = uuid.UUID

// ProjectID uniquely identifies a project.
type ProjectID = uuid.UUID

// ADRID uniquely identifies an architectural decision record.
type ADRID = uuid.UUID

// TaskID uniquely identifies a task.
type TaskID = uuid.UUID

// AgentSessionID uniquely identifies an agent session.
type AgentSessionID = uuid.UUID

// HITLQuestionID uniquely identifies a human-in-the-loop question.
type HITLQuestionID = uuid.UUID

// NewID generates a new random UUID.
func NewID() uuid.UUID { return uuid.New() }
