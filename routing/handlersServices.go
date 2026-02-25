package routing

import (
	"net/http"
	"reflect"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/interfaces"
	"github.com/goyourt/yogourt/services/database"
)

func HandleRequest(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		RespondAndAbort(c, 422, "Invalid request: argument mismatch")
		return false
	}

	// Hydrate relations in req if they got an uuid
	rv := reflect.ValueOf(req)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return true
	}
	rv = rv.Elem()

	for i := 0; i < rv.NumField(); i++ {
		f := rv.Field(i)
		if !f.CanInterface() {
			continue
		}

		// simple case : BaseInterface
		if (f.Kind() == reflect.Interface || f.Kind() == reflect.Ptr) && !f.IsNil() {
			if obj, valid := f.Interface().(interfaces.BaseInterface); valid && obj != nil {
				if obj.GetUuid() != "" {
					database.GetOneBy(obj, map[string]any{"uuid": obj.GetUuid()})
				}
			}
		}

		// slice case : []interfaces.BaseInterface
		if f.Kind() == reflect.Slice {
			for j := 0; j < f.Len(); j++ {
				elem := f.Index(j)
				if !elem.CanInterface() || elem.IsNil() {
					continue
				}
				if obj, valid := elem.Interface().(interfaces.BaseInterface); valid && obj != nil {
					if obj.GetUuid() != "" {
						database.GetOneBy(obj, map[string]any{"uuid": obj.GetUuid()})
					}
				}
			}
		}
	}

	return true
}

func RespondAndAbort(c *gin.Context, status int, error string) {
	c.JSON(status, gin.H{"error": error})
	c.Abort()
}

func RespondSuccess(c *gin.Context, status int, data interfaces.BaseInterface) {
	RespondWithContent(c, status, "data", data)
}

func RespondNoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
	c.Next()
}

func RespondWithContent(c *gin.Context, status int, key string, content any) {
	c.JSON(status, gin.H{key: content})
	c.Next()
}

func RespondNotFound(c *gin.Context) {
	RespondAndAbort(c, http.StatusNotFound, "Resource not found")
}
