package v1

import (
	"github.com/google/uuid"
)

// EmptyResponse represents a 204 No Content response.
type EmptyResponse struct{}

// PaginationRequest represents standard cursor-based pagination query parameters.
type PaginationRequest struct {
	Cursor *uuid.UUID `query:"cursor" required:"false" doc:"Cursor for pagination (UUID of the last item in the previous page)"`
	Limit  int        `query:"limit" default:"50" minimum:"1" maximum:"100" doc:"Number of items to return"`
}

// StubResponse represents a placeholder 501 Not Implemented response.
type StubResponse struct {
	Body struct {
		Message string `json:"message"`
	}
}
