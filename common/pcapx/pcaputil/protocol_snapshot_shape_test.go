package pcaputil

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProtocolSnapshotPreservesNullCollections(t *testing.T) {
	for name, value := range map[string]any{
		"null-map": map[string]any(nil), "empty-map": map[string]any{},
		"null-list": []any(nil), "empty-list": []any{},
		"null-maps": []map[string]any(nil), "empty-maps": []map[string]any{},
		"null-strings": []string(nil), "empty-strings": []string{},
		"null-int32": []int32(nil), "empty-int32": []int32{},
		"null-uint16": []uint16(nil), "empty-uint16": []uint16{},
		"null-uint64": []uint64(nil), "empty-uint64": []uint64{},
		"null-bytes": []byte(nil), "empty-bytes": []byte{},
		"nested": map[string]any{"null": []map[string]any(nil), "empty": []map[string]any{}},
	} {
		t.Run(name, func(t *testing.T) {
			cloned := cloneSessionValue(value)
			require.True(t, reflect.DeepEqual(value, cloned), "snapshot changed typed null/empty collection")
			before, err := json.Marshal(value)
			require.NoError(t, err)
			after, err := json.Marshal(cloned)
			require.NoError(t, err)
			require.Equal(t, before, after, "snapshot changed exported JSON shape")
		})
	}
	input := map[string]any{"nested": []map[string]any{{"wire": []byte("owned")}}}
	copy := cloneSession(input)
	copy["nested"].([]map[string]any)[0]["wire"].([]byte)[0] = 'X'
	require.Equal(t, "owned", string(input["nested"].([]map[string]any)[0]["wire"].([]byte)))
}
