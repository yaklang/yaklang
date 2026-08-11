package jsonextractor

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRawValueFormatterPreservesEmptyContainerKinds(t *testing.T) {
	objectKind, _, objectValue, objectArray := rawValueFormatter(map[string]any{})
	require.Equal(t, RAW_VALUE_TYPE_MAP, objectKind)
	require.NotNil(t, objectValue)
	require.Nil(t, objectArray)

	arrayKind, _, arrayObject, arrayValue := rawValueFormatter(map[int]any{})
	require.Equal(t, RAW_VALUE_TYPE_ARR, arrayKind)
	require.Nil(t, arrayObject)
	require.NotNil(t, arrayValue)
	require.Empty(t, arrayValue)
}

func TestStructuredExtractorPreservesEmptyContainerKinds(t *testing.T) {
	formatted := make(map[string]any)
	rawKinds := make(map[string]string)
	require.NoError(t, ExtractStructuredJSON(
		`{"object":{},"array":[]}`,
		WithRawKeyValueCallback(func(key, data any) {
			rawKinds[rawKeyFormatted(key)] = reflect.TypeOf(data).String()
		}),
		WithFormatKeyValueCallback(func(key, data any, _ []string) {
			formatted[rawKeyFormatted(key)] = data
		}),
	))
	require.IsType(t, map[string]any{}, formatted["object"], "raw kinds: %#v", rawKinds)
	require.IsType(t, []any{}, formatted["array"], "raw kinds: %#v", rawKinds)
}
