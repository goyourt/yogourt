package providers

import (
	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/interfaces"
)

const ContextCurrentUser string = "currentUser"

func GetCurrentUser(c *gin.Context) interfaces.BaseInterface {
	currentUser, exist := c.Get(ContextCurrentUser)
	if !exist {
		return nil
	}

	user, ok := currentUser.(interfaces.BaseInterface)
	if !ok {
		return nil
	}

	return user
}
