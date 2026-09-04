package bin_parser

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestP1ScorecardsCovered(t *testing.T) {
	schemaTraffic := map[int]bool{0: true, 8: true, 15: true, 20: true, 25: true}
	testsDim := map[int]bool{0: true, 6: true, 12: true, 16: true, 20: true}
	branchDim := map[int]bool{0: true, 8: true, 14: true, 20: true}
	stackDim := map[int]bool{0: true, 4: true, 7: true, 10: true}
	childNames := p1MustChildNames()

	var leftover []string
	for _, item := range ProtocolRoadmap {
		if item.Priority != priP1 {
			continue
		}
		sc, ok := ResolveP1Scorecard(item.Name)
		if !ok {
			leftover = append(leftover, item.Name+"=missing-scorecard")
			continue
		}
		require.True(t, schemaTraffic[sc.Schema], "%s schema %d", item.Name, sc.Schema)
		require.True(t, schemaTraffic[sc.Traffic], "%s traffic %d", item.Name, sc.Traffic)
		require.True(t, testsDim[sc.Tests], "%s tests %d", item.Name, sc.Tests)
		require.True(t, branchDim[sc.Branches], "%s branches %d", item.Name, sc.Branches)
		require.True(t, stackDim[sc.Stack], "%s stack %d", item.Name, sc.Stack)
		require.Contains(t, []string{"L1", "L2", "L3", "L4"}, sc.SampleClass, "%s sample class", item.Name)
		require.NotEmpty(t, sc.Evidence, "%s missing evidence", item.Name)
		if sc.SampleClass == "L4" {
			require.False(t, sc.G5, "P1 %q SampleClass L4 must fail G5", item.Name)
			require.LessOrEqual(t, sc.Traffic, 8, "P1 %q L4 traffic must be <= 8", item.Name)
		}
		if sc.SampleClass == "L3" {
			require.LessOrEqual(t, sc.Traffic, 8, "P1 %q L3 gopacket serialize traffic must be <= 8", item.Name)
		}
		require.Contains(t, []string{"A", "B", "C", "D", "F"}, sc.Grade())

		schCap := schemaCeiling(sc.Rule, sc.OpaqueRaw)
		eth := hasEthernetMustChild(sc, childNames)
		ported := strings.Contains(sc.Evidence, "TCP/") || strings.Contains(sc.Evidence, "UDP/")
		failN := failCount(sc.Rule)
		tCap := testsCeiling(sc.Rule, eth, ported)
		trCap := trafficCeiling(sc.SampleClass, eth)
		p0Rule := false
		for _, p0 := range P0Scorecards {
			if p0.Rule == sc.Rule {
				p0Rule = true
				break
			}
		}
		if p0Rule {
			schCap, tCap, trCap = 25, 20, 25
		}
		if sc.G6 && failN < 1 {
			t.Errorf("P1 %q claimed G6 but no parseMustFail for %s", item.Name, sc.Rule)
		}
		if sc.G7 && ported && !eth {
			t.Errorf("P1 %q claimed G7 without ethernet mustChild for %s", item.Name, sc.Rule)
		}
		sc = deriveP1Gates(sc, failN, eth, ported)
		if sc.Schema > schCap {
			t.Errorf("P1 %q schema %d exceeds ceiling %d for %s leftover=%q", item.Name, sc.Schema, schCap, sc.Rule, sc.OpaqueRaw)
		}
		if sc.Tests > tCap {
			t.Errorf("P1 %q tests %d exceeds ceiling %d (failCount=%d eth=%v rows=%d)", item.Name, sc.Tests, tCap, failN, eth, successBranchRows(sc.Rule))
		}
		if sc.Traffic > trCap {
			t.Errorf("P1 %q traffic %d exceeds ceiling %d (sample=%s eth=%v)", item.Name, sc.Traffic, trCap, sc.SampleClass, eth)
		}
		if !sc.G6 {
			t.Errorf("P1 %q G6 fail: no parseMustFail for %s", item.Name, sc.Rule)
		}
		if sc.Traffic == 25 && !eth {
			t.Errorf("P1 %q G7/Traffic 25 but no parseEthernet mustChild for %s", item.Name, sc.Rule)
		}

		if item.Status == stDone {
			if sc.SampleClass == "L4" {
				t.Errorf("P1 %q is done but SampleClass L4 (G5 requires L1/L2/L3)", item.Name)
			}
			if sc.Grade() != "A" {
				t.Errorf("P1 %q is done but grade %s total %d (need A) gates G1=%v G6=%v G7=%v", item.Name, sc.Grade(), sc.Total(), sc.G1, sc.G6, sc.G7)
			}
			if !sc.GatesOK() {
				t.Errorf("P1 %q is done but a hard gate failed G1=%v G2=%v G3=%v G4=%v G5=%v G6=%v G7=%v G8=%v", item.Name, sc.G1, sc.G2, sc.G3, sc.G4, sc.G5, sc.G6, sc.G7, sc.G8)
			}
			if sc.Total() < 90 {
				t.Errorf("P1 %q total %d < 90", item.Name, sc.Total())
			}
		}
	}
	require.Empty(t, leftover, "P1 protocols missing scorecards: %v", leftover)

	seen := map[string]int{}
	for _, s := range P1Scorecards {
		seen[s.Name]++
	}
	for name, n := range seen {
		require.Equal(t, 1, n, "duplicate scorecard name %s", name)
	}
}

