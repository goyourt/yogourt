package compiler

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"plugin"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/core"
)

// ErrSymbolNotFound wraps every symbol lookup failure. Callers treating a
// symbol as optional can detect it with errors.Is; any other LoadSymbol error
// means the symbol exists but does not have the expected type.
var ErrSymbolNotFound = errors.New("plugin symbol not found")

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
		fn, err := adaptRouteHandler(sym)
		if err != nil {
			return nil, fmt.Errorf("symbol %s has invalid signature in %s", m, soPath)
		}
		out[m] = fn
	}

	return out, nil
}

func adaptRouteHandler(sym any) (gin.HandlerFunc, error) {
	if fn, ok := sym.(func(*gin.Context)); ok {
		return fn, nil
	}

	if fn, ok := sym.(func(*core.Context)); ok {
		return func(c *gin.Context) {
			fn(core.NewContext(c))
		}, nil
	}

	value := reflect.ValueOf(sym)
	if value.Kind() != reflect.Func {
		return nil, fmt.Errorf("handler is not a function")
	}

	handlerType := value.Type()
	yogourtContextType := reflect.TypeOf((*core.Context)(nil))
	if handlerType.NumIn() != 2 || handlerType.NumOut() != 0 || handlerType.In(0) != yogourtContextType {
		return nil, fmt.Errorf("handler must be func(*gin.Context), func(*yogourt.Context), or func(*yogourt.Context, Params)")
	}

	paramsType := handlerType.In(1)
	if !isSupportedParamsType(paramsType) {
		return nil, fmt.Errorf("handler params must be a struct or pointer to struct")
	}

	return func(c *gin.Context) {
		params, err := hydrateRouteParams(c, paramsType)
		if err != nil {
			// The conversion detail describes the handler signature, not the
			// request: it stays server-side, the caller gets a generic body.
			log.Printf("invalid route parameters: %v", err)
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid request parameters"})
			return
		}
		value.Call([]reflect.Value{reflect.ValueOf(core.NewContext(c)), params})
	}, nil
}

func isSupportedParamsType(paramsType reflect.Type) bool {
	if paramsType.Kind() == reflect.Ptr {
		paramsType = paramsType.Elem()
	}
	return paramsType.Kind() == reflect.Struct
}

func hydrateRouteParams(c *gin.Context, paramsType reflect.Type) (reflect.Value, error) {
	isPointer := paramsType.Kind() == reflect.Ptr
	structType := paramsType
	if isPointer {
		structType = paramsType.Elem()
	}

	paramsValue := reflect.New(structType).Elem()
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		if field.PkgPath != "" {
			continue
		}

		paramName, ok := routeParamName(c, field)
		if !ok {
			continue
		}

		if err := setParamValue(paramsValue.Field(i), c.Param(paramName), paramName); err != nil {
			return reflect.Value{}, err
		}
	}

	if isPointer {
		ptr := reflect.New(structType)
		ptr.Elem().Set(paramsValue)
		return ptr, nil
	}

	return paramsValue, nil
}

func routeParamName(c *gin.Context, field reflect.StructField) (string, bool) {
	if tag, ok := field.Tag.Lookup("param"); ok {
		if tag == "-" {
			return "", false
		}
		return tag, tag != ""
	}

	return matchingRouteParamName(c, field.Name)
}

func matchingRouteParamName(c *gin.Context, fieldName string) (string, bool) {
	normalizedFieldName := normalizeParamName(fieldName)
	for _, param := range c.Params {
		if normalizeParamName(param.Key) == normalizedFieldName {
			return param.Key, true
		}
	}

	return "", false
}

func normalizeParamName(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(unicode.ToLower(r))
		}
	}
	return builder.String()
}

func setParamValue(field reflect.Value, value string, name string) error {
	if !field.CanSet() {
		return nil
	}

	if field.Kind() == reflect.Ptr {
		if value == "" {
			return nil
		}
		ptr := reflect.New(field.Type().Elem())
		if err := setParamValue(ptr.Elem(), value, name); err != nil {
			return err
		}
		field.Set(ptr)
		return nil
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Bool:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid route param %q: expected bool", name)
		}
		field.SetBool(parsed)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		parsed, err := strconv.ParseInt(value, 10, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("invalid route param %q: expected integer", name)
		}
		field.SetInt(parsed)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		parsed, err := strconv.ParseUint(value, 10, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("invalid route param %q: expected unsigned integer", name)
		}
		field.SetUint(parsed)
	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(value, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("invalid route param %q: expected float", name)
		}
		field.SetFloat(parsed)
	default:
		return fmt.Errorf("unsupported route param %q type %s", name, field.Type())
	}

	return nil
}

// symbolLookuper is the minimal surface LoadSymbol needs from a plugin.
// *plugin.Plugin satisfies it; tests can supply a fake to exercise
// LoadSymbolFrom without opening a real .so file.
type symbolLookuper interface {
	Lookup(symbol string) (plugin.Symbol, error)
}

func LoadSymbol[T any](soPath, symbol string) (*T, error) {
	plg, err := plugin.Open(soPath)
	if err != nil {
		return nil, err
	}

	return LoadSymbolFrom[T](plg, symbol)
}

// LoadSymbolFrom looks up symbol via looker and asserts it has type *T,
// returning a descriptive error instead of panicking when it doesn't.
func LoadSymbolFrom[T any](looker symbolLookuper, symbol string) (*T, error) {
	sym, err := looker.Lookup(symbol)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSymbolNotFound, err)
	}

	value, ok := sym.(*T)
	if !ok {
		return nil, fmt.Errorf("plugin symbol %q: expected type %T, got %T", symbol, value, sym)
	}

	return value, nil
}
