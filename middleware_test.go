package kernel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.edgescale.dev/kernel/sdk"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRequestID_GeneratesWhenMissing(t *testing.T) {
	k := New(DefaultConfig())
	r := gin.New()
	r.Use(k.requestID())
	r.GET("/test", func(c *gin.Context) {
		id := c.GetString("request_id")
		if id == "" {
			t.Error("request_id should be set")
		}
		c.String(200, id)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Header().Get("X-Request-ID") == "" {
		t.Error("X-Request-ID header should be set in response")
	}
}

func TestRequestID_PreservesExisting(t *testing.T) {
	k := New(DefaultConfig())
	r := gin.New()
	r.Use(k.requestID())
	r.GET("/test", func(c *gin.Context) {
		c.String(200, c.GetString("request_id"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "my-custom-id")
	r.ServeHTTP(w, req)

	if w.Body.String() != "my-custom-id" {
		t.Errorf("should preserve existing request ID, got %q", w.Body.String())
	}
}

func TestRecovery_CatchesPanic(t *testing.T) {
	k := New(DefaultConfig())
	r := gin.New()
	r.Use(k.recovery())
	r.GET("/panic", func(c *gin.Context) {
		panic("test panic")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/panic", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("panic response status = %d, want 500", w.Code)
	}
}

func TestAuthenticate_MissingHeader(t *testing.T) {
	k := New(DefaultConfig())
	r := gin.New()
	r.Use(k.authenticate())
	r.GET("/test", func(c *gin.Context) {
		c.String(200, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("missing auth status = %d, want 401", w.Code)
	}
}

func TestAuthenticate_InvalidFormat(t *testing.T) {
	k := New(DefaultConfig())
	r := gin.New()
	r.Use(k.authenticate())
	r.GET("/test", func(c *gin.Context) {
		c.String(200, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic abc123")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("invalid format status = %d, want 401", w.Code)
	}
}

// mockIdentityProvider accepts any token and returns a fixed Identity.
type mockIdentityProvider struct{}

func (mockIdentityProvider) ValidateToken(_ context.Context, token string) (*sdk.Identity, error) {
	return &sdk.Identity{
		Subject:      "user-123",
		Identifier:   "test@example.com",
		Verified:     true,
		Provider:     "mock",
		SignInMethod: "password",
		ExpiresAt:    time.Now().Add(time.Hour),
	}, nil
}
func (mockIdentityProvider) RevokeToken(_ context.Context, _ string) error { return nil }

func TestAuthenticate_ValidBearer(t *testing.T) {
	k := New(DefaultConfig())
	k.identityProvider = mockIdentityProvider{}
	r := gin.New()
	r.Use(k.authenticate())
	r.GET("/test", func(c *gin.Context) {
		token := c.GetString("auth_token")
		userID := c.GetString("user_id")
		c.String(200, token+"|"+userID)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer my-jwt-token")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("valid bearer status = %d, want 200", w.Code)
	}
	if w.Body.String() != "my-jwt-token|user-123" {
		t.Errorf("response = %q, want %q", w.Body.String(), "my-jwt-token|user-123")
	}
}

func TestResolveOrg_MissingHeader(t *testing.T) {
	k := New(DefaultConfig())
	r := gin.New()
	r.Use(k.resolveOrg())
	r.GET("/test", func(c *gin.Context) {
		c.String(200, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("missing org status = %d, want 400", w.Code)
	}
}

func TestResolveOrg_InvalidUUID(t *testing.T) {
	k := New(DefaultConfig())
	r := gin.New()
	r.Use(k.resolveOrg())
	r.GET("/test", func(c *gin.Context) {
		c.String(200, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Org-ID", "not-a-uuid")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid uuid status = %d, want 400", w.Code)
	}
}

func TestResolveOrg_ValidUUID(t *testing.T) {
	k := New(DefaultConfig())
	r := gin.New()
	r.Use(k.resolveOrg())
	r.GET("/test", func(c *gin.Context) {
		orgID, _ := c.Get("org_id")
		c.String(200, "%v", orgID)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Org-ID", "550e8400-e29b-41d4-a716-446655440000")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("valid org status = %d, want 200", w.Code)
	}
}

func TestCheckPermission_Allows(t *testing.T) {
	k := New(DefaultConfig())
	r := gin.New()

	// Simulate a user with permissions.
	r.Use(func(c *gin.Context) {
		c.Set("permissions", NewPermissionSet([]string{"orders.create"}))
		c.Next()
	})
	r.Use(k.checkPermission("orders.create"))
	r.GET("/test", func(c *gin.Context) {
		c.String(200, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("allowed permission status = %d, want 200", w.Code)
	}
}

func TestCheckPermission_Denies(t *testing.T) {
	k := New(DefaultConfig())
	r := gin.New()

	r.Use(func(c *gin.Context) {
		c.Set("permissions", NewPermissionSet([]string{"billing.read"}))
		c.Next()
	})
	r.Use(k.checkPermission("orders.create"))
	r.GET("/test", func(c *gin.Context) {
		c.String(200, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("denied permission status = %d, want 403", w.Code)
	}
}

func TestCheckPermission_PipeSeparatedOR(t *testing.T) {
	k := New(DefaultConfig())
	r := gin.New()

	r.Use(func(c *gin.Context) {
		c.Set("permissions", NewPermissionSet([]string{"billing.read"}))
		c.Next()
	})
	r.Use(k.checkPermission("orders.create|billing.read"))
	r.GET("/test", func(c *gin.Context) {
		c.String(200, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("OR permission status = %d, want 200", w.Code)
	}
}

func TestHealthz(t *testing.T) {
	k := New(DefaultConfig())
	r := gin.New()
	r.GET("/healthz", k.handleHealthz)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/healthz", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("healthz status = %d, want 200", w.Code)
	}

	var body struct {
		Success bool              `json:"success"`
		Result  map[string]string `json:"result"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if !body.Success {
		t.Error("healthz success should be true")
	}
	if body.Result["status"] != "ok" {
		t.Errorf("healthz status = %q, want %q", body.Result["status"], "ok")
	}
}

func TestListModules(t *testing.T) {
	k := New(DefaultConfig())
	k.MustRegister(newStub("orders"))
	k.MustRegister(newStub("billing"))

	r := gin.New()
	r.GET("/v1/modules", k.handleActiveModules)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/modules", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("modules status = %d, want 200", w.Code)
	}

	var body struct {
		Success bool         `json:"success"`
		Result  []moduleInfo `json:"result"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)

	if !body.Success {
		t.Error("modules success should be true")
	}
	// 2 auto-registered (iam, iam-admin) + 2 stubs = 4
	if len(body.Result) != 4 {
		t.Errorf("modules count = %d, want 4", len(body.Result))
	}
}
