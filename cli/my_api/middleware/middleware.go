package middleware

import (
	"github.com/gin-gonic/gin"
)

var Callbacks = map[string]func(*gin.Context){
	"/":          base,
}

func base(c *gin.Context) {
	c.Next()
}

