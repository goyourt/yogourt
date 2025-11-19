package services

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/interfaces"
	"github.com/goyourt/yogourt/services/database"
)

func Authenticate(c *gin.Context) {
	token, err := GetRequestToken(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err})
		c.Abort()
		return
	}

	parsedToken, err := ValidToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		c.Abort()
		return
	}

	userUuid, err := GetClaim(parsedToken, "uuid")
	fmt.Println(userUuid, err)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err})
		c.Abort()
		return
	}

	var currentUser interfaces.BaseInterface
	database.GetOneBy(currentUser, map[string]any{"uuid": userUuid})
	if currentUser.GetID() == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found"})
		c.Abort()
		return
	}

	c.Set("currentUser", currentUser)
}
