package utils

import (
	"fmt"
	"io"

	"github.com/samber/lo"
)

type YakError struct {
	msg            string
	originalErrors []error
	typ            any
	*stack
}

func Error(i interface{}) error {
	switch t := i.(type) {
	case string:
		return &YakError{msg: t, originalErrors: nil, stack: callers()}
	default:
		return &YakError{msg: fmt.Sprint(i), originalErrors: nil, stack: callers()}
	}
}

func Errorf(format string, args ...interface{}) error {
	oErr := fmt.Errorf(format, args...)
	return &YakError{
		msg:            oErr.Error(),
		originalErrors: []error{oErr},
		stack:          callers(),
	}
}

func JoinErrors(errs ...error) error {
	errs = lo.Filter(errs, func(err error, _ int) bool {
		return err != nil
	})
	if len(errs) == 0 {
		return nil
	}

	msg := ""
	var st *stack = &stack{st: make([]uintptr, 0, 1)}
	st.appendCurrentFrame()

	newErrs := make([]error, 0, len(errs))

	lenOfErrors := len(errs)
	for i, err := range errs {
		msg += err.Error()
		if i < lenOfErrors-1 {
			msg += " | "
		}
		if yakError, ok := err.(*YakError); ok {
			newErrs = append(newErrs, yakError.originalErrors...)
			st.appendEmptyFrame()
			st.appendStack(yakError.stack)
		} else {
			newErrs = append(newErrs, err)
		}
	}

	return &YakError{
		msg:            msg,
		originalErrors: errs,
		stack:          st,
	}
}

func Wrap(err error, msg string) error {
	if err == nil {
		return nil
	}
	if msg != "" {
		msg += ": "
	}

	var st *stack = nil

	if yakErr, ok := err.(*YakError); ok {
		st = &stack{st: make([]uintptr, len(yakErr.stack.st))}
		copy(st.st, yakErr.stack.st)
	} else {
		st = &stack{st: make([]uintptr, 0)}
	}
	st.appendCurrentFrame()

	return &YakError{msg: fmt.Sprintf("%s%s", msg, err.Error()), originalErrors: []error{err}, stack: st}
}

func Wrapf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	msg := fmt.Sprintf(format, args...)
	if msg != "" {
		msg += ": "
	}

	var st *stack = nil

	if yakErr, ok := err.(*YakError); ok {
		st = &stack{st: make([]uintptr, len(yakErr.stack.st))}
		copy(st.st, yakErr.stack.st)
	} else {
		st = &stack{st: make([]uintptr, 0)}
	}
	st.appendCurrentFrame()

	return &YakError{msg: fmt.Sprintf("%s%s", msg, err.Error()), originalErrors: []error{err}, stack: st}
}

func (err *YakError) Cause() []error {
	return err.originalErrors
}

func (err *YakError) Error() string {
	return err.msg
}

func (err *YakError) Unwrap() []error {
	return err.originalErrors
}

func (e *YakError) Is(rerr error) bool {
	if yakErr, ok := rerr.(*YakError); ok {
		return e == yakErr
	}

	return false
}

func (err *YakError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('#') {
			io.WriteString(s, err.Error())
			fmt.Fprintf(s, "%+v", err.stack)
		} else if s.Flag('+') {
			fmt.Fprintf(s, "%+v", err.stack)
		} else {
			io.WriteString(s, err.Error())
		}
	case 's':
		io.WriteString(s, err.Error())
	case 'q':
		fmt.Fprintf(s, "%q", err.Error())
	}
}
