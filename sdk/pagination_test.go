package sdk

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParsePageRequest_Defaults(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test", nil)

	page := ParsePageRequest(c)

	if page.Page != 1 {
		t.Errorf("default page = %d, want 1", page.Page)
	}
	if page.PerPage != 25 {
		t.Errorf("default per_page = %d, want 25", page.PerPage)
	}
	if page.Offset() != 0 {
		t.Errorf("default offset = %d, want 0", page.Offset())
	}
}

func TestParsePageRequest_Custom(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test?page=3&per_page=10", nil)

	page := ParsePageRequest(c)

	if page.Page != 3 {
		t.Errorf("page = %d, want 3", page.Page)
	}
	if page.PerPage != 10 {
		t.Errorf("per_page = %d, want 10", page.PerPage)
	}
	if page.Offset() != 20 {
		t.Errorf("offset = %d, want 20", page.Offset())
	}
}

func TestParsePageRequest_CapsPerPage(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test?per_page=500", nil)

	page := ParsePageRequest(c)

	if page.PerPage != 100 {
		t.Errorf("per_page should be capped at 100, got %d", page.PerPage)
	}
}

func TestParsePageRequest_NegativeValues(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test?page=-1&per_page=-5", nil)

	page := ParsePageRequest(c)

	if page.Page != 1 {
		t.Errorf("negative page should default to 1, got %d", page.Page)
	}
	if page.PerPage != 25 {
		t.Errorf("negative per_page should default to 25, got %d", page.PerPage)
	}
}
