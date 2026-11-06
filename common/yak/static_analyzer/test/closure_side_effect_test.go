package test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

// TestClosureInheritedSideEffectDoesNotLeakIntoCaller guards the split between a
// closure's own writes and the writes it inherited from calls inside its body.
//
// `toUtf8` assigns the outer `err`, so calling `toUtf8(...)` inside the
// goroutine records `err` as a side effect of that goroutine closure. The entry
// points at a SideEffect value, not at a value the closure computed.
// Propagating it to the `io.ReadEvery1s` call site overwrote the caller's own
// `err` assignment, and the caller's write then read as unhandled.
func TestClosureInheritedSideEffectDoesNotLeakIntoCaller(t *testing.T) {
	code := `
var err
toUtf8 = (s) => {
    res, err = codec.GBKSafe(s)
    if err {
        return codec.FixUTF8(s)
    }
    return res
}
go func() {
    streamLen := 0
    io.ReadEvery1s(context.Background(), os.Stdin, func(raw) {
        streamLen = 1
        yakit.Info("%v", toUtf8(string(raw)))
        return true
    })
}()
err = os.MkdirAll("/tmp/x", 0755)
if err != nil {
    println("bad")
}
`
	_, err := ssaapi.Parse(code, static_analyzer.GetPluginSSAOpt("yak")...)
	require.NoError(t, err)

	results := yak.StaticAnalyze(code, yak.WithStaticAnalyzePluginType("yak"))
	unhandled := make([]string, 0, len(results))
	for _, r := range results {
		if r.Severity != result.Error {
			continue
		}
		if r.Message != "Error Unhandled " {
			continue
		}
		unhandled = append(unhandled, fmt.Sprintf(
			"%s [%d:%d-%d:%d]", r.Message, r.StartLineNumber, r.StartColumn, r.EndLineNumber, r.EndColumn,
		))
	}
	for _, item := range unhandled {
		t.Logf("leaked unhandled error: %s", item)
	}
	require.Empty(t, unhandled,
		"a closure-inherited side effect must not overwrite the caller's own err assignment")
}
