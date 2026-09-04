package bin_parser

import (
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

func TestP1RoadmapCovered(t *testing.T) {
	var leftover []string
	for _, item := range ProtocolRoadmap {
		if item.Priority == priP1 && (item.Status == stTodo || item.Status == stPartial) {
			leftover = append(leftover, item.Name+"="+item.Status)
		}
	}
	require.Empty(t, leftover, "P1 protocols still todo/partial: %v", leftover)
}
