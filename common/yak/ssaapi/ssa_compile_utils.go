package ssaapi

import "errors"

var (
	ErrContextCancel       error = errors.New("context cancel")
	ErrNoFoundCompiledFile error = errors.New("not found can compiled file")
)
