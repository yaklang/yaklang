package bin_parser

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestProtocolCorpusConsumedLengthPublicDifferential(t *testing.T) {
	for index, fixture := range alljoynTestFixtures(t) {
		for _, entry := range []string{"AllJoynNS", "AllJoynNSCarrier", "UDP"} {
			t.Run(fmt.Sprintf("fixture-%d/%s", index, entry), func(t *testing.T) {
				wire, rule := fixture, alljoynCorpusRule
				if entry == "UDP" {
					wire, rule = alljoynTestUDP(fixture, false), "user_datagram_protocol"
				}
				parse := func(legacy bool) *base.Node {
					node := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, rule, entry, map[string]any{
						"parseConsumedLengthLegacy": legacy,
						"alljoynNSLegacyLengths":    false,
					})
					require.Equal(t, wire, NodeToBytes(node))
					// Check the imported runtime context, not just the outer
					// input map: the oracle must actually reach the fields.
					field := protocolCorpusFindNode(node, "Sender Version")
					require.NotNil(t, field)
					require.True(t, field.Ctx.Has("parseConsumedLengthLegacy"))
					require.Equal(t, legacy, field.Ctx.GetBool("parseConsumedLengthLegacy"))
					return node
				}
				fast, legacy := parse(false), parse(true)
				alljoynTestCompareTrees(t, fast, legacy)
				require.Equal(t, NodeToMap(legacy), NodeToMap(fast))
			})
		}
	}
}
