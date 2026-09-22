package budget

import (
	"fmt"
)

// Limits defines per-scan budgets. Zero selects the versioned defaults;
// negative values and values above the supported maxima are rejected.
type Limits struct {
	MaxArchiveEntries, MaxArchiveDepth, MaxFieldBytes, MaxSyntaxDepth, MaxExpressionNodes, MaxResolveSteps, MaxReferenceDepth int
	MaxExpandedBytes                                                                                                          int64

	MaxFiles, MaxCandidateFiles              int
	MaxFileBytes, MaxTotalReadBytes          int64
	MaxComponents, MaxObservations, MaxEdges int
	MaxResultBytes                           int64
}

func (l Limits) Normalize() (Limits, error) {
	ints := []struct {
		p        *int
		def, max int
	}{{&l.MaxFiles, 200000, 1000000}, {&l.MaxCandidateFiles, 10000, 100000}, {&l.MaxComponents, 200000, 1000000}, {&l.MaxObservations, 400000, 2000000}, {&l.MaxEdges, 1000000, 5000000}}
	ints = append(ints, []struct {
		p        *int
		def, max int
	}{{&l.MaxArchiveEntries, 100000, 1000000}, {&l.MaxArchiveDepth, 8, 32}, {&l.MaxFieldBytes, 1 << 20, 16 << 20}, {&l.MaxSyntaxDepth, 64, 128}, {&l.MaxExpressionNodes, 100000, 1000000}, {&l.MaxResolveSteps, 1000000, 10000000}, {&l.MaxReferenceDepth, 64, 128}}...)
	for _, x := range ints {
		if *x.p < 0 || *x.p > x.max {
			return l, fmt.Errorf("invalid resource limit")
		}
		if *x.p == 0 {
			*x.p = x.def
		}
	}
	big := []struct {
		p        *int64
		def, max int64
	}{{&l.MaxFileBytes, 64 << 20, 256 << 20}, {&l.MaxTotalReadBytes, 512 << 20, 2 << 30}}
	big = append(big, struct {
		p        *int64
		def, max int64
	}{&l.MaxExpandedBytes, 512 << 20, 2 << 30}, struct {
		p        *int64
		def, max int64
	}{&l.MaxResultBytes, 256 << 20, 2 << 30})
	for _, x := range big {
		if *x.p < 0 || *x.p > x.max {
			return l, fmt.Errorf("invalid byte limit")
		}
		if *x.p == 0 {
			*x.p = x.def
		}
	}
	return l, nil
}
