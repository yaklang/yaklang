package yakscript

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type retryDecision struct {
	reason string
	delay  time.Duration
}

// Retry marks the current attempt as transient and returns an error for the
// script to return. The side channel also survives Yak's panic stack wrapping;
// the global recovery policy separately decides whether failures keep retrying.
func Retry(req *Request, reason, retryAfter string) error {
	decision := &retryDecision{reason: reason, delay: parseRetryAfter(retryAfter, time.Now())}
	req.retryMu.Lock()
	req.retry = decision
	req.retryMu.Unlock()
	return fmt.Errorf("temporary model failure: %s", reason)
}

// IsTransientError deliberately excludes configuration, authentication and
// certificate errors. Adapters decide which protocol/business errors can retry.
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

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseUint(value, 10, 32); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if deadline, err := http.ParseTime(value); err == nil && deadline.After(now) {
		return deadline.Sub(now)
	}
	return 0
}
