package aicommon

import "errors"

// RecoverableError 标记可以由 ReAct 循环通过 Timeline 或纠正重试自愈的错误。
// 未包装的普通 error 不会自动进入自愈流程。
type RecoverableError struct{ cause error }

func (e *RecoverableError) Error() string { return e.cause.Error() }
func (e *RecoverableError) Unwrap() error { return e.cause }

func NewRecoverableError(err error) error {
	if err == nil {
		return nil
	}
	return &RecoverableError{cause: err}
}

func IsRecoverableError(err error) bool {
	var target *RecoverableError
	return errors.As(err, &target)
}
