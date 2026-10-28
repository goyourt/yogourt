package yogourt

import (
	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/core"
)

type Context = core.Context

func NewContext(c *gin.Context) *Context {
	return core.NewContext(c)
}
