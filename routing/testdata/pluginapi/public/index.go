// Route plugin fixture exporting a Permissions symbol: GET /api/public is
// declared public, so no RBAC middleware is inserted in front of it. Without
// an authorizer the symbol must simply be ignored (D1). See
// testdata/pluginapi/README.md.
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/authorization"
)

func main() {}

// Permissions overrides the permission derived by convention for this folder.
var Permissions = map[string]string{
	"GET": authorization.Public,
}

func GET(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"handler":   "public.index",
		"signature": "gin",
	})
}
