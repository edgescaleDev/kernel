package sdk

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestBindAndValidate_Success(t *testing.T) {
	r := gin.New()
	r.POST("/test", func(c *gin.Context) {
		var req struct {
			Name string `json:"name" binding:"required"`
		}
		if !BindAndValidate(c, &req) {
			return
		}
		c.String(200, req.Name)
	})

	w := httptest.NewRecorder()
	body := strings.NewReader(`{"name":"test"}`)
	req := httptest.NewRequest("POST", "/test", body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("valid body status = %d, want 200", w.Code)
	}
	if w.Body.String() != "test" {
		t.Errorf("body = %q, want %q", w.Body.String(), "test")
	}
}

func TestBindAndValidate_MissingField(t *testing.T) {
	r := gin.New()
	r.POST("/test", func(c *gin.Context) {
		var req struct {
			Name string `json:"name" binding:"required"`
		}
		if !BindAndValidate(c, &req) {
			return
		}
		c.String(200, "ok")
	})

	w := httptest.NewRecorder()
	body := strings.NewReader(`{}`)
	req := httptest.NewRequest("POST", "/test", body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("missing field status = %d, want 400", w.Code)
	}
}

func TestBindQuery_Success(t *testing.T) {
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		var filter struct {
			Status string `form:"status"`
		}
		if !BindQuery(c, &filter) {
			return
		}
		c.String(200, filter.Status)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test?status=active", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("query bind status = %d, want 200", w.Code)
	}
	if w.Body.String() != "active" {
		t.Errorf("body = %q, want %q", w.Body.String(), "active")
	}
}

func TestBindURI_Success(t *testing.T) {
	r := gin.New()
	r.GET("/orders/:id", func(c *gin.Context) {
		var params struct {
			ID string `uri:"id" binding:"required"`
		}
		if !BindURI(c, &params) {
			return
		}
		c.String(200, params.ID)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/orders/42", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("uri bind status = %d, want 200", w.Code)
	}
	if w.Body.String() != "42" {
		t.Errorf("body = %q, want %q", w.Body.String(), "42")
	}
}
