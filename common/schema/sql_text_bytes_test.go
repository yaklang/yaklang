package schema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSQLTextBytesPreservesTextAndWireEncoding(t *testing.T) {
	var payload SQLTextBytes
	for _, value := range []interface{}{`{"中文":true}`, []byte(`{"中文":true}`)} {
		require.NoError(t, payload.Scan(value))
		bound, err := payload.Value()
		require.NoError(t, err)
		require.Equal(t, `{"中文":true}`, bound)
		got, err := json.Marshal(payload)
		require.NoError(t, err)
		want, err := json.Marshal([]byte(payload))
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
	require.NoError(t, payload.Scan(nil))
	bound, err := payload.Value()
	require.NoError(t, err)
	require.Nil(t, bound)
}
