package sdk

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

// BindAndValidate binds the request body to the given struct and runs
// validation. Returns a 400 Bad Request with field-level errors on failure.
// Uses gin's built-in validator (go-playground/validator).
//
// Usage:
//
//	var req CreateOrderRequest
//	if !sdk.BindAndValidate(c, &req) {
//	    return // 400 already sent
//	}
func BindAndValidate(c *gin.Context, obj any) bool {
	if err := c.ShouldBindBodyWith(obj, binding.JSON); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, Envelope{
			Success: false,
			Errors:  formatValidationErrors(err),
		})
		return false
	}
	return true
}

// BindQuery binds URL query parameters to the given struct and validates.
// Useful for GET endpoints with structured query params.
//
// Usage:
//
//	var filter ListOrdersFilter
//	if !sdk.BindQuery(c, &filter) {
//	    return
//	}
func BindQuery(c *gin.Context, obj any) bool {
	if err := c.ShouldBindQuery(obj); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, Envelope{
			Success: false,
			Errors:  formatValidationErrors(err),
		})
		return false
	}
	return true
}

// BindURI binds URI parameters (e.g., :id) to a struct and validates.
//
// Usage:
//
//	var params struct { ID uint64 `uri:"id" binding:"required"` }
//	if !sdk.BindURI(c, &params) {
//	    return
//	}
func BindURI(c *gin.Context, obj any) bool {
	if err := c.ShouldBindUri(obj); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, Envelope{
			Success: false,
			Errors:  formatValidationErrors(err),
		})
		return false
	}
	return true
}

// formatValidationErrors converts a validation error into API error objects.
func formatValidationErrors(err error) []APIError {
	return []APIError{
		{
			Code:    "validation_error",
			Message: err.Error(),
		},
	}
}
