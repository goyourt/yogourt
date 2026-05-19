package dairy

func Map[T any, R any](array []T, callback func(T) R) []R {
	mapped := make([]R, 0)

	for _, elt := range array {
		mapped = append(mapped, callback(elt))
	}

	return mapped
}
