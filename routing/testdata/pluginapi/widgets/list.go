// Route plugin fixture: GET /api/widgets, written with the raw Gin signature
// the loader accepts as is. See testdata/pluginapi/README.md.
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func main() {}

// GET is adapted by compiler.adaptRouteHandler through its fast path: the
// symbol already is a gin.HandlerFunc.
func GET(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"handler":   "widgets.list",
		"signature": "gin",
	})
}
