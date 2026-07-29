package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type comparisonRow struct {
	Name              string          `json:"name"`
	Area              string          `json:"area"`
	Unit              string          `json:"unit"`
	Direction         metricDirection `json:"direction"`
	Baseline          float64         `json:"baseline"`
	Candidate         float64         `json:"candidate"`
	ChangePercent     float64         `json:"change_percent"`
	RegressionPercent float64         `json:"regression_percent"`
	Regression        bool            `json:"regression"`
	ChangeComparable  bool            `json:"change_comparable"`
	AbsoluteTolerance float64         `json:"absolute_tolerance,omitempty"`
	WithinNoiseFloor  bool            `json:"within_noise_floor,omitempty"`
}

type comparisonReport struct {
	Baseline        string          `json:"baseline"`
	Candidate       string          `json:"candidate"`
	MaxRegression   float64         `json:"max_regression_percent"`
	Rows            []comparisonRow `json:"rows"`
	CandidateChecks []check         `json:"candidate_checks"`
	Regressions     []string        `json:"regressions"`
	FailedChecks    []string        `json:"failed_checks"`
	CoverageIssues  []string        `json:"coverage_issues"`
	ConfigIssues    []string        `json:"configuration_issues"`
}

func runCompare(args []string) error {
	fs := newFlagSet("compare")
	baselinePath := fs.String("baseline", "", "baseline JSON report")
	candidatePath := fs.String("candidate", "", "candidate JSON report")
	outputPath := fs.String("out", "", "optional JSON comparison output")
	maxRegression := fs.Float64("max-regression", 15, "maximum allowed regression percentage")
	failOnRegression := fs.Bool("fail-on-regression", true, "return non-zero when a comparable metric regresses")
	requireChecks := fs.Bool("require-checks", true, "return non-zero when a candidate correctness check fails")
	requireComparable := fs.Bool("require-comparable", true, "return non-zero when configuration or report coverage differs")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *baselinePath == "" || *candidatePath == "" {
		return fmt.Errorf("-baseline and -candidate are required")
	}

	baseline, err := readReport(*baselinePath)
	if err != nil {
		return err
	}
	candidate, err := readReport(*candidatePath)
	if err != nil {
		return err
	}

	comparison := compareReports(*baselinePath, baseline, *candidatePath, candidate, *maxRegression)
	printComparison(comparison)
	if *outputPath != "" {
		raw, err := json.MarshalIndent(comparison, "", "  ")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(*outputPath, append(raw, '\n'), 0o644); err != nil {
			return err
		}
	}

	var reasons []string
	if *failOnRegression && len(comparison.Regressions) > 0 {
		reasons = append(reasons, fmt.Sprintf("%d metric regression(s)", len(comparison.Regressions)))
	}
	if *requireChecks && len(comparison.FailedChecks) > 0 {
		reasons = append(reasons, fmt.Sprintf("%d failed candidate check(s)", len(comparison.FailedChecks)))
	}
	if *requireComparable && (len(comparison.ConfigIssues) > 0 || len(comparison.CoverageIssues) > 0) {
		reasons = append(reasons, fmt.Sprintf("%d comparability issue(s)", len(comparison.ConfigIssues)+len(comparison.CoverageIssues)))
	}
	if len(reasons) > 0 {
		return fmt.Errorf("comparison gate failed: %s", strings.Join(reasons, ", "))
	}
	return nil
}