func TestP1RequiredAliases(t *testing.T) {
	childNames := p1MustChildNames()
	for _, name := range []string{
		"IEEE 802.1X", "RPC", "Portmap/Rpcbind", "RabbitMQ", "IIOP Locate", "Memcache binary",
	} {
		sc, ok := ResolveP1Scorecard(name)
		require.True(t, ok, "missing scorecard for %s", name)
		require.NotEmpty(t, sc.AliasOf, "%s should share a YAML via AliasOf", name)
		eth := hasEthernetMustChild(sc, childNames)
		ported := strings.Contains(sc.Evidence, "TCP/") || strings.Contains(sc.Evidence, "UDP/")
		sc = deriveP1Gates(sc, failCount(sc.Rule), eth, ported)
		require.Equal(t, "A", sc.Grade(), "%s alias grade %s total %d aliasOf=%s", name, sc.Grade(), sc.Total(), sc.AliasOf)
	}
}

func TestP1CardsDoNotHardcodeGates(t *testing.T) {
	for _, s := range P1Scorecards {
		if s.AliasOf != "" {
			continue
		}
		require.False(t, s.G1, "%s G1 must be derived, not hardcoded", s.Name)
		require.False(t, s.G6, "%s G6 must be derived, not hardcoded", s.Name)
		require.False(t, s.G7, "%s G7 must be derived, not hardcoded", s.Name)
	}
}

func TestP1DerivedGatesRejectFake(t *testing.T) {
	fake := p1card("NoSuchProto", "no-such.yaml", 25, 25, 20, 20, 10, "L2", "handmade", "")
	require.False(t, fake.G1)
	require.False(t, fake.G6)
	require.False(t, fake.GatesOK())
	require.Equal(t, "F", fake.Grade())
	derived := deriveP1Gates(fake, 0, false, true)
	require.False(t, derived.G1)
	require.False(t, derived.G6)
	require.False(t, derived.G7)
	require.Equal(t, "F", derived.Grade())
}

func TestP1DumpInventory(t *testing.T) {
	path := os.Getenv("P1_DUMP")
	if path == "" {
		t.Skip("set P1_DUMP to write inventory markdown")
	}
	require.NoError(t, os.WriteFile(path, []byte(p1InventoryMarkdown()), 0o644))
}

func TestP1ScoresDocListsEveryName(t *testing.T) {
	b, err := os.ReadFile("P1_SCORES.md")
	if err != nil {
		b, err = os.ReadFile("common/bin-parser/P1_SCORES.md")
	}
	require.NoError(t, err)
	text := string(b)
	for _, item := range ProtocolRoadmap {
		if item.Priority != priP1 {
			continue
		}
		require.Contains(t, text, item.Name, "P1_SCORES.md missing %s", item.Name)
	}
}

func p1InventoryMarkdown() string {
	childNames := p1MustChildNames()
	var b strings.Builder
	b.WriteString("# P1 协议交付打分\n\n")
	b.WriteString("对照 [PROTOCOL_DELIVERY.md](PROTOCOL_DELIVERY.md)。机器可读记录在 `p1_scores.go`，由 `TestP1ScorecardsCovered` 校验：每个 P1 名称都有计分卡；`Status: done` 必须是 **A**（总分 ≥ 90）。\n\n")
	b.WriteString("维度（满分 100）：Schema 25 / 真实流量 25 / 测试 20 / 分支覆盖 20 / 栈集成 10。硬门槛 G1–G8 全过才计分。\n\n")
	b.WriteString("别名与主规则共用同一张卡（见 `AliasOf`）。样本来源包括 gopacket 测试帧、RFC 完整 PDU，以及 Ethernet+IP+L4 整帧断言。\n\n")
	b.WriteString("G5 要求 SampleClass ∈ {L1, L2, L3}；L4-only handmade PDU 不计分。L3 gopacket serialize 的 Traffic ≤ 8。\n\n")
	b.WriteString("`TestP1ScorecardsCovered` 用 YAML/`p1FailCases`/mustChild 扫描卡死 Schema/Tests/Traffic 上限：声称分不得高于 `schemaCeiling` / `testsCeiling` / `trafficCeiling`。G1–G4/G6–G8 由测试从规则文件、失败路径和以太网封装推导，`p1card` 不得写死为 true。\n\n")
	b.WriteString("| 协议 | 等级 | 总分 | Schema | 流量 | 测试 | 分支 | 栈 | 样本 | 规则 |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, item := range ProtocolRoadmap {
		if item.Priority != priP1 {
			continue
		}
		sc, ok := ResolveP1Scorecard(item.Name)
		if !ok {
			fmt.Fprintf(&b, "| %s | missing | | | | | | | | |\n", item.Name)
			continue
		}
		eth := hasEthernetMustChild(sc, childNames)
		ported := strings.Contains(sc.Evidence, "TCP/") || strings.Contains(sc.Evidence, "UDP/")
		sc = deriveP1Gates(sc, failCount(sc.Rule), eth, ported)
		rule := "`" + sc.Rule + "`"
		if sc.AliasOf != "" {
			rule = "alias of " + sc.AliasOf
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %d | %d | %d | %s | %s |\n",
			item.Name, sc.Grade(), sc.Total(), sc.Schema, sc.Traffic, sc.Tests, sc.Branches, sc.Stack, sc.SampleClass, rule)
	}
	return b.String()
}

func TestP1RoadmapCovered(t *testing.T) {
	var leftover []string
	for _, item := range ProtocolRoadmap {
		if item.Priority == priP1 && (item.Status == stTodo || item.Status == stPartial) {
			leftover = append(leftover, item.Name+"="+item.Status)
		}
	}
	require.Empty(t, leftover, "P1 protocols still todo/partial: %v", leftover)
}
