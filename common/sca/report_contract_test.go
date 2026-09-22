package sca

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/model"
	"testing"
	"testing/fstest"
)

func TestMissingParentWithoutDependenciesIsIncomplete(t *testing.T) {
	input := fstest.MapFS{"pom.xml": {Data: []byte(`<project><parent><groupId>missing</groupId><artifactId>parent</artifactId><version>1</version></parent><artifactId>app</artifactId></project>`)}}
	r, err := ScanReport(context.Background(), input)
	if err == nil || r.Complete || len(r.Diagnostics) == 0 {
		t.Fatalf("missing parent reported as complete: %+v, %v", r, err)
	}
}

func TestReportMultipleProjectsWorkersAndSharedBudget(t *testing.T) {
	for _, limit := range []int64{1 << 20, 1500} {
		var want string
		for _, workers := range []int{1, 2, 4, 8} {
			in := fstest.MapFS{}
			for i := 0; i < 12; i++ {
				in[fmt.Sprintf("p%d/package-lock.json", i)] = &fstest.MapFile{Data: []byte(`{"lockfileVersion":3,"packages":{"node_modules/x":{"version":"1","dependencies":{"y":"*"}},"node_modules/y":{"version":"2"}}}`)}
			}
			r, err := ScanReport(context.Background(), in, _withConcurrent(workers), WithSnapshotID("fixed"), WithResourceLimits(ResourceLimits{MaxTotalReadBytes: limit}))
			if limit > 1500 && err != nil {
				t.Fatal(err)
			}
			if limit == 1500 && (err == nil || r.Complete) {
				t.Fatal("budget not enforced")
			}
			raw, _ := json.Marshal(r)
			if want != "" && want != string(raw) {
				t.Fatalf("worker-dependent report with limit %d", limit)
			}
			want = string(raw)
			ids := map[string]model.Observation{}
			for _, o := range r.Observations {
				ids[o.ID()] = o
			}
			for _, q := range r.Requirements {
				for _, target := range q.Resolved {
					if ids[q.From].Project != ids[target].Project {
						t.Fatal("cross-project binding")
					}
				}
			}
		}
	}
}
func TestReportCancellationAndInvalidLimits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := ScanReport(ctx, fstest.MapFS{})
	if err == nil || r.Complete {
		t.Fatal("cancelled scan complete")
	}
	r, err = ScanReport(context.Background(), fstest.MapFS{}, WithResourceLimits(ResourceLimits{MaxFiles: -1}))
	if err == nil || r.Complete {
		t.Fatal("invalid limit accepted")
	}
}

func TestUnknownManifestVersionAndSumEvidence(t *testing.T) {
	for _, file := range []string{"package.json", "composer.json"} {
		field := "dependencies"
		if file == "composer.json" {
			field = "require"
		}
		r, e := ScanReport(context.Background(), fstest.MapFS{file: {Data: []byte(`{"` + field + `":{"x":"^1","y":"*"}}`)}})
		if e != nil {
			t.Fatal(e)
		}
		if len(r.Components) != 2 {
			t.Fatal("unnamed or lost declarations", r.Components)
		}
		for _, c := range r.Components {
			if c.Key.Version != "" {
				t.Fatal("constraint promoted to version", c)
			}
		}
	}
	sum := "h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	in := fstest.MapFS{"go.mod": {Data: []byte("module example.org/app\nrequire example.org/x v1.2.3\n")}, "go.sum": {Data: []byte("example.org/x v1.2.3/go.mod " + sum + "\nexample.org/y v9.0.0+incompatible " + sum + "\n")}}
	r, e := ScanReport(context.Background(), in)
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Components) != 1 || r.Components[0].Key.Verification != "" {
		t.Fatal("go.sum became inventory or wrong checksum evidence")
	}
	if len(r.Observations) != 1 || r.Observations[0].StartLine != 2 {
		t.Fatal("declaration line lost")
	}
	in["go.sum"].Data = append(in["go.sum"].Data, []byte("example.org/x v1.2.3 "+sum+"\n")...)
	r, e = ScanReport(context.Background(), in)
	if e != nil || r.Components[0].Key.Verification != sum {
		t.Fatal("lost exact checksum", e)
	}
}

func TestCapabilityCandidatesAreNotResolvedEdges(t *testing.T) {
	in := fstest.MapFS{"lib/apk/db/installed": {Data: []byte("P:consumer\nV:1\nD:virtual>=1\n\nP:a\nV:1\np:virtual=1\n\nP:b\nV:2\np:virtual=2\n\n")}}
	r, e := ScanReport(context.Background(), in)
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Components) != 3 || len(r.Requirements) != 1 {
		t.Fatalf("unexpected inventory: %+v", r)
	}
	q := r.Requirements[0]
	if q.Constraint != ">=1" || len(q.Candidates) != 2 || len(q.Resolved) != 0 {
		t.Fatalf("provider ambiguity lost: %+v", q)
	}
}
