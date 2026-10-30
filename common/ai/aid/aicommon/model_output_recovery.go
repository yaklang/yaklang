package aicommon

import "errors"

// ModelOutputValidationError is used only by action parsing/selection before
// executing an action. Arbitrary post-handler/tool errors must not enter an
// unlimited model-regeneration loop.
type ModelOutputValidationError struct{ cause error }

func (e *ModelOutputValidationError) Error() string { return e.cause.Error() }
func (e *ModelOutputValidationError) Unwrap() error { return e.cause }
func NewModelOutputValidationError(err error) error {
	if err == nil {
		return nil
	}
	return &ModelOutputValidationError{cause: err}
}
func IsModelOutputValidationError(err error) bool {
	var target *ModelOutputValidationError
	return errors.As(err, &target)
}
