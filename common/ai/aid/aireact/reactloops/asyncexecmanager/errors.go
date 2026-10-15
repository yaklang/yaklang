package asyncexecmanager

import "errors"

var (
	ErrClosed          = errors.New("async execution manager is closed")
	ErrNotFound        = errors.New("async execution not found")
	ErrDuplicateID     = errors.New("async execution id already exists")
	ErrCapacity        = errors.New("async execution manager capacity exceeded")
	ErrInvalidResource = errors.New("async execution resource and inspector are required")
	ErrStopped         = errors.New("async execution stop requested")
)
