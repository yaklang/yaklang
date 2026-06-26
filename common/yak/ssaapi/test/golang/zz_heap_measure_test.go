package ssaapi

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestGoCompileHeapMeasure compiles a synthetic, declaration-heavy multi-file Go
// project in memory and reports peak HeapInuse. Pair with YAK_SSA_HEAP_LOG=1 to
// see retained heap after each phase (the f1 line isolates the AST win). A/B:
//
//	YAK_SSA_HEAP_LOG=1 go test ./.../golang -run TestGoCompileHeapMeasure -count=1 -v
//
// Throwaway measurement helper, not a correctness assertion.
func TestGoCompileHeapMeasure(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", "module example.com/m\n\ngo 1.20\n")

	const pkgs = 150
	const typesPerFile = 180
	const varsPerFile = 120
	for p := 0; p < pkgs; p++ {
		var sb strings.Builder
		fmt.Fprintf(&sb, "package pkg%d\n\n", p)
		if p > 0 {
			fmt.Fprintf(&sb, "import dep \"example.com/m/pkg%d\"\n\n", p-1)
		}
		// Many type declarations (non-body AST that should be freed after pass1).
		for i := 0; i < typesPerFile; i++ {
			fmt.Fprintf(&sb, "type S%d_%d struct {\n\tf0 int\n\tf1 string\n\tf2 float64\n\tf3 bool\n\tf4 []int\n\tf5 map[string]int\n}\n", p, i)
		}
		// Many global var declarations (also non-body AST).
		for i := 0; i < varsPerFile; i++ {
			fmt.Fprintf(&sb, "var V%d_%d int = %d\n", p, i, i)
		}
		// One tiny function body (so per-file retained body subtree is negligible).
		fmt.Fprintf(&sb, "func F%d() int {\n\treturn V%d_0\n}\n", p, p)
		if p > 0 {
			fmt.Fprintf(&sb, "func G%d() int {\n\treturn dep.F%d()\n}\n", p, p-1)
		}
		vf.AddFile(fmt.Sprintf("src/main/go/pkg%d/f.go", p), sb.String())
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

	runtime.GC()
	start := time.Now()
	progs, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.GO))
	elapsed := time.Since(start)
	close(stop)
	wg.Wait()
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	mode := "skeleton+detach"
	t.Logf("MODE=%-26s files=%d peak_heap_inuse=%6.1fMB compile=%v programs=%d",
		mode, pkgs, float64(atomic.LoadInt64(&peak))/(1024*1024), elapsed, len(progs))
}
