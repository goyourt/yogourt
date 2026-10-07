package providers

import (
	"github.com/gin-gonic/gin"
)

const ContextCurrentUser string = "currentUser"

func GetCurrentUser(c *gin.Context) any {
	currentUser, exist := c.Get(ContextCurrentUser)
	if !exist {
		return nil
	}
	return currentUser
}
