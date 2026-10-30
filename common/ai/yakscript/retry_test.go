package yakscript

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRetryHints(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	require.Equal(t, 90*time.Second, parseRetryAfter(" 90 ", now))
	require.Equal(t, 15*time.Minute, parseRetryAfter(now.Add(15*time.Minute).Format(http.TimeFormat), now))
	for _, bad := range []string{"", "-1", "invalid", now.Add(-time.Minute).Format(http.TimeFormat)} {
		require.Zero(t, parseRetryAfter(bad, now))
	}
	req := &Request{}
	require.Error(t, Retry(req, "busy", "900"))
	require.Equal(t, 15*time.Minute, req.retry.delay)
	require.Equal(t, "busy", req.retry.reason)
}

func TestTransientFailureClassification(t *testing.T) {
	for _, err := range []error{io.EOF, io.ErrUnexpectedEOF, context.DeadlineExceeded, syscall.ECONNRESET} {
		require.True(t, IsTransientError(fmt.Errorf("request: %w", err)))
	}
	for _, err := range []error{nil, context.Canceled, errors.New("unknown business failure")} {
		require.False(t, IsTransientError(err))
	}
}