func compareReports(baselinePath string, baseline *report, candidatePath string, candidate *report, maxRegression float64) *comparisonReport {
	result := &comparisonReport{
		Baseline:        baselinePath,
		Candidate:       candidatePath,
		MaxRegression:   maxRegression,
		CandidateChecks: append([]check(nil), candidate.Checks...),
	}
	baselineByName := make(map[string]metric, len(baseline.Metrics))
	for _, item := range baseline.Metrics {
		baselineByName[item.Name] = item
	}
	candidateByName := make(map[string]metric, len(candidate.Metrics))
	for _, current := range candidate.Metrics {
		candidateByName[current.Name] = current
		previous, ok := baselineByName[current.Name]
		if !ok || previous.Unit != current.Unit {
			continue
		}
		direction := current.Direction
		if direction == "" {
			direction = previous.Direction
		}
		change := 0.0
		changeComparable := previous.Value != 0
		if changeComparable {
			change = (current.Value - previous.Value) / previous.Value * 100
		}
		regressionPercent := 0.0
		if changeComparable {
			switch direction {
			case directionLower:
				regressionPercent = change
			case directionHigher:
				regressionPercent = -change
			}
		}
		absoluteTolerance := integrationMetricAbsoluteTolerance(current.Name, candidate.Profile)
		adverseDelta := 0.0
		switch direction {
		case directionLower:
			adverseDelta = current.Value - previous.Value
		case directionHigher:
			adverseDelta = previous.Value - current.Value
		}
		percentageRegression := changeComparable && direction != directionNeutral && regressionPercent > maxRegression
		withinNoiseFloor := percentageRegression && absoluteTolerance > 0 && adverseDelta <= absoluteTolerance
		isRegression := percentageRegression && !withinNoiseFloor
		row := comparisonRow{
			Name:              current.Name,
			Area:              current.Area,
			Unit:              current.Unit,
			Direction:         direction,
			Baseline:          previous.Value,
			Candidate:         current.Value,
			ChangePercent:     change,
			RegressionPercent: regressionPercent,
			Regression:        isRegression,
			ChangeComparable:  changeComparable,
			AbsoluteTolerance: absoluteTolerance,
			WithinNoiseFloor:  withinNoiseFloor,
		}
		result.Rows = append(result.Rows, row)
		if isRegression {
			result.Regressions = append(result.Regressions, current.Name)
		}
	}
	for _, previous := range baseline.Metrics {
		current, ok := candidateByName[previous.Name]
		if !ok {
			result.CoverageIssues = append(result.CoverageIssues, "missing metric: "+previous.Name)
			continue
		}
		if previous.Unit != current.Unit {
			result.CoverageIssues = append(result.CoverageIssues,
				fmt.Sprintf("unit changed for %s: %s -> %s", previous.Name, previous.Unit, current.Unit))
		}
		if previous.Direction != current.Direction {
			result.CoverageIssues = append(result.CoverageIssues,
				fmt.Sprintf("direction changed for %s: %s -> %s", previous.Name, previous.Direction, current.Direction))
		}
	}
	baselineChecks := make(map[string]check, len(baseline.Checks))
	for _, item := range baseline.Checks {
		baselineChecks[item.Name] = item
	}
	candidateChecks := make(map[string]check, len(candidate.Checks))
	for _, item := range candidate.Checks {
		candidateChecks[item.Name] = item
		if item.Status == "fail" {
			result.FailedChecks = append(result.FailedChecks, item.Name)
		}
	}
	for name, previous := range baselineChecks {
		current, ok := candidateChecks[name]
		if !ok {
			result.CoverageIssues = append(result.CoverageIssues, "missing check: "+name)
			continue
		}
		if previous.Status != "skipped" && current.Status == "skipped" {
			result.CoverageIssues = append(result.CoverageIssues, "candidate skipped previously exercised check: "+name)
		}
	}
	result.ConfigIssues = reportConfigurationIssues(baseline, candidate)
	sort.Slice(result.Rows, func(i, j int) bool { return result.Rows[i].Name < result.Rows[j].Name })
	sort.Strings(result.Regressions)
	sort.Strings(result.FailedChecks)
	sort.Strings(result.CoverageIssues)
	sort.Strings(result.ConfigIssues)
	return result
}

