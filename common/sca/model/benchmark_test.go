package model

import (
	"fmt"
	"testing"
)

func BenchmarkNormalize(b *testing.B) {
	for _, n := range []int{100, 1000, 10000, 50000, 100000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			components := make([]Component, n)
			obs := make([]Observation, n)
			for i := range components {
				components[i] = Component{Key: ComponentKey{Ecosystem: "npm", Name: "same-name", Version: fmt.Sprint(i)}}
				obs[i] = Observation{Component: components[i].Key.ID(), Snapshot: "s", Project: "p", File: "lock", NativeID: fmt.Sprint(i), Kind: "locked"}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				r := Report{Components: append([]Component(nil), components...), Observations: append([]Observation(nil), obs...), Complete: true}
				r.Normalize()
				if len(r.Components) != n || len(r.Observations) != n {
					b.Fatal("lost observations")
				}
			}
		})
	}
}
