package ssaapi

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// This file pins the descent-side half of the dataflow input-elimination
// optimization: a complete topdef descent publishes the nodes it fully
// expanded so the fanout can skip those inputs later (the coverage law), while
// a truncated or canceled descent publishes nothing. The fanout-side half
// (indexing inputs, skipping covered ones, gating) is unit-tested in
// common/syntaxflow/sfvm/dataflow_fanout_eliminator_test.go.

// recordingFanoutCollector records the coverage events of one or more
// descents, standing in for the sfvm eliminator session.
type recordingFanoutCollector struct {
	mu        sync.Mutex
	marks     map[int64]struct{}
	cuts      int
	publishes int
}

func newRecordingFanoutCollector() *recordingFanoutCollector {
	return &recordingFanoutCollector{marks: make(map[int64]struct{})}
}

func (c *recordingFanoutCollector) MarkVisited(id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.marks[id] = struct{}{}
}

func (c *recordingFanoutCollector) OnCycleCut() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cuts++
}

func (c *recordingFanoutCollector) Publish() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.publishes++
}

func (c *recordingFanoutCollector) snapshot() (marks map[int64]struct{}, cuts, publishes int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	marks = make(map[int64]struct{}, len(c.marks))
	for id := range c.marks {
		marks[id] = struct{}{}
	}
	return marks, c.cuts, c.publishes
}

// The chain cmd -> a -> b -> c: a descent from c must fully expand b and a.
const elimChainJava = `
package com.example;

class Sink {
    static void consume(Object o) {}
}

class CaseElim {
    public void run(String cmd) {
        String a = cmd;
        String b = a;
        String c = b;
        Sink.consume(c);
    }
}
`

func TestTopDefDescentPublishesCoverageMarks(t *testing.T) {
	prog, err := Parse(elimChainJava, WithLanguage(ssaconfig.JAVA))
	require.NoError(t, err)
	sources := prog.Ref("c")
	require.NotEmpty(t, sources, "the chain variable c must resolve to a value")

	var chainMarks int
	for _, source := range sources {
		coll := newRecordingFanoutCollector()
		_ = source.GetTopDefs(WithFanoutCollector(coll))

		marks, cuts, publishes := coll.snapshot()
		require.Equal(t, 0, cuts, "the chain has no cycle")
		require.Equal(t, 1, publishes, "a complete descent must publish exactly once")
		require.Contains(t, marks, source.GetId(), "the descent root is fully expanded")
		if len(marks) > 1 {
			chainMarks = len(marks)
		}
	}
	require.Greater(t, chainMarks, 1,
		"the descent must mark the chain's intermediate nodes, not just the source")
}

func TestTruncatedDescentDoesNotPublishCoverage(t *testing.T) {
	coll := newRecordingFanoutCollector()

	limited := NewAnalyzeContext(WithFanoutCollector(coll))
	limited.reachedDepthLimited = true
	limited.publishFanoutCoverage()
	_, _, publishes := coll.snapshot()
	require.Zero(t, publishes, "a limit-truncated descent must not promote its marks")

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled := NewAnalyzeContext(WithFanoutCollector(coll), WithExclusiveContext(canceledCtx))
	canceled.publishFanoutCoverage()
	_, _, publishes = coll.snapshot()
	require.Zero(t, publishes, "a canceled descent must not promote its marks")
}
