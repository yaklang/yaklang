package sca

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"testing/fstest"

	"github.com/yaklang/yaklang/common/sca/internal/fsio"
)

// Exercise both public output modes on every frozen representative. The
// report-only path must preserve every field and relationship, not just counts.
func TestReportOnlyMatchesCompatibilityPipeline(t *testing.T) {
	for _, tc := range formatScanCases() {
		t.Run(tc.name, func(t *testing.T) {
			input := loadFormatScanFS(t, tc.files)
			c := NewConfig()
			c.fs = fsio.New(input)
			c.snapshot = "same-snapshot"
			pkgs, want, err := scanPipeline(context.Background(), input, c)
			if err != nil || len(pkgs) == 0 {
				t.Fatalf("compatibility: %v", err)
			}
			got, err := ScanReport(context.Background(), input, WithSnapshotID(c.snapshot))
			if err != nil {
				t.Fatal(err)
			}
			a, _ := json.Marshal(want)
			b, _ := json.Marshal(got)
			if !bytes.Equal(a, b) {
				t.Fatalf("report differs from compatibility output:\n%s\n%s", a, b)
			}
		})
	}
}

func TestReportOnlyTruncationMatchesCompatibility(t *testing.T) {
	input := fstest.MapFS{"package-lock.json": {Data: []byte(`{"lockfileVersion":3,"packages":{"node_modules/a":{"version":"1","dependencies":{"b":"*"}},"node_modules/b":{"version":"2"},"node_modules/c":{"version":"3"}}}`)}}
	for _, limits := range []ResourceLimits{{MaxComponents: 1}, {MaxObservations: 1}, {MaxEdges: 1}, {MaxResultBytes: 256}, {MaxResultBytes: 20000}, {}} {
		c := NewConfig()
		c.fs = fsio.New(input)
		c.snapshot = "fixed"
		c.limits = limits
		_, want, we := scanPipeline(context.Background(), input, c)
		got, ge := ScanReport(context.Background(), input, WithSnapshotID("fixed"), WithResourceLimits(limits))
		a, _ := json.Marshal(want)
		b, _ := json.Marshal(got)
		if (we == nil) != (ge == nil) || !bytes.Equal(a, b) {
			t.Fatalf("limits=%+v\n%s\n%s\nerrors %v / %v", limits, a, b, we, ge)
		}
	}
}
