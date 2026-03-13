package utils

import (
	"github.com/yaklang/yaklang/common/log"
	"sync"
	"testing"
)

func TestNewStringRoundRobinSelector(t *testing.T) {
	log.Info("start test string round robin")
	selector := NewStringRoundRobinSelector("a", "b", "c")

	var a, b, c string
	a = selector.Next()
	b = selector.Next()
	c = selector.Next()

	if a != b && a != c && b != c {
		return
	}

	t.Logf("1:%v 2:%v 3:%v", a, b, c)
	t.Fail()
}

func TestStringRoundRobinSelectorNextConcurrent(t *testing.T) {
	selector := NewStringRoundRobinSelector("node-a", "node-b")

	const total = 1000
	results := make(chan string, total)
	var wg sync.WaitGroup

	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- selector.Next()
		}()
	}

	wg.Wait()
	close(results)

	counts := map[string]int{}
	for node := range results {
		counts[node]++
	}

	if counts["node-a"]+counts["node-b"] != total {
		t.Fatalf("unexpected total picks: counts=%v total=%d", counts, total)
	}

	diff := counts["node-a"] - counts["node-b"]
	if diff < 0 {
		diff = -diff
	}
	if diff > 1 {
		t.Fatalf("round robin became imbalanced under concurrency: counts=%v", counts)
	}
}
