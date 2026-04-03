package sdk_test

import (
	"net/http"
	"testing"

	"go.edgescale.dev/kernel/sdk"
)

func TestNotFound(t *testing.T) {
	err := sdk.NotFound("order", "abc-123")
	if err.HTTPStatus != http.StatusNotFound {
		t.Errorf("NotFound.HTTPStatus = %d, want %d", err.HTTPStatus, http.StatusNotFound)
	}
	if err.Code != "not_found" {
		t.Errorf("NotFound.Code = %q, want %q", err.Code, "not_found")
	}
	if err.Error() == "" {
		t.Error("NotFound.Error() should not be empty")
	}
}

func TestBadRequest(t *testing.T) {
	err := sdk.BadRequest("invalid email format")
	if err.HTTPStatus != http.StatusBadRequest {
		t.Errorf("BadRequest.HTTPStatus = %d, want %d", err.HTTPStatus, http.StatusBadRequest)
	}
	if err.Code != "bad_request" {
		t.Errorf("BadRequest.Code = %q, want %q", err.Code, "bad_request")
	}
}

func TestForbidden(t *testing.T) {
	err := sdk.Forbidden("insufficient permissions")
	if err.HTTPStatus != http.StatusForbidden {
		t.Errorf("Forbidden.HTTPStatus = %d, want %d", err.HTTPStatus, http.StatusForbidden)
	}
}

func TestConflict(t *testing.T) {
	err := sdk.Conflict("duplicate entry")
	if err.HTTPStatus != http.StatusConflict {
		t.Errorf("Conflict.HTTPStatus = %d, want %d", err.HTTPStatus, http.StatusConflict)
	}
}

func TestInternal(t *testing.T) {
	err := sdk.Internal("something went wrong")
	if err.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("Internal.HTTPStatus = %d, want %d", err.HTTPStatus, http.StatusInternalServerError)
	}
}

func TestUnauthorized(t *testing.T) {
	err := sdk.Unauthorized("invalid token")
	if err.HTTPStatus != http.StatusUnauthorized {
		t.Errorf("Unauthorized.HTTPStatus = %d, want %d", err.HTTPStatus, http.StatusUnauthorized)
	}
}

func TestAbortError(t *testing.T) {
	reason := sdk.Forbidden("cannot modify closed order")
	abortErr := sdk.Abort(reason)

	if abortErr.Reason != reason {
		t.Error("AbortError.Reason should point to the original ServiceError")
	}
	if abortErr.Error() == "" {
		t.Error("AbortError.Error() should not be empty")
	}

	// Test IsAbortError
	ae, ok := sdk.IsAbortError(abortErr)
	if !ok {
		t.Error("IsAbortError should return true for AbortError")
	}
	if ae.Reason.Code != "forbidden" {
		t.Errorf("IsAbortError.Reason.Code = %q, want %q", ae.Reason.Code, "forbidden")
	}

	// Test non-abort error
	_, ok = sdk.IsAbortError(sdk.NotFound("x", 1))
	if ok {
		t.Error("IsAbortError should return false for ServiceError")
	}
}

func TestIsServiceError(t *testing.T) {
	se := sdk.BadRequest("test")
	got, ok := sdk.IsServiceError(se)
	if !ok {
		t.Error("IsServiceError should return true for ServiceError")
	}
	if got.Code != "bad_request" {
		t.Errorf("IsServiceError.Code = %q, want %q", got.Code, "bad_request")
	}
}
