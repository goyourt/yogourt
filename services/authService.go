package services

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/interfaces"
	"github.com/goyourt/yogourt/routing"
	"github.com/goyourt/yogourt/services/database"
	"github.com/goyourt/yogourt/services/providers"
)

func Authenticate(c *gin.Context, currentUser interfaces.BaseInterface) {
	token, err := GetRequestToken(c)
	if err != nil {
		routing.RespondAndAbort(c, http.StatusUnauthorized, err.Error())
		return
	}

	parsedToken, err := ValidToken(token)
	if err != nil {
		routing.RespondAndAbort(c, http.StatusUnauthorized, "Invalid token")
		return
	}

	userUuid, err := GetClaim(parsedToken, "uuid")
	if err != nil {
		routing.RespondAndAbort(c, http.StatusUnauthorized, err.Error())
		return
	}

	database.GetOneBy(currentUser, map[string]any{"uuid": userUuid})
	if currentUser.GetID() == 0 {
		routing.RespondAndAbort(c, http.StatusUnauthorized, "User not found")
		return
	}

	setCurrentUser(c, currentUser)
}

func setCurrentUser(c *gin.Context, currentUser interfaces.BaseInterface) {
	c.Set(providers.ContextCurrentUser, currentUser)
}
