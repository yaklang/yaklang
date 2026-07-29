package main

import "testing"

func TestCompareReportsRespectsMetricDirection(t *testing.T) {
	baseline := newReport("smoke")
	baseline.Metrics = []metric{
		{Name: "latency", Value: 100, Unit: "ms", Direction: directionLower},
		{Name: "throughput", Value: 100, Unit: "flow/s", Direction: directionHigher},
		{Name: "informational", Value: 10, Unit: "count", Direction: directionNeutral},
	}
	candidate := newReport("smoke")
	candidate.Metrics = []metric{
		{Name: "latency", Value: 120, Unit: "ms", Direction: directionLower},
		{Name: "throughput", Value: 80, Unit: "flow/s", Direction: directionHigher},
		{Name: "informational", Value: 100, Unit: "count", Direction: directionNeutral},
	}

	result := compareReports("before", baseline, "after", candidate, 15)
	if len(result.Regressions) != 2 {
		t.Fatalf("expected two regressions, got %v", result.Regressions)
	}
}

func TestCompareReportsCollectsFailedChecks(t *testing.T) {
	baseline := newReport("smoke")
	candidate := newReport("smoke")
	candidate.Checks = []check{
		{Name: "pass", Status: "pass"},
		{Name: "skip", Status: "skipped"},
		{Name: "fail", Status: "fail"},
	}

	result := compareReports("before", baseline, "after", candidate, 15)
	if len(result.FailedChecks) != 1 || result.FailedChecks[0] != "fail" {
		t.Fatalf("unexpected failed checks: %v", result.FailedChecks)
	}
}

func TestCompareReportsDetectsConfigurationAndCoverageChanges(t *testing.T) {
	baseline := newReport("smoke")
	baseline.Config["concurrency"] = 4
	baseline.Metrics = []metric{{Name: "latency", Value: 10, Unit: "ms", Direction: directionLower}}
	baseline.Checks = []check{{Name: "race", Status: "fail"}, {Name: "persisted", Status: "pass"}}

	candidate := newReport("standard")
	candidate.Config["concurrency"] = 8
	candidate.Checks = []check{{Name: "race", Status: "skipped"}}

	result := compareReports("before", baseline, "after", candidate, 15)
	if len(result.ConfigIssues) != 2 {
		t.Fatalf("expected profile and concurrency issues, got %v", result.ConfigIssues)
	}
	if len(result.CoverageIssues) != 3 {
		t.Fatalf("expected missing metric, missing check, and skipped check issues, got %v", result.CoverageIssues)
	}
}

func TestCompareReportsIncludesZeroBaselineAsInformational(t *testing.T) {
	baseline := newReport("smoke")
	baseline.Metrics = []metric{{Name: "queue", Value: 0, Unit: "items", Direction: directionLower}}
	candidate := newReport("smoke")
	candidate.Metrics = []metric{{Name: "queue", Value: 1, Unit: "items", Direction: directionLower}}

	result := compareReports("before", baseline, "after", candidate, 15)
	if len(result.Rows) != 1 || result.Rows[0].ChangeComparable || result.Rows[0].Regression {
		t.Fatalf("unexpected zero-baseline comparison: %+v", result.Rows)
	}
}

func TestCompareReportsAllowsSmallMITMIntegrationNoise(t *testing.T) {
	baseline := newReport("smoke")
	baseline.Metrics = []metric{{
		Name: "mitm.baseline.query_p95_ms", Value: 2, Unit: "ms", Direction: directionLower,
	}}
	candidate := newReport("smoke")
	candidate.Metrics = []metric{{
		Name: "mitm.baseline.query_p95_ms", Value: 8, Unit: "ms", Direction: directionLower,
	}}

	result := compareReports("before", baseline, "after", candidate, 15)
	if len(result.Regressions) != 0 {
		t.Fatalf("small integration jitter should not fail the gate: %v", result.Regressions)
	}
	if len(result.Rows) != 1 || !result.Rows[0].WithinNoiseFloor || result.Rows[0].AbsoluteTolerance != 25 {
		t.Fatalf("unexpected noise-floor result: %+v", result.Rows)
	}
}

func TestCompareReportsRejectsMITMIntegrationRegressionBeyondNoise(t *testing.T) {
	baseline := newReport("smoke")
	baseline.Metrics = []metric{{
		Name: "mitm.baseline.query_p95_ms", Value: 2, Unit: "ms", Direction: directionLower,
	}}
	candidate := newReport("smoke")
	candidate.Metrics = []metric{{
		Name: "mitm.baseline.query_p95_ms", Value: 30, Unit: "ms", Direction: directionLower,
	}}

	result := compareReports("before", baseline, "after", candidate, 15)
	if len(result.Regressions) != 1 || !result.Rows[0].Regression || result.Rows[0].WithinNoiseFloor {
		t.Fatalf("material integration regression must fail the gate: %+v", result.Rows)
	}
}

func TestCompareReportsKeepsStandardIntegrationFloorTight(t *testing.T) {
	baseline := newReport("standard")
	baseline.Metrics = []metric{{
		Name: "mitm.baseline.query_p95_ms", Value: 2, Unit: "ms", Direction: directionLower,
	}}
	candidate := newReport("standard")
	candidate.Metrics = []metric{{
		Name: "mitm.baseline.query_p95_ms", Value: 20, Unit: "ms", Direction: directionLower,
	}}

	result := compareReports("before", baseline, "after", candidate, 15)
	if len(result.Regressions) != 1 || result.Rows[0].AbsoluteTolerance != 10 {
		t.Fatalf("standard profile should retain its tighter integration gate: %+v", result.Rows)
	}
}

func TestCompareReportsKeepsMicrobenchmarksPercentageStrict(t *testing.T) {
	baseline := newReport("smoke")
	baseline.Metrics = []metric{{
		Name: "go.trafficguard.clean32k.ns_op", Value: 2, Unit: "ns/op", Direction: directionLower,
	}}
	candidate := newReport("smoke")
	candidate.Metrics = []metric{{
		Name: "go.trafficguard.clean32k.ns_op", Value: 8, Unit: "ns/op", Direction: directionLower,
	}}

	result := compareReports("before", baseline, "after", candidate, 15)
	if len(result.Regressions) != 1 || result.Rows[0].AbsoluteTolerance != 0 || result.Rows[0].WithinNoiseFloor {
		t.Fatalf("microbenchmark regression must remain percentage-strict: %+v", result.Rows)
	}
}
