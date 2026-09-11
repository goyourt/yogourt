package routing

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/interfaces"
)

type stubResource struct {
	PublicId string `json:"id"`
}

func (s *stubResource) GetPublicId() string    { return "" }
func (s *stubResource) PublicIdColumn() string { return "public_id" }

type embeddedModel struct {
	interfaces.Base
}

type sliceRequest struct {
	Values   []stubResource      `json:"values"`
	Pointers []*stubResource     `json:"pointers"`
	Ignored  []int               `json:"ignored"`
	Direct   stubResource        `json:"direct"`
	TypedNil interfaces.Resource `json:"-"`
}

// HandleRequest must traverse direct struct fields, []T (via Addr) and []*T
// without panicking — a typed-nil pointer inside an interface field included.
// Empty public ids skip hydration, so no database is needed here.
func TestHandleRequestTraversesSlices(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := `{"values":[{"id":"a"}],"pointers":[{"id":"b"},null],"ignored":[1,2],"direct":{"id":"c"}}`
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	req := sliceRequest{TypedNil: (*embeddedModel)(nil)}
	if !HandleRequest(c, &req) {
		t.Fatalf("expected the request to pass, body: %s", w.Body.String())
	}
	if len(req.Values) != 1 || req.Values[0].PublicId != "a" {
		t.Error("[]T elements must be bound")
	}
	if len(req.Pointers) != 2 || req.Pointers[0].PublicId != "b" {
		t.Error("[]*T elements must be bound")
	}
}

func TestHandleRequestRejectsInvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"values":`))
	c.Request.Header.Set("Content-Type", "application/json")

	req := sliceRequest{}
	if HandleRequest(c, &req) {
		t.Fatal("expected a malformed body to be refused")
	}
	if w.Code != 422 {
		t.Errorf("expected 422, got %d", w.Code)
	}
}

func TestHandleRequestReportsOversizedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"values":[]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Body = http.MaxBytesReader(w, c.Request.Body, 8)

	if HandleRequest(c, &sliceRequest{}) {
		t.Fatal("expected an oversized body to be refused")
	}
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", w.Code)
	}
}
