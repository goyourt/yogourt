package compiler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	yogourt "github.com/goyourt/yogourt"
)

func TestAdaptRouteHandlerInjectsTaggedParams(t *testing.T) {
	type params struct {
		UserID string `param:"userId"`
		Slug   string `param:"slug"`
		Page   int    `param:"page"`
		Skip   string `param:"-"`
	}

	var got params
	handler, err := adaptRouteHandler(func(c *yogourt.Context, p params) {
		if c.Gin() == nil {
			t.Fatal("expected wrapped gin context")
		}
		got = p
	})
	if err != nil {
		t.Fatalf("adaptRouteHandler returned error: %v", err)
	}

	c := newTestGinContext()
	c.Params = gin.Params{
		{Key: "userId", Value: "abc-123"},
		{Key: "slug", Value: "hello-world"},
		{Key: "page", Value: "12"},
	}

	handler(c)

	if got.UserID != "abc-123" {
		t.Errorf("expected userId param, got %q", got.UserID)
	}
	if got.Slug != "hello-world" {
		t.Errorf("expected slug param, got %q", got.Slug)
	}
	if got.Page != 12 {
		t.Errorf("expected page param, got %d", got.Page)
	}
	if got.Skip != "" {
		t.Errorf("expected skipped field to stay empty, got %q", got.Skip)
	}
}

func TestAdaptRouteHandlerInjectsPointerParams(t *testing.T) {
	type params struct {
		ID string `param:"id"`
	}

	var got *params
	handler, err := adaptRouteHandler(func(c *yogourt.Context, p *params) {
		got = p
	})
	if err != nil {
		t.Fatalf("adaptRouteHandler returned error: %v", err)
	}

	c := newTestGinContext()
	c.Params = gin.Params{{Key: "id", Value: "42"}}

	handler(c)

	if got == nil {
		t.Fatal("expected params pointer")
	}
	if got.ID != "42" {
		t.Errorf("expected id param, got %q", got.ID)
	}
}

func TestAdaptRouteHandlerInjectsMatchingUntaggedParams(t *testing.T) {
	type params struct {
		ID     string
		UserID string
	}

	var got params
	handler, err := adaptRouteHandler(func(c *yogourt.Context, p params) {
		got = p
	})
	if err != nil {
		t.Fatalf("adaptRouteHandler returned error: %v", err)
	}

	c := newTestGinContext()
	c.Params = gin.Params{
		{Key: "id", Value: "42"},
		{Key: "userId", Value: "abc-123"},
	}

	handler(c)

	if got.ID != "42" {
		t.Errorf("expected id param, got %q", got.ID)
	}
	if got.UserID != "abc-123" {
		t.Errorf("expected userId param, got %q", got.UserID)
	}
}

func TestAdaptRouteHandlerRejectsInvalidParamValue(t *testing.T) {
	type params struct {
		Page int `param:"page"`
	}

	handler, err := adaptRouteHandler(func(c *yogourt.Context, p params) {})
	if err != nil {
		t.Fatalf("adaptRouteHandler returned error: %v", err)
	}

	c := newTestGinContext()
	c.Params = gin.Params{{Key: "page", Value: "not-an-int"}}

	handler(c)

	if !c.IsAborted() {
		t.Fatal("expected context to be aborted")
	}
	if c.Writer.Status() != 400 {
		t.Errorf("expected status 400, got %d", c.Writer.Status())
	}
}

func newTestGinContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	return c
}
