package domain

import "errors"

// Sentinel errors for the domain layer.
var (
	ErrNotFound             = errors.New("not found")
	ErrConflict             = errors.New("conflict")
	ErrInvalidTransition    = errors.New("invalid state transition")
	ErrUnauthorized         = errors.New("unauthorized")
	ErrForbidden            = errors.New("forbidden")
	ErrInvalidInput         = errors.New("invalid input")
	ErrContainerFailed      = errors.New("container failed")
	ErrAgentProtocol        = errors.New("agent protocol error")
	ErrSessionUnavailable   = errors.New("agent session unavailable")
	ErrMessengerUnavailable = errors.New("messenger unavailable")
	ErrDatabaseUnavailable  = errors.New("database unavailable")
	ErrConfigInvalid        = errors.New("config invalid")
)
