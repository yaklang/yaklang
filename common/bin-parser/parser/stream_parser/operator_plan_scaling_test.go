package stream_parser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func syntheticPortPlanSource(count int) string {
	var source strings.Builder
	source.WriteString(strings.ReplaceAll(portDispatchPrefix, "$TRANSPORT", "UDP"))
	source.WriteString("\ntypeNameList = [\"Default\"]\n")
	for i := 0; i < count; i++ {
		if i > 0 {
			source.WriteString(" else ")
		}
		fmt.Fprintf(&source, "if src == %d || dst == %d { typeNameList = [\"Protocol%d\"] }", i+1000, i+1000, i)
	}
	source.WriteString("\n" + portDispatchSuffix)
	return source.String()
}

func TestOperatorPlanLargePortTable(t *testing.T) {
	for _, count := range []int{300, 600} {
		p, err := loadOperatorPlan(syntheticPortPlanSource(count))
		require.NoError(t, err)
		require.NotNil(t, p)
		require.Len(t, p.ports.source, count)
		require.Len(t, p.ports.destination, count)
		for i := 0; i < count; i++ {
			port := uint16(1000 + i)
			require.Equal(t, []string{fmt.Sprintf("Protocol%d", i)}, p.ports.choose(0, port).candidates)
			// Earlier source-order branch must win over a later source-port hit.
			require.Equal(t, []string{"Protocol0"}, p.ports.choose(port, 1000).candidates)
		}
		require.Equal(t, []string{"Default"}, p.ports.choose(1, 65535).candidates)
	}
}

var portSelectionSink *portPlanBranch

// This measures ONLY table selection. It is not a packet parser throughput
// claim and cannot predict the cost/ambiguity of 600 actual protocol grammars.
func BenchmarkOperatorPlanPortSelection(b *testing.B) {
	for _, count := range []int{300, 600} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			p, err := loadOperatorPlan(syntheticPortPlanSource(count))
			if err != nil || p == nil {
				b.Fatalf("compile table: %v", err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			var selected *portPlanBranch
			for i := 0; i < b.N; i++ {
				selected = p.ports.choose(uint16(1000+i%count), 49152)
			}
			portSelectionSink = selected
		})
	}
}
