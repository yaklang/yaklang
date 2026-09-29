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

func TestActionGetParamsNativeAndTextCompatibility(t *testing.T) {
	const input = `{"@action":"grep_text","path":"/repo","pattern":"require_tool","limit":60,"options":{"case_sensitive":true},"calls":[{"name":"read"}]}`
	text, err := ExtractAction(input, "grep_text")
	require.NoError(t, err)
	native := NewSimpleAction("grep_text", aitool.InvokeParams{
		"@action": "grep_text", "path": "/repo", "pattern": "require_tool", "limit": float64(60),
		"options": map[string]any{"case_sensitive": true},
		"calls":   []any{map[string]any{"name": "read"}},
	})
	for _, action := range []*Action{text, native} {
		params := action.GetParams()
		require.Equal(t, "/repo", params.GetString("path"))
		require.Equal(t, "require_tool", params.GetString("pattern"))
		require.EqualValues(t, 60, params.GetInt("limit"))
		require.True(t, params.GetObject("options").GetBool("case_sensitive"))
		require.Len(t, params.GetObjectArray("calls"), 1)
	}
	// Native arguments are not a wrapper. A real field named "params" or ""
	// must not replace the canonical outer object.
	nested := NewSimpleAction("test", aitool.InvokeParams{
		"path": "/outer", "params": map[string]any{"path": "/inner"},
		"": map[string]any{"path": "/empty-key"},
	})
	require.Equal(t, "/outer", nested.GetParams().GetString("path"))
	forwarded := native.GetParams()
	delete(forwarded, "path")
	forwarded["runtime_id"] = "tool-only"
	require.Equal(t, "/repo", native.GetString("path"))
	_, exists := native.LookupCanonicalParam("runtime_id")
	require.False(t, exists)
	require.Empty(t, NewSimpleAction("empty", nil).GetParams())
	require.Empty(t, (*Action)(nil).GetParams())
}

func TestActionGetParamsTextStillWaitsForCanonicalObject(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()
	action := NewActionMaker("grep_text").ReadFromReader(ctx, r)
	firstWritten := make(chan error, 1)
	go func() {
		_, err := io.Copy(w, strings.NewReader(`{"@action":"grep_text","path":"/repo",`))
		firstWritten <- err
	}()
	require.NoError(t, <-firstWritten)
	result := make(chan aitool.InvokeParams, 1)
	go func() { result <- action.GetParams() }()
	select {
	case <-result:
		t.Fatal("GetParams returned the incremental cache before the root object completed")
	case <-time.After(40 * time.Millisecond):
	}
	_, err := io.WriteString(w, `"pattern":"ready"}`)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	select {
	case params := <-result:
		require.Equal(t, "/repo", params.GetString("path"))
		require.Equal(t, "ready", params.GetString("pattern"))
	case <-ctx.Done():
		t.Fatal("GetParams did not resume after the complete text action")
	}
}
