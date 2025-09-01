package utils

import (
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"strings"
	"syscall"

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

type RequireTestingT struct {
	AssertTestingT
	FalNow func()
}

func NewRequireTestT(a AssertTestingT, f func()) *RequireTestingT {
	return &RequireTestingT{AssertTestingT: a, FalNow: f}
}

func (r *RequireTestingT) FailNow() {
	r.FalNow()
}

type AssertTestingT func(msg string, args ...any)

func (a AssertTestingT) Errorf(format string, args ...interface{}) {
	if a == nil {
		return
	}
	a(format, args...)
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

	var st *stack = &stack{st: make([]uintptr, 0, 1)}
	st.appendCurrentFrame()

	newErrs := make([]error, 0, len(errs))

	for _, err := range errs {
		if yakError, ok := err.(*YakError); ok {
			newErrs = append(newErrs, yakError.originalErrors...)
			st.appendEmptyFrame()
			st.appendStack(yakError.stack)
		} else {
			newErrs = append(newErrs, err)
		}
	}

	return &YakError{
		msg:            "", // msg will be built on demand in Error()
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
	if err == nil {
		return "<nil>"
	}
	if err.msg != "" {
		return err.msg
	}
	if len(err.originalErrors) > 0 {
		var sb strings.Builder
		for i, e := range err.originalErrors {
			if e == nil {
				continue
			}
			sb.WriteString(e.Error())
			if i < len(err.originalErrors)-1 {
				sb.WriteString(" | ")
			}
		}
		err.msg = sb.String()
		return err.msg
	}
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

func (e *YakError) ErrorWithStack() string {
	return fmt.Sprintf("%s\n%+v", e.Error(), e.stack)
}

func (err *YakError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('#') || s.Flag('+') {
			io.WriteString(s, err.Error())
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

// ErrorStack 捕获 panic 并返回 error
func ErrorStack(origin any) (err error) {
	// 收集调用栈信息，跳过前3个栈帧
	var pcs [32]uintptr
	n := runtime.Callers(3, pcs[:])

	// 获取对应的调用栈帧信息
	frames := runtime.CallersFrames(pcs[:n])

	// 构建错误信息
	var sb strings.Builder
	fmt.Fprintf(&sb, "panic: %v\n", origin)
	fmt.Fprintf(&sb, "stack trace:\n")

	// 遍历并记录函数调用栈
	for {
		frame, more := frames.Next()
		fmt.Fprintf(&sb, "    %s\n        %s:%d\n",
			frame.Function, frame.File, frame.Line)
		if !more {
			break
		}
	}

	// 返回包含堆栈信息的错误
	var originErr error
	switch origin.(type) {
	case error:
		originErr = origin.(error)
	default:
		originErr = Error(origin)
	}
	err = Wrapf(originErr, "%s", sb.String())
	return
}

func IsConnectResetError(err error) bool { // check net error like "an existing connection was forcibly closed by the remote host"
	if opErr, ok := err.(*net.OpError); ok {
		if opErr.Err == syscall.ECONNRESET {
			return true
		} else if runtime.GOOS == "windows" {
			if se, ok := opErr.Err.(*os.SyscallError); ok {
				if errno, ok := se.Err.(syscall.Errno); ok {
					if errno == 10054 {
						return true
					}
				}
			}
		}
	}
	return false
}
