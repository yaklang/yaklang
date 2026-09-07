package stream_parser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBoundedJSONTextPreservesTypesOrderAndNumbers(t *testing.T) {
	text := ` {"a": [true, null, -0, 9007199254740993, 1e400], "b": {"\u006b":"a\\b\"c\n\u4e2d"}} `
	value, err := decodeJSONText(text, 4)
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, value.Keys)
	require.Equal(t, strings.TrimSpace(text), value.Raw)
	require.Equal(t, []string{"k"}, value.Members["b"].Keys)
	require.Equal(t, "a\\b\"c\n中", value.Members["b"].Members["k"].StringValue)
	array := value.Members["a"].Elements
	require.Len(t, array, 5)
	require.True(t, array[0].BoolValue)
	require.Equal(t, "null", array[1].Kind)
	for i, number := range []string{"-0", "9007199254740993", "1e400"} {
		require.Equal(t, "number", array[i+2].Kind)
		require.Equal(t, number, array[i+2].Raw)
		require.Equal(t, number, array[i+2].NumberValue)
	}
	for _, text := range []string{`{}`, `[]`, `""`, `0`, `false`, `null`} {
		_, err := decodeJSONText(text, 1)
		require.NoError(t, err)
	}
	for _, text := range []string{`"\ud83d\ude00"`, `"\\ud800"`, `{"\ud83d\ude00":0}`} {
		_, err := decodeJSONText(text, 4)
		require.NoError(t, err)
	}
}

func TestBoundedJSONTextRejectsInvalidAndAmbiguousDocuments(t *testing.T) {
	for _, text := range []string{"", `{"x":1,"x":2}`, `{"x":1,"\u0078":2}`, `{"x": [1,]}`, `{"x":01}`, `{"x":+1}`, `{"x":NaN}`, `{"x":tru}`, `{"x":"\q"}`, "\"line\nfeed\"", `{} []`, `{}x`, string([]byte{'"', 0xff, '"'}), `"\ud800"`, `"\udfff"`, `"\ud800\u0041"`, `{"\ud800":0}`} {
		_, err := decodeJSONText(text, 64)
		require.Error(t, err, "%q", text)
	}
	_, err := decodeJSONText(`[[0]]`, 2)
	require.ErrorContains(t, err, "depth")
	for _, limit := range []int{0, 129} {
		_, err := decodeJSONText(`0`, limit)
		require.Error(t, err)
	}
	_, err = decodeJSONText(strings.Repeat(" ", 1<<20)+"0", 64)
	require.ErrorContains(t, err, "input exceeds limit")
	_, err = decodeJSONText("["+strings.Repeat("0,", 100000)+"0]", 64)
	require.ErrorContains(t, err, "value limit")
}
