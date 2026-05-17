//go:build bucketbench
// +build bucketbench

package aicommon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 关键词: timeline_bucket_bench, 字节子桶调优, 实验入口
//
// 跑法 (注意需要 -tags bucketbench 才会进入):
//
//	go test -tags bucketbench -v -run TestBucketBench \
//	  ./common/ai/aid/aicommon/ -timeout 5m
//
// 默认产出落在 ./testdata/bucket_bench/<unix-ts>-<name>.md。

const realSessionDir = "/Users/v1ll4n/yakit-projects/aispace/11194_redhaze_pentest_auth_20260517_1d364"

// fixedBudgetCandidates 是固定桶大小扫描的候选值 (byte)。
// 关键词: bucket_bench 固定值候选
var fixedBudgetCandidates = []struct {
	label  string
	budget int64
}{
	{"-1 (no-split)", -1},
	{"4K", 4 * 1024},
	{"6K", 6 * 1024},
	{"8K", 8 * 1024},
	{"12K", 12 * 1024},
	{"16K (current)", 16 * 1024},
	{"24K", 24 * 1024},
	{"32K", 32 * 1024},
	{"48K", 48 * 1024},
	{"64K", 64 * 1024},
	{"96K", 96 * 1024},
	{"128K", 128 * 1024},
	{"192K", 192 * 1024},
}

// allScenarios 实验用到的数据集集合。
// 关键词: bucket_bench 数据集
func allScenarios(t *testing.T) []BucketBenchScenario {
	baseTs := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	scenarios := []BucketBenchScenario{
		BuildSyntheticScenario("short_query", baseTs),
		BuildSyntheticScenario("dense_tools", baseTs),
		BuildSyntheticScenario("single_huge", baseTs),
		BuildSyntheticScenario("mixed", baseTs),
	}

	if _, err := os.Stat(realSessionDir); err == nil {
		events, err := LoadRealSessionEvents(realSessionDir)
		if err == nil && len(events) > 0 {
			scenarios = append(scenarios, BucketBenchScenario{
				Name:   "real_redhaze",
				Events: events,
			})
			t.Logf("loaded real session: %d events", len(events))
		} else {
			t.Logf("real session load failed/empty (err=%v)", err)
		}
	} else {
		t.Logf("real session dir missing: %s (skipping real_redhaze)", realSessionDir)
	}
	return scenarios
}

func writeBenchReport(t *testing.T, name, body string) {
	t.Helper()
	dir := filepath.Join("testdata", "bucket_bench")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ts := time.Now().Unix()
	out := filepath.Join(dir, fmt.Sprintf("%d-%s.md", ts, name))
	if err := os.WriteFile(out, []byte(body), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
	t.Logf("report written to %s", out)
}

// TestBucketBench_FixedSweep 固定桶大小扫描入口。
// 关键词: bucket_bench 固定值扫描
func TestBucketBench_FixedSweep(t *testing.T) {
	scenarios := allScenarios(t)

	var allResults []BucketBenchResult
	for _, sc := range scenarios {
		t.Logf("scanning scenario: %s (events=%d)", sc.Name, len(sc.Events))
		for _, cand := range fixedBudgetCandidates {
			res := ReplayAndMeasure(sc.Name, sc.Events, BucketBenchOptions{Budget: cand.budget})
			res.BudgetLabel = cand.label
			allResults = append(allResults, res)
		}
	}

	var buf strings.Builder
	buf.WriteString("# Bucket Bench: Fixed Budget Sweep\n\n")
	buf.WriteString(fmt.Sprintf("generated at %s\n\n", time.Now().Format(time.RFC3339)))
	buf.WriteString(fmt.Sprintf("scenarios: %d, candidates per scenario: %d\n\n",
		len(scenarios), len(fixedBudgetCandidates)))
	buf.WriteString(FormatBucketBenchTable(allResults))
	buf.WriteString("\n## Per-scenario minimum net cost\n\n")
	buf.WriteString("| scenario | best budget | net-cost | flush | avg-frozen |\n")
	buf.WriteString("| --- | --- | --- | --- | --- |\n")
	byScenario := map[string][]BucketBenchResult{}
	for _, r := range allResults {
		byScenario[r.Scenario] = append(byScenario[r.Scenario], r)
	}
	for name, rs := range byScenario {
		best := rs[0]
		for _, r := range rs {
			if r.EstNetCost < best.EstNetCost {
				best = r
			}
		}
		buf.WriteString(fmt.Sprintf("| %s | %s | %s | %d | %s |\n",
			name, best.BudgetLabel, signedHumanBytes(best.EstNetCost),
			best.FlushCount, humanBytes(best.AvgFrozenBytes)))
	}

	writeBenchReport(t, "fixed-sweep", buf.String())
	fmt.Println(buf.String())
}

// TestBucketBench_AlgorithmCompare 在同一数据集上跑 4 个动态算法 + 1 个 baseline。
// 关键词: bucket_bench 动态算法对比
func TestBucketBench_AlgorithmCompare(t *testing.T) {
	scenarios := allScenarios(t)

	algos := []struct {
		label string
		opts  BucketBenchOptions
	}{
		{"A_Fixed_16K(baseline)", BucketBenchOptions{Budget: 16 * 1024}},
		{"A_Fixed_64K", BucketBenchOptions{Budget: 64 * 1024}},
		{"B_TimeRemaining(64K->8K)", BucketBenchOptions{Sizer: TimeRemainingBucketSizer(64*1024, 8*1024)}},
		{"C_EntryAdaptive(8x,32K-256K)", BucketBenchOptions{Sizer: EntryAdaptiveBucketSizer(8, 32*1024, 256*1024)}},
		{"D_TokenAware(5000tok)", BucketBenchOptions{Sizer: TokenAwareBucketSizer(5000)}},
	}

	var allResults []BucketBenchResult
	for _, sc := range scenarios {
		for _, a := range algos {
			res := ReplayAndMeasure(sc.Name, sc.Events, a.opts)
			res.BudgetLabel = a.label
			allResults = append(allResults, res)
		}
	}

	var buf strings.Builder
	buf.WriteString("# Bucket Bench: Dynamic Algorithm Compare\n\n")
	buf.WriteString(fmt.Sprintf("generated at %s\n\n", time.Now().Format(time.RFC3339)))
	buf.WriteString(FormatBucketBenchTable(allResults))
	buf.WriteString("\n## Per-scenario winner (lowest net cost)\n\n")
	buf.WriteString("| scenario | best algo | net-cost | flush | avg-frozen |\n")
	buf.WriteString("| --- | --- | --- | --- | --- |\n")
	byScenario := map[string][]BucketBenchResult{}
	for _, r := range allResults {
		byScenario[r.Scenario] = append(byScenario[r.Scenario], r)
	}
	for name, rs := range byScenario {
		best := rs[0]
		for _, r := range rs {
			if r.EstNetCost < best.EstNetCost {
				best = r
			}
		}
		buf.WriteString(fmt.Sprintf("| %s | %s | %s | %d | %s |\n",
			name, best.BudgetLabel, signedHumanBytes(best.EstNetCost),
			best.FlushCount, humanBytes(best.AvgFrozenBytes)))
	}

	writeBenchReport(t, "algo-compare", buf.String())
	fmt.Println(buf.String())
}
