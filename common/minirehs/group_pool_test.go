package minirehs

import (
	"bytes"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestGroupCloseOwnsWorkerScratch(t *testing.T) {
	g, err := BuildGroup([]string{`token[0-9]+`}, WithGroupExistenceOnly(true))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	sc, err := g.getScratchErr()
	if err != nil {
		t.Fatal(err)
	}
	root := sc.(*scratch)
	defer root.Close() // also clean up the known-bad pool during reproduction
	if err := g.db.Scan([]byte("token123"), sc, nil); err != nil {
		g.putScratch(sc)
		t.Fatal(err)
	}
	g.putScratch(sc)
	if root.mergedTasks == nil {
		t.Fatal("fixture did not start the actual permanent workers")
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	for name, closed := range map[string]bool{
		"merged":   programMergedTasksClosed(root),
		"assert":   programAssertTasksClosed(root),
		"gated":    programGatedTasksClosed(root),
		"anchored": programAnchoredTasksClosed(root),
	} {
		if !closed {
			t.Errorf("Group.Close left %s scratch worker channel open", name)
		}
	}
}

func programMergedTasksClosed(s *scratch) bool {
	select {
	case _, open := <-s.mergedTasks:
		return !open
	default:
		return false
	}
}
func programAssertTasksClosed(s *scratch) bool {
	select {
	case _, open := <-s.assertTasks:
		return !open
	default:
		return false
	}
}
func programGatedTasksClosed(s *scratch) bool {
	select {
	case _, open := <-s.gatedTasks:
		return !open
	default:
		return false
	}
}
func programAnchoredTasksClosed(s *scratch) bool {
	select {
	case _, open := <-s.anchoredTasks:
		return !open
	default:
		return false
	}
}

func TestGroupPoolEvictionAndClose(t *testing.T) {
	g, err := BuildGroup([]string{`token[0-9]+`}, WithGroupExistenceOnly(true))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	var borrowed []*scratch
	for i := 0; i < maxGroupIdleScratch+3; i++ {
		s, err := g.getScratchErr()
		if err != nil {
			t.Fatal(err)
		}
		root := s.(*scratch)
		defer root.Close()
		if err := g.db.Scan([]byte("token123"), root, nil); err != nil {
			g.putScratch(root)
			t.Fatal(err)
		}
		borrowed = append(borrowed, root)
	}
	for _, s := range borrowed {
		g.putScratch(s)
	}
	runtime.GC() // channel-owned idle scratch must survive GC, or close explicitly
	closed := 0
	for _, s := range borrowed {
		if programMergedTasksClosed(s) {
			closed++
		}
	}
	if closed != len(borrowed)-cap(g.pool) {
		t.Fatalf("idle bound/eviction closed=%d borrowed=%d idle capacity=%d", closed, len(borrowed), cap(g.pool))
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	for _, s := range borrowed {
		if !programMergedTasksClosed(s) {
			t.Fatal("Close stranded a cached scratch")
		}
	}
}

func TestGroupCloseWaitsForBorrowers(t *testing.T) {
	g, err := BuildGroup([]string{`token[0-9]+`}, WithGroupExistenceOnly(true))
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	scanDone, closeDone := make(chan error, 1), make(chan error, 1)
	go func() {
		scanDone <- g.ScanErr([]byte("token123"), func(*GroupMatch) bool { close(entered); <-release; return true })
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not enter callback")
	}
	go func() { closeDone <- g.Close() }()
	deadline := time.Now().Add(5 * time.Second)
	for g.state.Load()&groupClosingBit == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if g.state.Load()&groupClosingBit == 0 {
		t.Fatal("Close did not stop admission")
	}
	if _, err := g.MatchErr("token123"); !errors.Is(err, ErrClosed) {
		t.Fatalf("new borrower admitted during Close: %v", err)
	}
	select {
	case err := <-closeDone:
		t.Fatalf("Close returned before current scan finished: %v", err)
	default:
	}
	unblock()
	select {
	case err := <-scanDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("current scan did not finish")
	}
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not join scratch")
	}
}

func TestGroupOversizedScratchIsReleased(t *testing.T) {
	g, err := BuildGroup([]string{`token[0-9]+`}, WithGroupExistenceOnly(true))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	s, err := g.getScratchErr()
	if err != nil {
		t.Fatal(err)
	}
	root := s.(*scratch)
	defer root.Close()
	if err := g.db.Scan([]byte("token123"), root, nil); err != nil {
		g.putScratch(root)
		t.Fatal(err)
	}
	root.cpairs = make([]int32, int(groupScratchBufferBudget/4)+1)
	g.putScratch(root)
	if !programMergedTasksClosed(root) || len(g.pool) != 0 {
		t.Fatal("oversized scratch was retained")
	}
	fresh, err := g.getScratchErr()
	if err != nil {
		t.Fatal(err)
	}
	if fresh == root {
		t.Fatal("closed scratch reused after a burst")
	}
	if err := g.db.Scan([]byte("token456"), fresh, nil); err != nil {
		g.putScratch(fresh)
		t.Fatal(err)
	}
	g.putScratch(fresh)
}

func TestGroupPoolCountsHistoricalRangeCapacity(t *testing.T) {
	s := &scratch{anchorRanges: make([][]anchorSpan, 3), anchorBatch: []int32{0}}
	s.anchorRanges[0] = make([]anchorSpan, 0, 100)
	if s.groupPoolOversized() {
		t.Fatal("small range unexpectedly discarded")
	}
	want := s.groupRangeBytes
	s.anchorBatch = []int32{1}
	s.anchorRanges[1] = make([]anchorSpan, 0, 200)
	if s.groupPoolOversized() || s.groupRangeBytes != 3*want {
		t.Fatal("lost capacity of a previous record's range")
	}
	s.anchorBatch = []int32{0}
	if s.groupPoolOversized() || s.groupRangeBytes != 3*want {
		t.Fatal("double-counted an unchanged range")
	}
	s.anchorRanges[0] = make([]anchorSpan, 0, 150)
	if s.groupPoolOversized() || s.groupRangeBytes != want*7/2 {
		t.Fatal("range growth not accounted")
	}
	s.anchorRanges[0] = nil
	if s.groupPoolOversized() || s.groupRangeBytes != 2*want {
		t.Fatal("released range still counted")
	}
}

func TestGroupConcurrentScratchReuseAndInputOverwrite(t *testing.T) {
	g, err := BuildGroup([]string{`token[0-9]+`, `foo(?=!)`}, WithGroupExistenceOnly(true))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	var workers sync.WaitGroup
	for n := 0; n < 8; n++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := 0; i < 50; i++ {
				data := []byte("token123 foo!")
				indexes, err := g.MatchedIndexesErr(data)
				if err != nil || len(indexes) != 2 || indexes[0] == indexes[1] {
					t.Errorf("concurrent full result: %v %v", indexes, err)
					return
				}
				found, err := g.MatchErr(data)
				if err != nil || !found {
					t.Errorf("concurrent early stop: %v %v", found, err)
					return
				}
				for j := range data {
					data[j] = 'x'
				} // every reader must already be joined
				if indexes, err := g.MatchedIndexesErr(data); err != nil || len(indexes) != 0 {
					t.Errorf("stale subject: %v %v", indexes, err)
					return
				}
			}
		}()
	}
	workers.Wait()
}

