package sdk_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.edgescale.dev/kernel/sdk"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestOK(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	sdk.OK(c, map[string]string{"name": "test"})

	if w.Code != http.StatusOK {
		t.Errorf("OK status = %d, want %d", w.Code, http.StatusOK)
	}

	var env sdk.Envelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if !env.Success {
		t.Error("OK should set success=true")
	}
	if env.Result == nil {
		t.Error("OK should include result")
	}
}

func TestCreated(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	sdk.Created(c, map[string]string{"id": "abc"})

	if w.Code != http.StatusCreated {
		t.Errorf("Created status = %d, want %d", w.Code, http.StatusCreated)
	}
}

func TestAccepted(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	sdk.Accepted(c, map[string]string{"operation_id": "op-123"})

	if w.Code != http.StatusAccepted {
		t.Errorf("Accepted status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestNoContent(t *testing.T) {
	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		sdk.NoContent(c)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("NoContent status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestList(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	items := []string{"a", "b", "c"}
	info := sdk.ResultInfo{
		Page:       1,
		PerPage:    10,
		TotalCount: 3,
		TotalPages: 1,
	}

	sdk.List(c, items, info)

	if w.Code != http.StatusOK {
		t.Errorf("List status = %d, want %d", w.Code, http.StatusOK)
	}

	var env sdk.Envelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if env.ResultInfo == nil {
		t.Error("List should include result_info")
	}
	if env.ResultInfo.TotalCount != 3 {
		t.Errorf("List result_info.total_count = %d, want 3", env.ResultInfo.TotalCount)
	}
}

func TestError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	sdk.Error(c, sdk.NotFound("order", "xyz"))

	if w.Code != http.StatusNotFound {
		t.Errorf("Err status = %d, want %d", w.Code, http.StatusNotFound)
	}

	var env sdk.Envelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if env.Success {
		t.Error("Err should set success=false")
	}
	if len(env.Errors) != 1 {
		t.Errorf("Err should have 1 error, got %d", len(env.Errors))
	}
	if env.Errors[0].Code != "not_found" {
		t.Errorf("Err error code = %q, want %q", env.Errors[0].Code, "not_found")
	}
}

func TestFromError_ServiceError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	sdk.FromError(c, sdk.BadRequest("invalid"))

	if w.Code != http.StatusBadRequest {
		t.Errorf("FromError(ServiceError) status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestFromError_GenericError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	sdk.FromError(c, &sdk.ServiceError{HTTPStatus: 0, Code: "", Message: "generic"})

	// A zero-value HTTPStatus should default to 500, not pass through as 0
	if w.Code != http.StatusInternalServerError {
		t.Errorf("FromError(zero-status ServiceError) status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestOKWithMessage(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	sdk.OKWithMessage(c, "result", "Resource updated successfully")

	var env sdk.Envelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if len(env.Messages) != 1 {
		t.Errorf("OKWithMessage should have 1 message, got %d", len(env.Messages))
	}
}
