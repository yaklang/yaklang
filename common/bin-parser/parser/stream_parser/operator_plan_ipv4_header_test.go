package stream_parser

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func ipv4HeaderPlanSource(t *testing.T) string {
	t.Helper()
	for source := range embeddedOperatorSources() {
		if strings.HasPrefix(planTokens(source), planTokens(boundedDispatchPrefix)+" "+planTokens(boundedIPv4HeaderPrelude)+" ") {
			return source
		}
	}
	t.Fatal("missing exact embedded IPv4 header program")
	return ""
}

func ipv4HeaderPlanFixture(t *testing.T, protocol byte) *base.Node {
	t.Helper()
	root := giopBridgeInlineRoot(t, "endian: big\nPackage:\n  Internet Protocol:\n    Version: uint8\n    Header Length: uint8\n    Protocol: uint8\n    Total Length: uint16\n    Flags And Fragment Offset: raw,2\n    Padding: raw,13\n    Payload: raw,0\nTCP: raw\nUDP: raw\nNext Protocol Data: raw\n")
	wire := make([]byte, 28)
	copy(wire, []byte{4, 5, protocol, 0, 28, 0, 0})
	require.NoError(t, root.Parse(base.NewBitReader(bytes.NewReader(wire))))
	n := root.Children[0].Children[0].Children[6]
	// The synthetic header occupies 20 bytes; reserve the unconsumed eight
	// body bytes for the child callback rather than inheriting raw,0's limit.
	n.Cfg.SetItem(CfgLength, uint64(64))
	return n
}

func TestOperatorPlanIPv4HeaderAdmissionBeforeEffects(t *testing.T) {
	plan, err := loadOperatorPlan(ipv4HeaderPlanSource(t))
	require.NoError(t, err)
	require.NotNil(t, plan)
	for _, profile := range []string{"TCP", "UDP", "unknown", "options", "wrong-version", "short-total", "MF", "offset", "header-out", "version-out", "fragment-out", "custom-formatter"} {
		t.Run(profile, func(t *testing.T) {
			protocol := byte(17)
			if profile == "TCP" {
				protocol = 6
			} else if profile == "unknown" {
				protocol = 253
			}
			n := ipv4HeaderPlanFixture(t, protocol)
			parent := getNodeByPath(n, "@Internet Protocol")
			buffer := parent.Ctx.GetItem("buffer").(*bytes.Buffer).Bytes()
			switch profile {
			case "options":
				buffer[1] = 6
			case "wrong-version":
				buffer[0] = 6
			case "short-total":
				buffer[4] = 19
			case "MF":
				buffer[5] = 0x20
			case "offset":
				buffer[6] = 1
			case "header-out", "version-out", "fragment-out":
				index := map[string]int{"header-out": 1, "version-out": 0, "fragment-out": 4}[profile]
				parent.Children[index].Cfg.SetItem("out", "sideEffect()")
			case "custom-formatter":
				n.Ctx.SetItem("formatter", "custom")
			}
			var calls []string
			e := &planExecution{plan: plan, this: convertOperatorNode(n, func(node *base.Node) (func(bool), error) {
				calls = append(calls, node.Name)
				require.Equal(t, uint64(8), convertOperatorNode(node, nil).GetMaxLength())
				node.Cfg.SetItem(CfgNodeResult, [2]uint64{0, 0})
				return func(bool) {}, nil
			})}
			if profile == "TCP" || profile == "UDP" || profile == "unknown" {
				require.True(t, plan.run(e))
				want := map[string]string{"TCP": "TCP", "UDP": "UDP", "unknown": "Next Protocol Data"}[profile]
				require.Equal(t, []string{want}, calls)
			} else {
				require.False(t, plan.run(e), "VM must own every unqualified header before any effects")
				require.Empty(t, calls)
				require.Empty(t, n.Children)
			}
		})
	}
}

func TestOperatorPlanIPv4HeaderMatchesFreshVM(t *testing.T) {
	source := ipv4HeaderPlanSource(t)
	plan, err := loadOperatorPlan(source)
	require.NoError(t, err)
	for _, protocol := range []byte{6, 17, 253} {
		t.Run(fmt.Sprint(protocol), func(t *testing.T) {
			var effects [2][]string
			for mode := range effects {
				n := ipv4HeaderPlanFixture(t, protocol)
				process := func(child *base.Node) (func(bool), error) {
					effects[mode] = append(effects[mode], fmt.Sprintf("%s:%d", child.Name, convertOperatorNode(child, nil).GetMaxLength()))
					child.Cfg.SetItem(CfgNodeResult, [2]uint64{0, 0})
					return func(rollback bool) { effects[mode] = append(effects[mode], fmt.Sprint(rollback)) }, nil
				}
				if mode == 0 {
					require.NoError(t, execFreshOperator(n, source, process, []string{ParserMode}))
				} else {
					require.True(t, plan.run(&planExecution{plan: plan, this: convertOperatorNode(n, process)}))
				}
			}
			require.Equal(t, effects[0], effects[1])
		})
	}
	for _, pair := range [][2]string{{"header < 20", "header > 20"}, {"header > 20", "header >= 20"}, {"optionLength < 2", "optionLength < 1"}, {"position += optionLength", "position++"}, {"fragments[1] != 0", "fragments[1] == 0"}, {"data = []byte{}", "data = sideEffect()"}} {
		require.Contains(t, source, pair[0])
		require.Nil(t, compileBoundedDispatchPlan(planTokens(strings.Replace(source, pair[0], pair[1], 1))))
	}
}
