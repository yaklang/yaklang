package model

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/yaklang/yaklang/common/sca/core/budget"
)

// Scale is input records/edges, not just distinct component count. In the
// dense case a sqrt(N)-vertex clique supplies O(N) explicit edges. A cycle
// stays a cycle; normalization must never recursively traverse relationships.
func BenchmarkNormalizeTopology(b *testing.B) {
	for _, shape := range []string{"duplicates", "sources", "dense", "cycle", "ranges"} {
		for _, n := range []int{100, 1000, 10000, 100000} {
			b.Run(fmt.Sprintf("%s/%d", shape, n), func(b *testing.B) {
				input := Report{Complete: true}
				vertices := n
				if shape == "duplicates" {
					vertices = 1
				}
				if shape == "dense" {
					vertices = int(math.Sqrt(float64(n)))
				}
				for i := 0; i < n; i++ {
					k := ComponentKey{Ecosystem: "npm", Name: "same-name", Version: fmt.Sprint(i % vertices)}
					if shape == "sources" {
						k.Version = "1"
						k.Source = fmt.Sprintf("registry-%d", i)
					}
					input.Components = append(input.Components, Component{Key: k})
				}
				for i := 0; i < vertices; i++ {
					input.Observations = append(input.Observations, Observation{Component: input.Components[i].Key.ID(), Snapshot: "s", Project: "p", NativeID: fmt.Sprint(i), Kind: "locked"})
				}
				for i := 0; i < n; i++ {
					from, to := i%vertices, (i+1)%vertices
					if shape == "dense" {
						from = (i / vertices) % vertices
						to = i % vertices
					}
					q := Requirement{From: input.Observations[from].ID(), Target: "same-name", Resolved: []string{input.Observations[to].ID()}}
					if shape == "ranges" {
						q.Resolved = nil
						q.Constraint = fmt.Sprintf(">=%d < %d", i, i+2)
					}
					input.Requirements = append(input.Requirements, q)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					r := Report{Complete: true, Components: append([]Component(nil), input.Components...), Observations: append([]Observation(nil), input.Observations...), Requirements: append([]Requirement(nil), input.Requirements...)}
					st := budget.From(budget.Bind(context.Background(), budget.Limits{MaxResultBytes: 8 << 30}))
					if err := r.NormalizeBudget(st); err != nil {
						b.Fatal(err)
					}
					if len(r.Components) != vertices || len(r.Observations) != vertices {
						b.Fatal("lost identities", len(r.Components), len(r.Observations), vertices)
					}
					expected := n
					if shape == "duplicates" {
						expected = 1
					}
					if shape == "dense" {
						expected = vertices * vertices
					}
					if len(r.Requirements) != expected {
						b.Fatal("lost or invented edges", len(r.Requirements), expected)
					}
					b.ReportMetric(float64(st.ResultBytes())/float64(n), "reserved-B/record")
				}
			})
		}
	}
}
