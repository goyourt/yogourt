// Route plugin fixture with an inconsistent Permissions declaration: the
// DELETE entry names no exported handler, which the loader must report as a
// violation instead of registering the route. See
// testdata/pluginapi/README.md.
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func main() {}

// Permissions declares one method that does not exist in this folder.
var Permissions = map[string]string{
	"GET":    "widgets.read",
	"DELETE": "widgets.delete",
}

func GET(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"handler": "orphan.index"})
}
