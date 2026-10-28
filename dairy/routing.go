package dairy

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

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

func InternalServerError(c *gin.Context) {
	RespondAndAbort(c, http.StatusInternalServerError, "Internal server error")
}
