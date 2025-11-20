package routing

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/interfaces"
)

func HandleRequest(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		RespondAndAbort(c, 422, "Invalid request: argument mismatch")
		return false
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
