package yak

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestFuzzStringsUnquoteExportIsSafeInYakRawString(t *testing.T) {
	// Cover every possible byte so exported binary Fuzztags are constrained by
	// the actual user workflow: paste into fuzz.Strings(`...`), compile, render,
	// and recover the exact source bytes. In particular, 0x60 must not terminate
	// the outer Yak raw string.
	original := make([]byte, 256)
	for i := range original {
		original[i] = byte(i)
	}

	tag := lowhttp.ToUnquoteFuzzTagForce(original)
	require.NotContains(t, tag, "`", "exported Fuzztag must be safe inside a Yak raw string")

	engine := newTestHookEngine(t, "values = fuzz.Strings(`"+tag+"`)\ngetResult = func() { return values[0] }", nil)
	result, err := engine.CallYakFunction(context.Background(), "getResult", nil)
	require.NoError(t, err)

	switch value := result.(type) {
	case string:
		require.Equal(t, original, []byte(value))
	case []byte:
		require.Equal(t, original, value)
	default:
		t.Fatalf("unexpected fuzz.Strings result type %T", result)
	}
}
