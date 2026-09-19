package stream_parser

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestNativeBridgeExactRegistrationAndDiagnostics(t *testing.T) {
	for _, source := range []string{bridgeTestSource("stats-request") + "return", " " + bridgeTestSource("stats-request"), bridgeTestSource("unknown")} {
		handled, err := execRegisteredNativeBridge(nil, source, nil, []string{ParserMode})
		require.False(t, handled)
		require.NoError(t, err)
	}
	for _, failure := range []string{"error", "panic"} {
		var expected string
		for _, legacy := range []bool{true, false} {
			root := giopBridgeInlineRoot(t, "unit: byte\nPackage:\n  Message: {}\n")
			require.NoError(t, (&DefParser{}).OnRoot(root))
			n := root.Children[0].Children[0]
			n.Cfg.SetItem(CfgLength, uint64(7*8))
			n.Ctx.SetItem("nativeBridgeLegacy", legacy)
			calls := 0
			err := ExecOperator(n, bridgeTestSource("stats-request"), func(*base.Node) (func(bool), error) {
				calls++
				if failure == "panic" {
					panic("injected panic")
				}
				return nil, fmt.Errorf("injected callback error")
			}, ParserMode)
			require.Equal(t, 1, calls, "native diagnostic replay must not call process twice")
			require.Error(t, err)
			if legacy {
				expected = err.Error()
			} else {
				require.Equal(t, expected, err.Error())
			}
		}
	}
}
