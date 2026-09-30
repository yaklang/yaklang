package utils

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStableReaderCompletedStreamReturnsAllBytes(t *testing.T) {
	payload := strings.Repeat("completed-stream", 4096)
	start := time.Now()
	got := StableReader(strings.NewReader(payload), 5*time.Second, len(payload)+1)
	require.Equal(t, payload, string(got))
	require.Less(t, time.Since(start), 500*time.Millisecond, "EOF must release a completed reader without a stability wait")
}

func TestStableReaderFragmentedStreamRetainsFinalChunk(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	done := make(chan error, 1)
	go func() {
		for _, fragment := range []string{"first", "-middle", "-last"} {
			if _, err := writer.Write([]byte(fragment)); err != nil {
				done <- err
				return
			}
		}
		done <- writer.Close()
	}()
	require.Equal(t, []byte("first-middle-last"), StableReader(reader, time.Second, 1024))
	require.NoError(t, <-done)
}

func TestStableReaderDeadlineRetainsReceivedBytes(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	done := make(chan error, 1)
	go func() { _, err := writer.Write(bytes.Repeat([]byte("x"), 10)); done <- err }()
	require.Equal(t, strings.Repeat("x", 10), string(StableReader(reader, 50*time.Millisecond, 1024)))
	require.NoError(t, <-done)
}

func TestStableReaderOpenStreamPreservesStabilityWindow(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	done := make(chan error, 1)
	go func() { _, err := writer.Write([]byte("open-stream")); done <- err }()
	start := time.Now()
	require.Equal(t, "open-stream", string(StableReader(reader, 3*time.Second, 1024)))
	require.GreaterOrEqual(t, time.Since(start), 500*time.Millisecond, "an open stream must still be observed for stability")
	require.NoError(t, <-done)
}
