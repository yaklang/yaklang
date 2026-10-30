package aispec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
)

// ModelCallFailure describes a failed provider attempt, before Agent action validation.
// Kind is network, http, protocol, business, configuration, or script.
type ModelCallFailure struct {
	Kind       string
	StatusCode int
	Code       string
	RetryAfter string
	Cause      error
	Attempt    int
	Decision   *RetryDecision
}

func (f *ModelCallFailure) Error() string {
	return fmt.Sprintf("model %s failure (status=%d code=%s): %v", f.Kind, f.StatusCode, f.Code, f.Cause)
}
func (f *ModelCallFailure) Unwrap() error { return f.Cause }

// RetryDecision is a decision only; aispec owns waiting, admission and replay.
type RetryDecision struct {
	Retry      bool
	Reason     string
	RetryAfter string
}

// Retry asks the model request executor to retry. retryAfter accepts seconds or
// an HTTP date. It never overrides cancellation or shortens a server Retry-After.
func Retry(reason, retryAfter string) *RetryDecision {
	return &RetryDecision{Retry: true, Reason: reason, RetryAfter: retryAfter}
}

// StopRetry rejects replay of a permanent provider failure.
func StopRetry(reason string) *RetryDecision { return &RetryDecision{Reason: reason} }

// IsTransientError recognizes interrupted reads, timeouts and temporary network
// failures. HTTP status and provider business errors must be reported separately.
func IsTransientError(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	for _, temporary := range []error{syscall.ECONNRESET, syscall.ECONNREFUSED, syscall.ECONNABORTED, syscall.EPIPE, syscall.ENETUNREACH, syscall.EHOSTUNREACH} {
		if errors.Is(err, temporary) {
			return true
		}
	}
	var network net.Error
	return errors.As(err, &network) && network.Timeout()
}

func ClassifyModelFailure(err error, status int, retryAfter string) *ModelCallFailure {
	var failure *ModelCallFailure
	if errors.As(err, &failure) {
		clone := *failure
		return &clone
	}
	kind := "network"
	if status >= 400 {
		kind = "http"
	}
	return &ModelCallFailure{Kind: kind, StatusCode: status, RetryAfter: retryAfter, Cause: err}
}

func (f *ModelCallFailure) CanRetry() bool {
	if errors.Is(f.Cause, context.Canceled) {
		return false
	}
	if f.Kind == "configuration" || f.Kind == "script" {
		return false
	}
	if f.StatusCode >= 400 && f.StatusCode < 500 && f.StatusCode != 408 && f.StatusCode != 409 && f.StatusCode != 425 && f.StatusCode != 429 {
		return false
	}
	if f.Decision != nil {
		return f.Decision.Retry
	}
	// Unknown business failures need an adapter decision. A valid envelope alone
	// cannot establish that replaying an upstream workflow is safe.
	return f.Kind != "business"
}
