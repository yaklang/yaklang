package builtin

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type printlnErrorWriter struct{ err error }

func (w printlnErrorWriter) Write(p []byte) (int, error) { return 0, w.err }

func TestPrintlnToFormattingAndErrors(t *testing.T) {
	var output bytes.Buffer
	n, err := PrintlnTo(&output, []byte("hello"), uint8(65), uint8(1))
	require.NoError(t, err)
	require.Equal(t, "hello 'A' '\\x01'\n", output.String())
	require.Equal(t, output.Len(), n)
	failure := errors.New("writer failed")
	n, err = PrintlnTo(printlnErrorWriter{failure}, "value")
	require.Zero(t, n)
	require.ErrorIs(t, err, failure)
	output.Reset()
	n, err = PrintlnTo(&output)
	require.NoError(t, err)
	require.Equal(t, "\n", output.String())
	require.Equal(t, 1, n)
}

func TestPrintToPreservesFormatAndErrors(t *testing.T) {
	var output bytes.Buffer
	n, err := PrintTo(&output, []byte("ab"), uint8(65))
	require.NoError(t, err)
	require.Equal(t, "[97 98] 65", output.String())
	require.Equal(t, output.Len(), n)
	output.Reset()
	n, err = PrintfTo(&output, "%s=%02d", "value", 3)
	require.NoError(t, err)
	require.Equal(t, "value=03", output.String())
	require.Equal(t, output.Len(), n)
	failure := errors.New("writer failed")
	n, err = PrintTo(printlnErrorWriter{failure}, "value")
	require.Zero(t, n)
	require.ErrorIs(t, err, failure)
	n, err = PrintfTo(printlnErrorWriter{failure}, "%s", "value")
	require.Zero(t, n)
	require.ErrorIs(t, err, failure)
}
