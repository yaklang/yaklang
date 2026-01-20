package jsonextractor

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type failingReader struct {
	err error
}

func (r *failingReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

func TestExtractStructuredJSONFromStream_StreamFinishedCallback(t *testing.T) {
	var finishedCalled bool
	var errorCalled bool

	err := ExtractStructuredJSONFromStream(
		strings.NewReader(`{"a":1}`),
		WithStreamFinishedCallback(func() { finishedCalled = true }),
		WithStreamErrorCallback(func(_ error) { errorCalled = true }),
	)
	require.NoError(t, err)
	require.True(t, finishedCalled)
	require.False(t, errorCalled)
}

func TestExtractStructuredJSONFromStream_StreamErrorCallback(t *testing.T) {
	var finishedCalled bool
	var gotErr error

	want := errors.New("read boom")
	err := ExtractStructuredJSONFromStream(
		&failingReader{err: want},
		WithStreamFinishedCallback(func() { finishedCalled = true }),
		WithStreamErrorCallback(func(e error) { gotErr = e }),
	)

	require.ErrorIs(t, err, want)
	require.ErrorIs(t, gotErr, want)
	require.False(t, finishedCalled)
}
