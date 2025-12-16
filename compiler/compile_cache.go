package compiler

import (
	"fmt"
	"plugin"
	"sync"

	"github.com/gin-gonic/gin"
)

var pluginCache sync.Map

func CompileCached(srcGoFile string) (string, error) {
	if v, ok := pluginCache.Load(srcGoFile); ok {
		return v.(string), nil
	}
	so, err := CompilePlugin(srcGoFile)
	if err != nil {
		return "", err
	}
	pluginCache.Store(srcGoFile, so)
	return so, nil
}

func LoadRoutes(soPath string) (map[string]gin.HandlerFunc, error) {
	plg, err := plugin.Open(soPath)
	if err != nil {
		return nil, err
	}

	methods := []string{"GET", "PUT", "POST", "PATCH", "DELETE"}
	out := make(map[string]gin.HandlerFunc)

	for _, m := range methods {
		sym, err := plg.Lookup(m)
		if err != nil {
			continue
		}
		fn, ok := sym.(func(*gin.Context))
		if !ok {
			return nil, fmt.Errorf("symbol %s has invalid signature in %s", m, soPath)
		}
		out[m] = fn
	}

	return out, nil
}

func LoadSymbol[T any](soPath, symbol string) (*T, error) {
	plg, err := plugin.Open(soPath)
	if err != nil {
		return nil, err
	}

	sym, err := plg.Lookup(symbol)
	if err != nil {
		return nil, err
	}

	return sym.(*T), nil
}
