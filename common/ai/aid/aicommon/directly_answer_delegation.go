package aicommon

import "errors"

// ErrDirectlyAnswerDelegatedToMainLoop means no standalone AI request was
// issued. The active loop received a Timeline stage-summary request and will
// make the next decision through its normal action schema.
var ErrDirectlyAnswerDelegatedToMainLoop = errors.New("directly answer delegated to active main loop")

func IsDirectlyAnswerDelegatedToMainLoop(err error) bool {
	return errors.Is(err, ErrDirectlyAnswerDelegatedToMainLoop)
}
