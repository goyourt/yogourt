package dairy

import (
	"reflect"
	"runtime"
)

func FuncName(f func()) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}
