//go:build windows && cgo

package minirehs

import (
	"os"
	"runtime"
	"runtime/debug"
	"syscall"
	"testing"
	"unsafe"
)

type systemProcessMemory struct {
	Size, PageFaults                                 uint32
	PeakWorkingSet, WorkingSet                       uintptr
	PeakPagedPool, PagedPool, PeakNonPaged, NonPaged uintptr
	Pagefile, PeakPagefile, Private                  uintptr
}

var systemMemoryInfo = syscall.NewLazyDLL("kernel32.dll").NewProc("K32GetProcessMemoryInfo")

func TestProgramSystemResources(t *testing.T) {
	dir := programEvidenceDir(t)
	set := os.Getenv("MINIREHS_RESOURCE_SET")
	if set != "GetRequest" && set != "MITM" {
		t.Fatal("set MINIREHS_RESOURCE_SET GetRequest/MITM; use a fresh process per set")
	}
	type sample struct {
		Stage                                             string
		GoHeapAlloc, GoHeapInuse, GoHeapSys, GoTotalAlloc uint64
		GoMallocs                                         uint64
		Goroutines                                        int
		Windows                                           systemProcessMemory
	}
	var rows []sample
	snapshot := func(stage string) {
		// Resource-only diagnostic, deliberately separate from timings. The
		// lifetime peak is not the current footprint and is not reset here.
		runtime.GC()
		debug.FreeOSMemory()
		var memory systemProcessMemory
		memory.Size = uint32(unsafe.Sizeof(memory))
		ok, _, err := systemMemoryInfo.Call(^uintptr(0), uintptr(unsafe.Pointer(&memory)), uintptr(memory.Size))
		runtime.KeepAlive(&memory)
		if ok == 0 {
			t.Fatal(err)
		}
		var goMem runtime.MemStats
		runtime.ReadMemStats(&goMem)
		rows = append(rows, sample{stage, goMem.HeapAlloc, goMem.HeapInuse, goMem.HeapSys, goMem.TotalAlloc, goMem.Mallocs, runtime.NumGoroutine(), memory})
	}
	snapshot("baseline")
	pats := getRequestImagePatterns(t)
	if set == "MITM" {
		pats, _ = compilableMITMPatterns(t)
	}
	db, err := Compile(pats, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	snapshot("compiled_post_gc")
	image, err := db.MarshalImage()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadImage(image)
	if err != nil {
		t.Fatal(err)
	}
	snapshot("old_and_new_db_post_gc")
	a, err := db.NewScratch()
	if err != nil {
		t.Fatal(err)
	}
	b, err := loaded.NewScratch()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"HTTP912", "Sparse16K", "DenseHTTP16K", "Zero16K"} {
		data := programRuntimeInputs()[name]
		if err := db.Scan(data, a, nil); err != nil {
			t.Fatal(err)
		}
		if err := loaded.Scan(data, b, nil); err != nil {
			t.Fatal(err)
		}
	}
	snapshot("old_and_new_with_warm_owned_scratch")
	runtime.KeepAlive(pats)
	runtime.KeepAlive(image)
	a.Close()
	b.Close()
	db.Close()
	loaded.Close()
	a, b, db, loaded, pats, image = nil, nil, nil, nil, nil, nil
	snapshot("closed_and_unreachable_post_gc_scavenge")
	writeProgramJSON(t, dir, "system-resources.json", map[string]interface{}{
		"set": set, "P": runtime.GOMAXPROCS(0), "logical_cpus": runtime.NumCPU(), "samples": rows,
		"notes":      "Fresh-process diagnostic with explicitGC/scavenge, not a timing benchmark. GoHeap/TotalAlloc, WindowsPrivate(commit)/currentWorkingSet/lifetimePeakWorkingSet are different quantities, never summed. Old+newDB/scratch deliberately coexist for reload. Native requested-table counters are separately proven with the unchanged native-source probe; PCRE/Teddy/allocator/runtime not included there. A lifetime peak never shrinks at Close; current private/resident need not exactly equal baseline due runtime/DLL caches.",
		"memory_api": "https://learn.microsoft.com/en-us/windows/win32/api/psapi/nf-psapi-getprocessmemoryinfo",
	})
}
