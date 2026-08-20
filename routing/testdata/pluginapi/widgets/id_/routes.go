// Route plugin fixture for the dynamic folder "id_" (route /api/widgets/:id).
// It exports the two typed-parameter signatures: by value and by pointer. See
// testdata/pluginapi/README.md.
package main

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/core"
)

func main() {}

// detailParams is filled from the route parameters by field name: "ID"
// normalizes to the ":id" segment, and the framework must convert the raw
// string to an int.
type detailParams struct {
	ID int
}

// patchParams names its parameter explicitly and opts one field out, to prove
// both tag forms survive the plugin boundary.
type patchParams struct {
	ID    int64  `param:"id"`
	Label string `param:"-"`
}

// GET receives its parameters by value.
func GET(c *core.Context, params detailParams) {
	c.JSON(http.StatusOK, gin.H{
		"handler":   "widgets.detail",
		"signature": "core+params",
		"id":        params.ID,
		"idType":    fmt.Sprintf("%T", params.ID),
	})
}

// PATCH receives its parameters by pointer.
func PATCH(c *core.Context, params *patchParams) {
	c.JSON(http.StatusOK, gin.H{
		"handler":   "widgets.patch",
		"signature": "core+*params",
		"id":        params.ID,
		"idType":    fmt.Sprintf("%T", params.ID),
		"label":     params.Label,
	})
}
