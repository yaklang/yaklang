package errtype

import (
	"fmt"
	"strings"
)

var ErrContextCanceleld = fmt.Errorf("context canceled")

var _ error = (*ErrContextCanceled)(nil)

type ErrContextCanceled struct {
	msg string
}

func (e *ErrContextCanceled) Error() string {
	return fmt.Sprintf("context canceled: %s", e.msg)
}

func NewContextCanceled(err ...string) *ErrContextCanceled {
	return &ErrContextCanceled{msg: strings.Join(err, " ")}
}
