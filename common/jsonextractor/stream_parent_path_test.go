package jsonextractor

import (
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFieldStreamParentsExcludePrecedingSiblingFields(t *testing.T) {
	var mu sync.Mutex
	parentsByValue := make(map[string][]string)
	var readErr error
	err := ExtractStructuredJSON(`{"identifier":"greet_reply","answer_payload":"outer","nested":{"answer_payload":"inner"}}`,
		WithRegisterFieldStreamHandler("answer_payload", func(_ string, reader io.Reader, parents []string) {
			value, err := io.ReadAll(reader)
			mu.Lock()
			if readErr == nil {
				readErr = err
			}
			parentsByValue[string(value)] = append([]string(nil), parents...)
			mu.Unlock()
		}))
	require.NoError(t, err)
	require.NoError(t, readErr)
	require.Contains(t, parentsByValue, `"outer"`)
	require.Empty(t, parentsByValue[`"outer"`])
	require.Equal(t, []string{"nested"}, parentsByValue[`"inner"`])
}
