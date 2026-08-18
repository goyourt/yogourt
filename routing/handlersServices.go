package routing

import (
	"errors"
	"net/http"
	"reflect"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/interfaces"
	"github.com/goyourt/yogourt/services/database"
	"gorm.io/gorm"
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
				if obj.GetUuid() != "" && !hydrateRelation(c, obj) {
					return false
				}
			}
		}

		// slice case : []interfaces.BaseInterface
		if f.Kind() == reflect.Slice {
			for j := 0; j < f.Len(); j++ {
				elem := f.Index(j)
				if elem.Kind() != reflect.Interface {
					continue
				}
				if !elem.CanInterface() || elem.IsNil() {
					continue
				}
				if obj, valid := elem.Interface().(interfaces.BaseInterface); valid && obj != nil {
					if obj.GetUuid() != "" && !hydrateRelation(c, obj) {
						return false
					}
				}
			}
		}
	}

	return true
}

// hydrateRelation loads obj by its uuid. An unknown uuid leaves the object
// unhydrated and lets the handler run, exactly as before v2 (D1) — a 422 here
// would also give anonymous callers an existence oracle on any referenced
// table, defeating the 404 masking of D8. Only a technical database failure
// aborts the request.
func hydrateRelation(c *gin.Context, obj interfaces.BaseInterface) bool {
	if err := database.GetOneBy(obj, map[string]any{"uuid": obj.GetUuid()}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return true
		}
		RespondServiceUnavailable(c)
		return false
	}
	return true
}

func RespondAndAbort(c *gin.Context, status int, error string) {
	c.JSON(status, gin.H{"error": error})
	c.Abort()
}

func RespondSuccess(c *gin.Context, data any) {
	RespondWithContent(c, http.StatusOK, "data", data)
}

func RespondCreated(c *gin.Context, data any) {
	RespondWithContent(c, http.StatusCreated, "data", data)
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

func RespondServiceUnavailable(c *gin.Context) {
	RespondAndAbort(c, http.StatusServiceUnavailable, "Service temporarily unavailable")
}
