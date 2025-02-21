package middleware

import (
	"github.com/gin-gonic/gin"
	"strings"
)

func GetMiddleware(path string, middlewares map[string]func(*gin.Context)) []gin.HandlerFunc {
	var middlewareList []gin.HandlerFunc
	subroutes := strings.Split(path, "/")
	for i := -1; i <= len(subroutes); i++ {
		route := "/"
		if i >= 0 {
			route = strings.Join(subroutes[:i], "/")
		}
		if route != "" && middlewares[route] != nil {
			middlewareList = append(middlewareList, middlewares[route])

			if middlewares[route] == nil {
				middlewareList = make([]gin.HandlerFunc, 0)
			}
		}
	}

	return middlewareList
}
