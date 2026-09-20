// Package scanerr defines stable, errors.Is/As-recognizable scan failure classes.
package scanerr

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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
	Err  error
}

func New(code, format string, args ...any) error {
	return &Error{Code: code, Err: fmt.Errorf(format, args...)}
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	msg := ""
	if e.Err != nil {
		msg = e.Err.Error()
	}
	if e.Code == "" {
		return msg
	}
	if msg == "" {
		return e.Code
	}
	if strings.HasPrefix(msg, e.Code) {
		return msg
	}
	return e.Code + ": " + msg
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
	ErrResourceLimit = &Error{Code: ResourceLimit}
	ErrInputChanged  = &Error{Code: InputChanged}
	ErrInvalidPath   = &Error{Code: InvalidPath}
	ErrCancelled     = &Error{Code: Cancelled}
)

// CodeOf returns the stable diagnostic code for err across scan stages.
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
	if errors.Is(err, fs.ErrPermission) {
		return InvalidPath
	}
	msg := err.Error()
	for _, code := range []string{ResourceLimit, InputChanged, InvalidPath, InternalError, UnsupportedSyntax, EvidenceInsufficient, MalformedInput, InvalidInput, InvalidConfig, UnsupportedInput, InputError, Cancelled} {
		if strings.Contains(msg, code) {
			return code
		}
	}
	if strings.Contains(msg, "resource limit") {
		return ResourceLimit
	}
	return MalformedInput
}
