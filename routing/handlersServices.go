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

// HandleRequest binds the JSON body into req, then hydrates every field
// implementing interfaces.Resource whose public id is set. Other fields are
// bound as-is.
func HandleRequest(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		RespondAndAbort(c, 422, "Invalid request: argument mismatch")
		return false
	}

	rv := reflect.ValueOf(req)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return true
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return true
	}

	for i := 0; i < rv.NumField(); i++ {
		field := rv.Field(i)
		if !field.CanInterface() {
			continue
		}

		switch field.Kind() {
		case reflect.Interface, reflect.Ptr:
			if field.IsNil() {
				continue
			}
			if !hydrateCandidate(c, field.Interface()) {
				return false
			}
		case reflect.Struct:
			if !field.CanAddr() || !field.Addr().CanInterface() {
				continue
			}
			if !hydrateCandidate(c, field.Addr().Interface()) {
				return false
			}
		case reflect.Slice:
			for j := 0; j < field.Len(); j++ {
				elem := field.Index(j)
				switch elem.Kind() {
				case reflect.Interface, reflect.Ptr:
					if elem.IsNil() || !elem.CanInterface() {
						continue
					}
					if !hydrateCandidate(c, elem.Interface()) {
						return false
					}
				case reflect.Struct:
					if !elem.CanAddr() || !elem.Addr().CanInterface() {
						continue
					}
					if !hydrateCandidate(c, elem.Addr().Interface()) {
						return false
					}
				}
			}
		}
	}

	return true
}

func hydrateCandidate(c *gin.Context, candidate any) bool {
	// An interface field can hide a typed-nil pointer: it passes the type
	// assertion and panics on the first method call.
	rv := reflect.ValueOf(candidate)
	if rv.Kind() == reflect.Ptr && rv.IsNil() {
		return true
	}
	obj, ok := candidate.(interfaces.Resource)
	if !ok || obj.GetPublicId() == "" {
		return true
	}
	return hydrateRelation(c, obj)
}

// hydrateRelation loads obj by its public id. An unknown id leaves the object
// unhydrated, identifier included, and lets the handler run — a 422 here
// would give anonymous callers an existence oracle on any referenced table,
// defeating the 404 masking of D8. Only a technical database failure aborts
// the request.
func hydrateRelation(c *gin.Context, obj interfaces.Resource) bool {
	if err := database.GetOneByIdentity(obj, obj.PublicIdColumn(), obj.GetPublicId()); err != nil {
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
