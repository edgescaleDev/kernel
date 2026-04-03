package sdk

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Envelope is the standard API response wrapper, inspired by the Cloudflare v4 API.
// Every endpoint returns this shape — clients always know what to expect.
type Envelope struct {
	// Success indicates whether the request was successful.
	Success bool `json:"success"`

	// Result contains the response data (single object or array).
	Result any `json:"result"`

	// Errors contains machine-readable error details.
	Errors []APIError `json:"errors"`

	// Messages contains informational messages (e.g., deprecation notices).
	Messages []string `json:"messages"`

	// ResultInfo contains pagination metadata for list responses.
	ResultInfo *ResultInfo `json:"result_info,omitempty"`
}

// APIError represents a single error in the response envelope.
type APIError struct {
	// Code is the machine-readable error code.
	Code string `json:"code"`

	// Message is the human-readable error description.
	Message string `json:"message"`
}

// ResultInfo provides pagination metadata for list endpoints.
type ResultInfo struct {
	// Page is the current page number (1-indexed).
	Page int `json:"page"`

	// PerPage is the number of items per page.
	PerPage int `json:"per_page"`

	// TotalCount is the total number of items across all pages.
	TotalCount int64 `json:"total_count"`

	// TotalPages is the total number of pages.
	TotalPages int `json:"total_pages"`

	// Cursor is an opaque token for cursor-based pagination.
	Cursor string `json:"cursor,omitempty"`
}

// OK sends a 200 response with the result.
func OK(c *gin.Context, result any) {
	c.JSON(http.StatusOK, Envelope{
		Success: true,
		Result:  result,
	})
}

// OKWithMessage sends a 200 response with the result and an informational message.
func OKWithMessage(c *gin.Context, result any, message string) {
	c.JSON(http.StatusOK, Envelope{
		Success:  true,
		Result:   result,
		Messages: []string{message},
	})
}

// Created sends a 201 response with the created resource.
func Created(c *gin.Context, result any) {
	c.JSON(http.StatusCreated, Envelope{
		Success: true,
		Result:  result,
	})
}

// Accepted sends a 202 response for asynchronous operations.
// The result should contain an operation ID or status URL.
func Accepted(c *gin.Context, result any) {
	c.JSON(http.StatusAccepted, Envelope{
		Success: true,
		Result:  result,
	})
}

// NoContent sends a 204 response with no body.
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// List sends a 200 response with a paginated list.
func List(c *gin.Context, items any, info ResultInfo) {
	c.JSON(http.StatusOK, Envelope{
		Success:    true,
		Result:     items,
		ResultInfo: &info,
	})
}

// Err sends an error response derived from a ServiceError.
func Error(c *gin.Context, err *ServiceError) {
	status := err.HTTPStatus
	if status == 0 {
		status = http.StatusInternalServerError
	}
	c.AbortWithStatusJSON(status, Envelope{
		Success: false,
		Errors: []APIError{
			{Code: err.Code, Message: err.Message},
		},
	})
}

// Errs sends an error response with multiple errors (e.g., validation).
func Errs(c *gin.Context, status int, errors []APIError) {
	c.AbortWithStatusJSON(status, Envelope{
		Success: false,
		Errors:  errors,
	})
}

// FromError sends an error response, auto-detecting ServiceError.
// Falls back to a generic 500 if the error is not a ServiceError.
func FromError(c *gin.Context, err error) {
	if se, ok := IsServiceError(err); ok {
		Error(c, se)
		return
	}
	if ae, ok := IsAbortError(err); ok {
		Error(c, ae.Reason)
		return
	}
	Error(c, Internal(err.Error()))
}