func TestGroupFinalizerClosesOwnedCache(t *testing.T) {
	makeDetached := func() *scratch {
		g, err := BuildGroup([]string{`token[0-9]+`}, WithGroupExistenceOnly(true))
		if err != nil {
			t.Fatal(err)
		}
		s, err := g.getScratchErr()
		if err != nil {
			t.Fatal(err)
		}
		if err := g.db.Scan([]byte("token123"), s, nil); err != nil {
			g.putScratch(s)
			g.Close()
			t.Fatal(err)
		}
		g.putScratch(s)
		return s.(*scratch) // no Group reference is retained
	}
	root := makeDetached()
	defer root.Close()
	deadline := time.Now().Add(5 * time.Second)
	for !programMergedTasksClosed(root) && time.Now().Before(deadline) {
		runtime.GC()
		time.Sleep(time.Millisecond)
	}
	if !programMergedTasksClosed(root) {
		t.Fatal("unreachable Group retained permanent worker scratch")
	}
}

func TestGroupRealBurstThenSmallRecord(t *testing.T) {
	g, err := BuildGroup([]string{`ab`, `end-marker`}, WithGroupExistenceOnly(true))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	s, err := g.getScratchErr()
	if err != nil {
		t.Fatal(err)
	}
	root := s.(*scratch)
	defer root.Close()
	g.putScratch(s)
	input := append(bytes.Repeat([]byte("ab"), 300000), []byte(" end-marker")...)
	indexes, err := g.MatchedIndexesErr(input)
	if err != nil || len(indexes) != 2 || indexes[0] == indexes[1] {
		t.Fatalf("burst did not process the full input through the end marker: %v %v", indexes, err)
	}
	if root.groupPoolBufferBytes() <= groupScratchBufferBudget || len(g.pool) != 0 {
		t.Fatal("actual burst buffers stayed in the idle cache")
	}
	indexes, err = g.MatchedIndexesErr([]byte("ab"))
	if err != nil || len(indexes) != 1 || indexes[0] != 0 || len(g.pool) != 1 {
		t.Fatalf("small scan after burst: %v %v", indexes, err)
	}
}

func BenchmarkProgramGroupReuse(b *testing.B) {
	for _, scenario := range []string{"SmallLiteral", "AnchoredPCRE", "GetRequest"} {
		b.Run(scenario, func(b *testing.B) {
			var exprs []string
			var data []byte
			switch scenario {
			case "SmallLiteral":
				exprs = []string{`alpha`, `beta`, `gamma`}
				data = []byte("alpha beta")
			case "AnchoredPCRE":
				exprs = []string{`token[0-9]+`, `abc(?=!)`, `nope[ab]{2}`}
				data = []byte("prefix token123 abc! tail")
			case "GetRequest":
				for _, p := range getRequestImagePatterns(b) {
					exprs = append(exprs, buildExprWithFlags(p))
				}
				data = programRuntimeInputs()["HTTP912"]
			}
			g, err := BuildGroup(exprs, WithGroupExistenceOnly(true))
			if err != nil {
				b.Fatal(err)
			}
			defer g.Close()
			want, err := g.MatchedIndexesErr(data)
			if err != nil || len(want) == 0 {
				b.Fatalf("invalid warmup: %v %v", want, err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := g.MatchedIndexesErr(data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
