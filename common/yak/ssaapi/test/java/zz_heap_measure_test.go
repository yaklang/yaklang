package java

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestJavaCompileHeapMeasure compiles a real Java tree when JAVA_HEAP_PROJECT is set.
// Pair with YAK_SSA_HEAP_LOG=1 for per-phase retained heap (f1 isolates AST win).
//
//	JAVA_HEAP_PROJECT=/path/to/spring YAK_SSA_HEAP_LOG=1 go test ./.../java -run TestJavaCompileHeapMeasure -count=1 -v -timeout=60m
func TestJavaCompileHeapMeasure(t *testing.T) {
	root := os.Getenv("JAVA_HEAP_PROJECT")
	if root == "" {
		t.Skip("set JAVA_HEAP_PROJECT to a Java project directory")
	}

	var peak int64
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		var m runtime.MemStats
		for {
			select {
			case <-stop:
				return
			default:
			}
			runtime.ReadMemStats(&m)
			if v := int64(m.HeapInuse); v > atomic.LoadInt64(&peak) {
				atomic.StoreInt64(&peak, v)
			}
			time.Sleep(500 * time.Microsecond)
		}
	}()

	branch := os.Getenv("YAK_HEAP_BRANCH_LABEL")
	if branch == "" {
		branch = "current"
	}
	progName := fmt.Sprintf("heap-measure-%s-%d", branch, time.Now().UnixNano())

	runtime.GC()
	start := time.Now()
	progs, err := ssaapi.ParseProjectFromPath(root,
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithMemory(),
		ssaapi.WithProgramName(progName),
	)
	elapsed := time.Since(start)
	close(stop)
	wg.Wait()
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	mode := "skeleton+detach"
	t.Logf("BRANCH=%-12s MODE=%-26s path=%s peak_heap_inuse=%6.1fMB compile=%v programs=%d",
		branch, mode, root, float64(atomic.LoadInt64(&peak))/(1024*1024), elapsed, len(progs))
}

// TestJavaCompileHeapMeasureSpring is a convenience wrapper for spring-cloud-netflix.
// It is intentionally opt-in because normal package tests must not compile a
// real project tree just because this machine happens to have one checked out.
func TestJavaCompileHeapMeasureSpring(t *testing.T) {
	const defaultSpring = "/home/wlz/Target/spring-project/spring-cloud-netflix"
	if os.Getenv("JAVA_HEAP_PROJECT") == "" {
		t.Skipf("set JAVA_HEAP_PROJECT to run heap measurement, for example %s", defaultSpring)
	}
	TestJavaCompileHeapMeasure(t)
}
