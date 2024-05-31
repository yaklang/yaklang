package dashscopego

import "errors"

type WrapMessageError struct {
	Message string
	Cause   error
}

func (e *WrapMessageError) Error() string {
	if e.Cause == nil {
		return e.Message
	}
	return e.Message + ": " + e.Cause.Error()
}

var (
	ErrModelNotSet     = errors.New("model is not set")
	ErrEmptyResponse   = errors.New("empty response")
	ErrImageFilePrefix = errors.New("file prefix is not supported, must be one of: file://, https://, http://")
)
