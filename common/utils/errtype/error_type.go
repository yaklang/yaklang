package errtype

func IsError[T any](err error) bool {
	_, ok := err.(T)
	return ok
}

func AsError[T any](err error) (T, bool) {
	e, ok := err.(T)
	return e, ok
}
