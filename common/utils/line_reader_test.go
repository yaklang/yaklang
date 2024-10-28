package utils

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFileReader(t *testing.T) {
	data := []byte("hello\nworld")
	fd, err := os.CreateTemp("", "file-reader-test")
	require.NoError(t, err)
	defer func() {
		fd.Close()
		os.Remove(fd.Name())
	}()
	_, err = fd.Write(data)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	reader, err := FileLineReaderWithContext(fd.Name(), ctx)
	require.NoError(t, err)
	for _, line := range bytes.Split(data, []byte("\n")) {
		require.Equal(t, string(line), string(<-reader))
	}
	require.NoError(t, ctx.Err())
}
