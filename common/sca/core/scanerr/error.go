// Package scanerr defines stable, errors.Is/As-recognizable scan failure classes.
package scanerr

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	ResourceLimit        = "resource_limit"
	InputError           = "input_error"
	InputChanged         = "input_changed"
	InvalidPath          = "invalid_path"
	Cancelled            = "cancelled"
	MalformedInput       = "malformed_input"
	UnsupportedSyntax    = "unsupported_syntax"
	EvidenceInsufficient = "evidence_insufficient"
	InternalError        = "internal_error"
	InvalidInput         = "invalid_input"
	InvalidConfig        = "invalid_config"
	UnsupportedInput     = "unsupported_input"
)

// Error is a classified failure. Callers should use CodeOf or errors.Is/As,
// not parse Reason strings.
type Error struct {
	Code string
	File string
	Err  error
}

func New(code, format string, args ...any) error {
	return &Error{Code: code, Err: fmt.Errorf(format, args...)}
}

func WithFile(err error, file string) error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		if e.File == file {
			return err
		}
		cp := *e
		cp.File = file
		return &cp
	}
	return &Error{Code: CodeOf(err), File: file, Err: err}
}

// Wrap keeps an existing classified error. Otherwise it attaches code while
// preserving unwrap targets such as context.Canceled.
func Wrap(code string, err error) error {
	if err == nil {
		if code == "" {
			return nil
		}
		return &Error{Code: code}
	}
	var e *Error
	if errors.As(err, &e) && e.Code != "" {
		return err
	}
	if code == "" {
		return err
	}
	return &Error{Code: code, Err: err}
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	msg := ""
	if e.Err != nil {
		msg = e.Err.Error()
	}
	if e.Code != "" && (msg == e.Code || strings.HasPrefix(msg, e.Code+": ")) {
		msg = strings.TrimPrefix(strings.TrimPrefix(msg, e.Code+": "), e.Code)
	}
	switch {
	case e.Code != "" && e.File != "" && msg != "":
		return e.Code + ": " + e.File + ": " + msg
	case e.Code != "" && e.File != "":
		return e.Code + ": " + e.File
	case e.Code != "" && msg != "":
		return e.Code + ": " + msg
	case e.Code != "":
		return e.Code
	case e.File != "" && msg != "":
		return e.File + ": " + msg
	default:
		return msg
	}
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && e != nil && t != nil && e.Code == t.Code
}

var (
	ErrResourceLimit  = &Error{Code: ResourceLimit}
	ErrInputChanged   = &Error{Code: InputChanged}
	ErrInvalidPath    = &Error{Code: InvalidPath}
	ErrCancelled      = &Error{Code: Cancelled}
	ErrMalformedInput = &Error{Code: MalformedInput}
)

// CodeOf returns the stable diagnostic code for err across scan stages.
// String matching is only a prefix compatibility path for historical
// fmt.Errorf("%s: ...") producers, never a substring search of paths or reasons.
func CodeOf(err error) string {
	if err == nil {
		return ""
	}
	var e *Error
	if errors.As(err, &e) && e.Code != "" {
		return e.Code
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return Cancelled
	}
	msg := err.Error()
	for _, code := range []string{ResourceLimit, InputChanged, InvalidPath, InternalError, UnsupportedSyntax, EvidenceInsufficient, MalformedInput, InvalidInput, InvalidConfig, UnsupportedInput, InputError, Cancelled} {
		if msg == code || strings.HasPrefix(msg, code+":") {
			return code
		}
	}
	return MalformedInput
}
