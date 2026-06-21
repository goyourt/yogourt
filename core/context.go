package core

import "github.com/gin-gonic/gin"

// Context is Yogourt's request context.
//
// It embeds Gin's context to keep the full Gin API available while giving
// Yogourt room to add framework-level helpers over time.
type Context struct {
	*gin.Context
}

func NewContext(c *gin.Context) *Context {
	return &Context{Context: c}
}

func (c *Context) Gin() *gin.Context {
	return c.Context
}
