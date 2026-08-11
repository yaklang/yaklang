package aicommon

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestActionWaitParseResultRejectsTruncatedCanonicalObject(t *testing.T) {
	raw := `{
		"@action":"batch",
		"calls":[
			{"tool_name":"first","params":{"path":"/a"}},
			{"tool_name":"second","params":`

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	action, err := ExtractActionFromStream(ctx, strings.NewReader(raw), "batch")
	require.NoError(t, err, "the streaming constructor must remain asynchronous")
	require.ErrorContains(t, action.WaitParseResult(ctx), "complete canonical object")
	// A flattened callback can observe @action before the complete root object.
	// Its presence must not turn the partial response into an executable action.
	require.True(t, action.ValidCheck("batch"))
	_, exists := action.LookupCanonicalParam("calls")
	require.False(t, exists)
}

func TestActionParseErrorPropagatesThroughSynchronousExtractors(t *testing.T) {
	raw := `{"@action":"batch","calls":[{"tool_name":"first"},`

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	action, err := ExtractValidActionFromStream(ctx, strings.NewReader(raw), "batch")
	require.Nil(t, action)
	require.ErrorContains(t, err, "complete canonical object")

	action, err = ExtractAction(raw, "batch")
	require.Nil(t, action)
	require.ErrorContains(t, err, "complete canonical object")
}

func TestActionWaitParseResultAcceptsCompleteCanonicalObject(t *testing.T) {
	action, err := ExtractActionFromStream(
		context.Background(),
		strings.NewReader(`{"@action":"batch","calls":[]}`),
		"batch",
	)
	require.NoError(t, err)
	require.NoError(t, action.WaitParseResult(context.Background()))

	items, exists, err := action.GetCanonicalObjectArray("calls")
	require.NoError(t, err)
	require.True(t, exists)
	require.Empty(t, items)
}

func TestActionWaitParseResultReportsCallerCancellationBeforeParserCompletion(t *testing.T) {
	reader, writer := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	action, err := ExtractActionFromStream(ctx, reader, "batch")
	require.NoError(t, err)
	cancel()
	require.ErrorIs(t, action.WaitParseResult(ctx), context.Canceled)

	// Release the parser goroutine and make sure the eventual parser completion
	// remains observable independently from the cancelled construction context.
	require.NoError(t, writer.Close())
	parseCtx, parseCancel := context.WithTimeout(context.Background(), time.Second)
	defer parseCancel()
	require.NoError(t, action.WaitParseResult(parseCtx))
}

func TestDecodeStrictObjectArray(t *testing.T) {
	tests := []struct {
		name    string
		raw     any
		wantErr string
		wantLen int
	}{
		{name: "null", raw: nil, wantErr: "got null"},
		{name: "typed null", raw: []any(nil), wantErr: "got null"},
		{name: "scalar string", raw: "tool", wantErr: "got string"},
		{name: "scalar number", raw: 1, wantErr: "got int"},
		{name: "object is not array", raw: map[string]any{"tool_name": "one"}, wantErr: "got map"},
		{name: "null item", raw: []any{nil}, wantErr: "item 0 must be an object, got null"},
		{name: "string item", raw: []any{map[string]any{"tool_name": "one"}, "two"}, wantErr: "item 1 must be an object, got string"},
		{name: "array item", raw: []any{[]any{}}, wantErr: "item 0 must be an object, got slice"},
		{name: "non-string object keys", raw: []any{map[int]any{1: "one"}}, wantErr: "string keys"},
		{name: "empty", raw: []any{}, wantLen: 0},
		{name: "invoke params", raw: []aitool.InvokeParams{{"tool_name": "one"}}, wantLen: 1},
		{name: "objects", raw: []any{map[string]any{"tool_name": "one"}, map[string]any{"tool_name": "two"}}, wantLen: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			items, err := DecodeStrictObjectArray(test.raw)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				require.Nil(t, items)
				return
			}
			require.NoError(t, err)
			require.Len(t, items, test.wantLen)
		})
	}
}

func TestCanonicalObjectArrayDoesNotUseFlattenedNestedFields(t *testing.T) {
	raw := `{
		"@action":"batch",
		"calls":[
			{"tool_name":"first","params":{"path":"/a"}},
			{"tool_name":"second","params":{"path":"/b"}}
		]
	}`
	action, err := ExtractAction(raw, "batch")
	require.NoError(t, err)

	// Legacy flattening exposes the first nested field under its simple key.
	require.Equal(t, "first", action.GetString("tool_name"))
	// A canonical lookup only sees fields on the action's root object.
	_, exists := action.LookupCanonicalParam("tool_name")
	require.False(t, exists)

	items, exists, err := action.GetCanonicalObjectArray("calls")
	require.NoError(t, err)
	require.True(t, exists)
	require.Len(t, items, 2)
	require.Equal(t, "first", items[0].GetString("tool_name"))
	require.Equal(t, "second", items[1].GetString("tool_name"))

	items[0]["tool_name"] = "mutated"
	canonicalRaw, exists := action.LookupCanonicalParam("calls")
	require.True(t, exists)
	canonical, err := DecodeStrictObjectArray(canonicalRaw)
	require.NoError(t, err)
	require.Equal(t, "first", canonical[0].GetString("tool_name"), "strict decoding must return independent item maps")
}

func TestGetCanonicalObjectArrayDistinguishesOmittedAndInvalid(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		exists  bool
		wantErr string
	}{
		{name: "omitted", payload: `{"@action":"batch"}`, exists: false},
		{name: "null", payload: `{"@action":"batch","calls":null}`, exists: true, wantErr: "got null"},
		{name: "scalar", payload: `{"@action":"batch","calls":"one"}`, exists: true, wantErr: "got string"},
		{name: "object", payload: `{"@action":"batch","calls":{"tool_name":"one"}}`, exists: true, wantErr: "got map"},
		{name: "non-object item", payload: `{"@action":"batch","calls":[{"tool_name":"one"},2]}`, exists: true, wantErr: "item 1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			action, err := ExtractAction(test.payload, "batch")
			require.NoError(t, err)
			items, exists, err := action.GetCanonicalObjectArray("calls")
			require.Equal(t, test.exists, exists)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				require.Nil(t, items)
				return
			}
			require.NoError(t, err)
			require.Nil(t, items)
		})
	}
}
