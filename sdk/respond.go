package sdk

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Envelope is the standard API response wrapper, inspired by the Cloudflare v4 API.
// Every endpoint returns this shape - clients always know what to expect.
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
	success(c, http.StatusOK, result, nil, []string{})
}

// OKWithMessage sends a 200 response with the result and an informational message.
func OKWithMessage(c *gin.Context, result any, message string) {
	success(c, http.StatusOK, result, nil, []string{message})
}

// Created sends a 201 response with the created resource.
func Created(c *gin.Context, result any) {
	success(c, http.StatusCreated, result, nil, []string{})
}

// Accepted sends a 202 response for asynchronous operations.
// The result should contain an operation ID or status URL.
func Accepted(c *gin.Context, result any) {
	success(c, http.StatusAccepted, result, nil, []string{})
}

// NoContent sends a 204 response with no body.
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// List sends a 200 response with a paginated list.
func List(c *gin.Context, items any, info ResultInfo) {
	success(c, http.StatusOK, items, &info, []string{})
}

func success(c *gin.Context, status int, result any, info *ResultInfo, messages []string) {
	c.JSON(status, Envelope{
		Success:    true,
		Result:     result,
		ResultInfo: info,
		Messages:   messages,
		Errors:     []APIError{},
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
		Messages: []string{},
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
// Raw error details are logged server-side but never exposed to the client.
func FromError(c *gin.Context, err error) {
	if se, ok := IsServiceError(err); ok {
		Error(c, se)
		return
	}
	if ae, ok := IsAbortError(err); ok {
		Error(c, ae.Reason)
		return
	}

	// Log the raw error for debugging — never send it to the client.
	slog.Error("unhandled error",
		"error", err.Error(),
		"path", c.Request.URL.Path,
		"method", c.Request.Method,
		"request_id", c.GetString("request_id"),
	)
	Error(c, Internal("an internal error occurred"))
}
