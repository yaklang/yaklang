package utils

func LastNonZero[T comparable](values ...T) T {
	var zero T
	var result T
	for _, value := range values {
		if value != zero {
			result = value
		}
	}
	return result
}
