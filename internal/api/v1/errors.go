package v1

import (
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/gosuda/steerlane/internal/domain"
)

const internalServerError = "Internal Server Error"

// MapDomainError translates a domain-layer error into an HTTP status code
// and a Huma ErrorModel following RFC 9457 Problem Details.
// Returns (500, generic error) for unmapped errors.
func MapDomainError(err error) (int, *huma.ErrorModel) {
	if err == nil {
		return http.StatusOK, nil
	}

	status, title := mapErrorToStatus(err)
	model := &huma.ErrorModel{
		Title:  title,
		Status: status,
		Detail: err.Error(),
	}

	return status, model
}

// mapErrorToStatus returns the HTTP status code and title for a domain error.
func mapErrorToStatus(err error) (status int, title string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "Not Found"

	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, "Conflict"

	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized, "Unauthorized"

	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, "Forbidden"

	case errors.Is(err, domain.ErrInvalidInput):
		return http.StatusBadRequest, "Bad Request"

	case errors.Is(err, domain.ErrInvalidTransition):
		return http.StatusUnprocessableEntity, "Unprocessable Entity"

	case errors.Is(err, domain.ErrDatabaseUnavailable):
		return http.StatusServiceUnavailable, "Service Unavailable"

	case errors.Is(err, domain.ErrMessengerUnavailable):
		return http.StatusBadGateway, "Bad Gateway"

	case errors.Is(err, domain.ErrContainerFailed):
		return http.StatusInternalServerError, internalServerError

	case errors.Is(err, domain.ErrAgentProtocol):
		return http.StatusBadGateway, "Bad Gateway"

	case errors.Is(err, domain.ErrConfigInvalid):
		return http.StatusInternalServerError, internalServerError

	default:
		return http.StatusInternalServerError, internalServerError
	}
}
