package dairy

import "reflect"

func Map[T any, R any](array []T, callback func(T) R) []R {
	mapped := make([]R, 0)

	for _, elt := range array {
		mapped = append(mapped, callback(elt))
	}

	return mapped
}

func IsArray(toTest any) bool {
	valueType := reflect.TypeOf(toTest)
	if valueType != nil && (valueType.Kind() == reflect.Slice || valueType.Kind() == reflect.Array) {
		return true
	}
	return false
}
