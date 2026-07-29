package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseGoBenchmarkOutputUsesMedian(t *testing.T) {
	output := `BenchmarkScanClean32K-4  10  100 ns/op  20 MB/s  8 B/op  1 allocs/op
BenchmarkScanClean32K-4  10  300 ns/op  10 MB/s  12 B/op  3 allocs/op
BenchmarkScanClean32K-4  10  200 ns/op  15 MB/s  10 B/op  2 allocs/op
`
	metrics := parseGoBenchmarkOutput(output, "trafficguard", "trafficguard")
	byName := make(map[string]metric)
	for _, item := range metrics {
		byName[item.Name] = item
	}
	if got := byName["go.trafficguard.BenchmarkScanClean32K.ns_op"].Value; got != 200 {
		t.Fatalf("unexpected ns/op median: %v", got)
	}
	if got := byName["go.trafficguard.BenchmarkScanClean32K.MB_s"].Value; got != 15 {
		t.Fatalf("unexpected MB/s median: %v", got)
	}
}

func TestParseGoBenchmarkOutputHandlesLogsBetweenNameAndMeasurement(t *testing.T) {
	output := `BenchmarkScanClean32K-4  [INFO] scanner initialized
[INFO] another setup message
      56    2263449 ns/op    14.48 MB/s    1708 B/op    4 allocs/op
BenchmarkScanHit32K-4  51  2251779 ns/op  14.59 MB/s  11589 B/op  26 allocs/op
`
	metrics := parseGoBenchmarkOutput(output, "trafficguard", "trafficguard")
	byName := make(map[string]metric)
	for _, item := range metrics {
		byName[item.Name] = item
	}
	if got := byName["go.trafficguard.BenchmarkScanClean32K.ns_op"].Value; got != 2263449 {
		t.Fatalf("unexpected split-line ns/op: %v", got)
	}
	if got := byName["go.trafficguard.BenchmarkScanClean32K.MB_s"].Value; got != 14.48 {
		t.Fatalf("unexpected split-line MB/s: %v", got)
	}
	if got := byName["go.trafficguard.BenchmarkScanHit32K.allocs_op"].Value; got != 26 {
		t.Fatalf("unexpected same-line allocs/op: %v", got)
	}
}

func TestParseVitestBenchmarkReport(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "vitest.json")
	raw := `{"testResults":{"manual list":[{"name":"add burst","hz":50,"mean":20,"p99":25}]}}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	metrics, err := parseVitestBenchmarkReport(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 3 {
		t.Fatalf("expected three metrics, got %d", len(metrics))
	}
}
