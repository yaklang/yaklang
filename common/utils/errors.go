package utils

import (
	"fmt"
	"io"

	"github.com/samber/lo"
)

type YakError struct {
	msg            string
	originalErrors []error
	*stack
}

func Error(i interface{}) error {
	switch t := i.(type) {
	case string:
		return YakError{msg: t, originalErrors: nil, stack: callers()}
	default:
		return YakError{msg: fmt.Sprint(i), originalErrors: nil, stack: callers()}
	}
}

func Errorf(format string, args ...interface{}) error {
	oErr := fmt.Errorf(format, args...)
	return YakError{
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
	var st *stack = nil
	newErrs := make([]error, 0, len(errs))

	lenOfErrors := len(errs)
	for i, err := range errs {
		msg += err.Error()
		if i < lenOfErrors-1 {
			msg += ": "
		}
		if yakError, ok := err.(YakError); ok {
			newErrs = append(newErrs, yakError.originalErrors...)
			if st == nil {
				st = yakError.stack
				st.appendCurrentFrame()
			} else {
				st.appendEmptyFrame()
				st.appendStack(yakError.stack)
			}
		} else {
			newErrs = append(newErrs, err)
		}
	}

	if st == nil {
		st = callers()
	}

	return YakError{
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
	if yakErr, ok := err.(YakError); ok {
		yakErr.msg = fmt.Sprintf("%s%s", msg, yakErr.Error())
		yakErr.stack.appendCurrentFrame()
		yakErr.originalErrors = append(yakErr.originalErrors, err)
		return yakErr
	}

	return YakError{msg: fmt.Sprintf("%s%s", msg, err.Error()), originalErrors: []error{err}, stack: callers()}
}

func Wrapf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	msg := fmt.Sprintf(format, args...)
	if msg != "" {
		msg += ": "
	}

	if yakErr, ok := err.(YakError); ok {
		yakErr.msg = fmt.Sprintf("%s%s", msg, yakErr.Error())
		yakErr.stack.appendCurrentFrame()
		yakErr.originalErrors = append(yakErr.originalErrors, err)
		return yakErr
	}

	return YakError{msg: fmt.Sprintf("%s%s", msg, err.Error()), originalErrors: []error{err}, stack: callers()}
}

func (err YakError) Cause() []error {
	return err.originalErrors
}

func (err YakError) Error() string {
	return err.msg
}

func (err YakError) Unwrap() []error {
	return err.originalErrors
}

func (err YakError) Format(s fmt.State, verb rune) {
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
