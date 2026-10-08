package stream_parser

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestOperatorPlanBoundedDispatchAdmission(t *testing.T) {
	var source string
	for candidate := range embeddedOperatorSources() {
		if strings.HasPrefix(planTokens(candidate), planTokens(boundedDispatchPrefix)) {
			source = candidate
			break
		}
	}
	require.NotEmpty(t, source)
	plan, err := loadOperatorPlan(source)
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Equal(t, "bounded-dispatch", plan.kind)
	for _, protocol := range []byte{2, 9, 33, 36, 53, 136} {
		root := giopBridgeInlineRoot(t, "Package:\n  Internet Protocol:\n    Protocol: uint8\n    Total Length: uint16\n    Payload: raw,0\n")
		require.NoError(t, root.Parse(base.NewBitReader(bytes.NewReader([]byte{protocol, 0, 3}))))
		n := root.Children[0].Children[0].Children[2]
		calls := 0
		e := &planExecution{plan: plan, this: convertOperatorNode(n, func(*base.Node) (func(bool), error) {
			calls++
			return func(bool) {}, nil
		})}
		require.False(t, plan.run(e), "guarded protocol %d", protocol)
		require.Zero(t, calls)
		require.Empty(t, n.Children)
	}
	for _, mutated := range []string{
		source + "\nthis.ProcessByType(\"Unexpected\")",
		strings.Replace(source, "protocol == 9", "protocol != 9", 1),
		strings.Replace(source, "dccpDestination = nil", "dccpDestination = sideEffect()", 1),
		strings.Replace(source, "var node", "sideEffect(); var node", 1),
		strings.Replace(source, "node.SetMaxLength(l)", "node.SetMaxLength(l+1)", 1),
		strings.Replace(source, `node = this.NewSubNode("Next Protocol Data")`, `node = this.NewSubNode("Next Protocol Data"); sideEffect()`, 1),
		strings.Replace(source, `node = this.NewSubNode("UDP")`, `node = this.NewSubNode("TCP")`, 1),
	} {
		require.Nil(t, compileBoundedDispatchPlan(planTokens(mutated)), "whole grammar or unique diagnostic location changed")
	}
	require.NotNil(t, compileOptionsLoopPlan(planTokens(optionsLoopSource)))
	require.Nil(t, compileOptionsLoopPlan(planTokens(strings.Replace(optionsLoopSource, "== 0", "== 1", 1))))
	require.Nil(t, compileOptionsLoopPlan(planTokens(optionsLoopSource+"\nsideEffect()")))
}
