// Route plugin fixture: POST /api/widgets, in a second file of the same
// folder, so one route is served by two plugins. See
// testdata/pluginapi/README.md.
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/core"
)

func main() {}

// POST uses the Yogourt context: the loader must wrap it into a
// gin.HandlerFunc across the plugin boundary.
func POST(c *core.Context) {
	c.JSON(http.StatusCreated, gin.H{
		"handler":   "widgets.create",
		"signature": "core",
	})
}