// integrationMetricAbsoluteTolerance defines absolute noise floors for
// one-shot MITM process measurements. Percentage-only gates are misleading for
// values near zero (for example, a 2 ms shutdown becoming 8 ms). Regressions
// beyond these small absolute bounds still fail. Repeated microbenchmarks and
// database/frontend benchmarks intentionally receive no noise floor.
func integrationMetricAbsoluteTolerance(name, profile string) float64 {
	if !strings.HasPrefix(name, "mitm.") {
		return 0
	}
	switch {
	case strings.HasSuffix(name, ".query_p95_ms"):
		if profile == "smoke" {
			return 25
		}
		return 10
	case strings.HasSuffix(name, ".backend_query_p95_ms"),
		strings.HasSuffix(name, ".backend_count_p95_ms"),
		strings.HasSuffix(name, ".backend_data_query_p95_ms"),
		strings.HasSuffix(name, ".backend_conversion_p95_ms"):
		if profile == "smoke" {
			return 5
		}
		return 2
	case strings.HasSuffix(name, ".persist_queue_wait_p95_ms"),
		strings.HasSuffix(name, ".persist_write_p95_ms"),
		strings.HasSuffix(name, ".request_to_flow_built_p95_ms"),
		strings.HasSuffix(name, ".response_to_flow_built_p95_ms"),
		strings.HasSuffix(name, ".duplex_delivery_p95_ms"):
		if profile == "smoke" {
			return 5
		}
		return 2
	case strings.HasSuffix(name, ".request_to_probe_receive_p95_ms"),
		strings.HasSuffix(name, ".response_to_probe_receive_p95_ms"),
		strings.HasSuffix(name, ".persist_to_probe_receive_p95_ms"):
		if profile == "smoke" {
			return 25
		}
		return 10
	case strings.HasSuffix(name, ".database_change_detection_p95_ms"):
		if profile == "smoke" {
			return 100
		}
		return 50
	case strings.HasSuffix(name, ".request_p95_ms"):
		if profile == "smoke" {
			return 10
		}
		return 0
	case strings.HasSuffix(name, ".shutdown_ms"):
		return 15
	case strings.HasSuffix(name, ".post_shutdown_cpu_cores"):
		return 0.01
	case strings.HasSuffix(name, ".queue_peak"):
		return 10
	case strings.HasSuffix(name, ".goroutine_peak"),
		strings.HasSuffix(name, ".goroutine_peak_delta"),
		strings.HasSuffix(name, ".goroutine_after_shutdown_delta"):
		if profile == "smoke" {
			return 10
		}
		return 5
	default:
		return 0
	}
}

func reportConfigurationIssues(baseline, candidate *report) []string {
	var issues []string
	if baseline.Profile != candidate.Profile {
		issues = append(issues, fmt.Sprintf("profile changed: %s -> %s", baseline.Profile, candidate.Profile))
	}
	compareValues := func(section string, before, after map[string]any, keys ...string) {
		for _, key := range keys {
			beforeValue, beforeOK := before[key]
			afterValue, afterOK := after[key]
			if beforeOK != afterOK || beforeOK && fmt.Sprint(beforeValue) != fmt.Sprint(afterValue) {
				issues = append(issues, fmt.Sprintf("%s.%s changed: %v -> %v", section, key, beforeValue, afterValue))
			}
		}
	}
	compareValues("config", baseline.Config, candidate.Config,
		"requests", "concurrency", "body_bytes", "seed_rows", "repetitions", "query_hz",
		"bench_time", "bench_count", "scenario_timeout_seconds", "race_enabled",
		"frontend_benchmark_enabled", "go_microbenchmarks_enabled")
	compareValues("system", baseline.System, candidate.System,
		"goos", "goarch", "logical_cpus", "gomaxprocs", "go_version", "memory_limit_gib", "cpu_measurement")
	return issues
}

func printComparison(result *comparisonReport) {
	fmt.Println("| Metric | Before | After | Change | Direction | Gate |")
	fmt.Println("|---|---:|---:|---:|---|---|")
	for _, row := range result.Rows {
		gate := "ok"
		if row.Direction == directionNeutral {
			gate = "info"
		} else if row.Regression {
			gate = "REGRESSION"
		} else if row.WithinNoiseFloor {
			gate = "noise"
		}
		change := "n/a (zero baseline)"
		if row.ChangeComparable {
			change = fmt.Sprintf("%+.2f%%", row.ChangePercent)
		}
		fmt.Printf("| `%s` | %.4g %s | %.4g %s | %s | %s | %s |\n",
			row.Name, row.Baseline, row.Unit, row.Candidate, row.Unit, change, row.Direction, gate)
	}
	if len(result.FailedChecks) > 0 {
		fmt.Printf("\nFailed candidate checks: %s\n", strings.Join(result.FailedChecks, ", "))
	}
	if len(result.ConfigIssues) > 0 {
		fmt.Printf("\nConfiguration issues: %s\n", strings.Join(result.ConfigIssues, "; "))
	}
	if len(result.CoverageIssues) > 0 {
		fmt.Printf("\nCoverage issues: %s\n", strings.Join(result.CoverageIssues, "; "))
	}
	if len(result.Rows) == 0 {
		fmt.Println("No metrics with matching names and units were found.")
	}
}
