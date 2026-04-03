package sdk

import (
	"fmt"
	"net/http"
)

// ServiceError is the standard error type returned by service handlers.
// It carries an HTTP status code, a machine-readable error code, and a human-readable message.
type ServiceError struct {
	// HTTPStatus is the HTTP status code to return (e.g., 404, 400, 403).
	HTTPStatus int `json:"-"`

	// Code is a machine-readable error code (e.g., "not_found", "validation_error").
	Code string `json:"code"`

	// Message is a human-readable error description.
	Message string `json:"message"`
}

// Error implements the error interface.
func (e *ServiceError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NotFound creates a 404 error.
func NotFound(resource string, id any) *ServiceError {
	return &ServiceError{
		HTTPStatus: http.StatusNotFound,
		Code:       "not_found",
		Message:    fmt.Sprintf("%s with ID '%v' not found", resource, id),
	}
}

// BadRequest creates a 400 error.
func BadRequest(message string) *ServiceError {
	return &ServiceError{
		HTTPStatus: http.StatusBadRequest,
		Code:       "bad_request",
		Message:    message,
	}
}

// Forbidden creates a 403 error.
func Forbidden(message string) *ServiceError {
	return &ServiceError{
		HTTPStatus: http.StatusForbidden,
		Code:       "forbidden",
		Message:    message,
	}
}

// Conflict creates a 409 error.
func Conflict(message string) *ServiceError {
	return &ServiceError{
		HTTPStatus: http.StatusConflict,
		Code:       "conflict",
		Message:    message,
	}
}

// Internal creates a 500 error.
func Internal(message string) *ServiceError {
	return &ServiceError{
		HTTPStatus: http.StatusInternalServerError,
		Code:       "internal_error",
		Message:    message,
	}
}

// Unauthorized creates a 401 error.
func Unauthorized(message string) *ServiceError {
	return &ServiceError{
		HTTPStatus: http.StatusUnauthorized,
		Code:       "unauthorized",
		Message:    message,
	}
}

// Unprocessable creates a 422 error for validation failures.
func Unprocessable(message string) *ServiceError {
	return &ServiceError{
		HTTPStatus: http.StatusUnprocessableEntity,
		Code:       "validation_error",
		Message:    message,
	}
}

// RateLimited creates a 429 error.
func RateLimited(message string) *ServiceError {
	return &ServiceError{
		HTTPStatus: http.StatusTooManyRequests,
		Code:       "rate_limited",
		Message:    message,
	}
}

// Unavailable creates a 503 error for temporary outages.
func Unavailable(message string) *ServiceError {
	return &ServiceError{
		HTTPStatus: http.StatusServiceUnavailable,
		Code:       "service_unavailable",
		Message:    message,
	}
}

// AbortError is returned by hook handlers to abort the current operation.
// When a Before hook returns an AbortError, the kernel stops processing
// and returns the embedded ServiceError to the client.
type AbortError struct {
	// Reason is the underlying service error to return to the client.
	Reason *ServiceError
}

// Error implements the error interface.
func (e *AbortError) Error() string {
	return fmt.Sprintf("hook aborted: %s", e.Reason.Error())
}

// Abort creates an AbortError that stops hook chain execution.
func Abort(reason *ServiceError) *AbortError {
	return &AbortError{Reason: reason}
}

// IsAbortError checks if an error is an AbortError.
func IsAbortError(err error) (*AbortError, bool) {
	if ae, ok := err.(*AbortError); ok {
		return ae, true
	}
	return nil, false
}

// IsServiceError checks if an error is a ServiceError.
func IsServiceError(err error) (*ServiceError, bool) {
	if se, ok := err.(*ServiceError); ok {
		return se, true
	}
	return nil, false
}
