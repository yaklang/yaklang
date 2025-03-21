package aid

import "fmt"

type TaskStackError struct {
	retryable bool
	err       error
	ToolName  string
}

func IsRetryableTaskStackError(err error) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*TaskStackError); ok {
		return e.IsRetryable()
	}
	return false
}

func (e *TaskStackError) IsRetryable() bool {
	if e == nil {
		return false
	}
	return e.retryable
}

func NewRetryableTaskStackError(err error) *TaskStackError {
	return &TaskStackError{
		retryable: true,
		err:       err,
	}
}

func NewNonRetryableTaskStackError(err error) *TaskStackError {
	return &TaskStackError{
		retryable: false,
		err:       err,
	}
}

func (e *TaskStackError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("aiTask stack error: %v", e.err)
}
