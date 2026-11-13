package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// collectPointerTargets walks phi edges to find the variables behind a `*p = x`
// store. A loop-carried address (`while (...) { *p++ = x; }`) makes those edges
// cyclic, and the walk used to recurse without any visited set: a two-phi cycle
// overflowed the goroutine stack (fatal error, not recoverable) and killed the
// engine process mid-scan. See apache/httpd `http2env`.

func TestCollectPointerTargetsPhiSelfEdgeDoesNotOverflow(t *testing.T) {
	_, builder := newTestBuilder(t)

	base := builder.EmitEmptyContainer()
	phi := builder.EmitPhi("cursor", Values{base})
	// A phi that carries itself is what a loop body produces for `*p++ = x`.
	phi.Edge = append(phi.Edge, phi.GetId())

	require.NotPanics(t, func() {
		_ = builder.collectPointerTargets(phi)
	})
}

func TestCollectPointerTargetsTwoPhiCycleDoesNotOverflow(t *testing.T) {
	_, builder := newTestBuilder(t)

	base := builder.EmitEmptyContainer()
	// Two phis referencing each other is the exact shape in the crash: the
	// stack alternated between two pointer values a few million times.
	first := builder.EmitPhi("cursor", Values{base})
	second := builder.EmitPhi("cursor", Values{first})
	first.Edge = append(first.Edge, second.GetId())
	second.Edge = append(second.Edge, first.GetId())

	require.NotPanics(t, func() {
		_ = builder.collectPointerTargets(first)
	})
}

func TestCollectPointerTargetsKeepsDistinctOrigins(t *testing.T) {
	_, builder := newTestBuilder(t)

	// A pointer origin stringifies as `&name#globalID`, and parameters keep that
	// form verbatim, so they stand in for the `&left` / `&right` values a phi
	// carries after `TrimPrefix`.
	left := builder.NewParam("left#1")
	right := builder.NewParam("right#2")
	phi := builder.EmitPhi("cursor", Values{left, right})

	targets := builder.collectPointerTargets(phi)
	require.Equal(t, []pointerTarget{
		{name: "left", globalID: 1},
		{name: "right", globalID: 2},
	}, targets, "a diamond phi must still yield every distinct origin")
}

func TestCollectPointerTargetsDedupesRepeatedOrigin(t *testing.T) {
	_, builder := newTestBuilder(t)

	only := builder.NewParam("cursor#7")
	phi := builder.EmitPhi("cursor", Values{only, only})

	targets := builder.collectPointerTargets(phi)
	require.Equal(t, []pointerTarget{{name: "cursor", globalID: 7}}, targets,
		"both edges point at the same variable")
}
