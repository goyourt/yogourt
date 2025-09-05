package test

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/middleware"
)

func TestMiddlewares(t *testing.T) {
	middleware.SetMiddlewares(map[string]func(*gin.Context){
		"/": func(c *gin.Context) {
			c.Set("mw1", "mw1")
			c.Next()
		},
		"/a": func(c *gin.Context) {
			c.Set("mw2", "mw2")
			c.Next()
		},
		"/a/b": nil,
		"^/a/c": func(c *gin.Context) {
			c.Set("mw3", "mw3")
			c.Next()
		},
	})

	c1 := executeMiddlewares("/")
	c2 := executeMiddlewares("/test")
	c3 := executeMiddlewares("/a")
	c4 := executeMiddlewares("/a/b")
	c5 := executeMiddlewares("/a/c")

	if val, exist := c1.Get("mw1"); !exist || val != "mw1" {
		t.Errorf("middleware not applied on route '/'")
	}
	if val, exist := c2.Get("mw1"); !exist || val != "mw1" {
		t.Errorf("middleware not applied on route '/test'")
	}
	if val, exist := c3.Get("mw1"); !exist || val != "mw1" {
		t.Errorf("'/' middlewares not applied on route '/a'")
	}
	if val, exist := c3.Get("mw2"); !exist || val != "mw2" {
		t.Errorf("middleware not applied on route '/a'")
	}
	if _, exist := c4.Get("mw1"); exist {
		t.Errorf("middlewares from '/a' should not be applied on route '/a/b'")
	}
	if _, exist := c5.Get("mw1"); exist {
		t.Errorf("middlewares from '/' should not be applied on route '/a/c'")
	}
	if val, exist := c5.Get("mw3"); !exist || val != "mw3" {
		t.Errorf("middleware not applied on route '/a/c'")
	}
}

func executeMiddlewares(path string) *gin.Context {
	c, _ := gin.CreateTestContext(nil)

	for _, mw := range middleware.GetMiddleware(path) {
		mw(c)
	}

	return c
}
