package bin_parser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestP1ScorecardsCovered(t *testing.T) {
	schemaTraffic := map[int]bool{0: true, 8: true, 15: true, 20: true, 25: true}
	testsDim := map[int]bool{0: true, 6: true, 12: true, 16: true, 20: true}
	branchDim := map[int]bool{0: true, 8: true, 14: true, 20: true}
	stackDim := map[int]bool{0: true, 4: true, 7: true, 10: true}

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
		require.NotEmpty(t, sc.SampleClass, "%s missing sample class", item.Name)
		require.NotEmpty(t, sc.Evidence, "%s missing evidence", item.Name)
		require.Contains(t, []string{"A", "B", "C", "D", "F"}, sc.Grade())
		if item.Status == stDone {
			if sc.Grade() != "A" {
				t.Errorf("P1 %q is done but grade %s total %d (need A)", item.Name, sc.Grade(), sc.Total())
			}
			if !sc.GatesOK() {
				t.Errorf("P1 %q is done but a hard gate failed", item.Name)
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

func TestP1RoadmapCovered(t *testing.T) {
	var leftover []string
	for _, item := range ProtocolRoadmap {
		if item.Priority == priP1 && (item.Status == stTodo || item.Status == stPartial) {
			leftover = append(leftover, item.Name+"="+item.Status)
		}
	}
	require.Empty(t, leftover, "P1 protocols still todo/partial: %v", leftover)
}
