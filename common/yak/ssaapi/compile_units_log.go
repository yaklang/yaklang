package ssaapi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// heapLogEnabled gates retained-heap phase logging. Set YAK_SSA_HEAP_LOG=1 to print
// GC'd HeapInuse after each compile phase. Set YAK_SSA_HEAP_PROFILE_DIR=<dir> to write
// a heap profile (pprof) after each phase (GC first).
var heapLogEnabled = envFlagEnabled("YAK_SSA_HEAP_LOG")

func envFlagEnabled(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return value != "" && value != "0" && value != "false" && value != "no" && value != "off" && value != "disable" && value != "disabled"
}

func captureHeapMetrics() float64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.HeapInuse) / (1024 * 1024)
}

func logPhaseHeap(tag string) {
	profileDir := heapProfileDir()
	if !heapLogEnabled && profileDir == "" {
		return
	}
	// Note: no runtime.GC() here — the per-unit flush already GCs once at unit
	// end, and an extra GC per phase was deemed too frequent. The retained-heap
	// numbers and opt-in heap profiles therefore include un-collected garbage;
	// acceptable for a diagnostic. Run GODEBUG=gctrace=1 or a manual GC if you
	// need a precise retained snapshot.
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	if heapLogEnabled {
		fmt.Fprintf(os.Stderr, "[ssa.heap] %-16s retained_HeapInuse=%7.1fMB HeapObjects=%d\n", tag, float64(m.HeapInuse)/(1024*1024), m.HeapObjects)
	}
	if profileDir != "" {
		_ = os.MkdirAll(profileDir, 0o755)
		target := filepath.Join(profileDir, normalizeHeapProfileName(tag)+".heap.pb.gz")
		f, err := os.Create(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ssa.heap] profile write failed %s: %v\n", target, err)
			return
		}
		if err := pprof.WriteHeapProfile(f); err != nil {
			fmt.Fprintf(os.Stderr, "[ssa.heap] profile write failed %s: %v\n", target, err)
		}
		_ = f.Close()
		if heapLogEnabled {
			fmt.Fprintf(os.Stderr, "[ssa.heap] profile saved %s\n", target)
		}
	}
}

func heapProfileDir() string {
	raw := strings.TrimSpace(os.Getenv("YAK_SSA_HEAP_PROFILE_DIR"))
	switch strings.ToLower(raw) {
	case "", "0", "false", "no", "off", "none", "disable", "disabled":
		return ""
	default:
		return raw
	}
}

func normalizeHeapProfileName(tag string) string {
	replacer := strings.NewReplacer("/", "_", " ", "_", ".", "_")
	return replacer.Replace(tag)
}

func compileUnitLogEnabled() bool {
	return envFlagEnabled("YAK_SSA_COMPILE_UNIT_LOG")
}

type compileUnitPlanLog struct {
	Program       string                    `json:"program"`
	Language      string                    `json:"language"`
	SpillMode     string                    `json:"spill_mode"`
	CacheMode     string                    `json:"cache_mode"`
	WriterRequest bool                      `json:"writer_cache_requested"`
	WriterEnabled bool                      `json:"writer_cache_enabled"`
	Units         []compileUnitPlanUnitLog  `json:"units"`
	Edges         []UnitRef                 `json:"edges"`
	SCCOrder      [][]string                `json:"scc_order"`
	Batches       []compileUnitPlanBatchLog `json:"batches"`
	UnitCount     int                       `json:"unit_count"`
	EdgeCount     int                       `json:"edge_count"`
	SCCCount      int                       `json:"scc_count"`
	BatchCount    int                       `json:"batch_count"`
	BatchMinFiles int                       `json:"batch_min_files"`
	BatchMinBytes int64                     `json:"batch_min_bytes"`
}

type compileUnitPlanUnitLog struct {
	Key       string   `json:"key"`
	Path      string   `json:"path"`
	Language  string   `json:"language"`
	Files     []string `json:"files"`
	FileCount int      `json:"file_count"`
	Bytes     int64    `json:"bytes"`
}

type compileUnitPlanBatchLog struct {
	Index     int      `json:"index"`
	SCCStart  int      `json:"scc_start"`
	SCCEnd    int      `json:"scc_end"`
	Units     []string `json:"units"`
	UnitCount int      `json:"unit_count"`
	FileCount int      `json:"file_count"`
	Bytes     int64    `json:"bytes"`
}

func logCompileUnitPlan(
	prog *ssa.Program,
	language string,
	plan *UnitPlan,
	batches []compileUnitExecutionBatch,
	batchMinFiles int,
	batchMinBytes int64,
	spillMode string,
	cacheMode string,
	writerRequested bool,
	writerEnabled bool,
) {
	if prog == nil || plan == nil {
		return
	}
	if !compileUnitLogEnabled() && os.Getenv("YAK_SSA_COMPILE_UNIT_LOG_DIR") == "" {
		return
	}
	payload := buildCompileUnitPlanLog(prog.Name, language, plan, batches, batchMinFiles, batchMinBytes, spillMode, cacheMode, writerRequested, writerEnabled)
	prog.ProcessInfof("[SSA/unit-plan] program=%s language=%s spill=%s cache=%s writer_requested=%v writer_enabled=%v units=%d edges=%d scc=%d batches=%d batch_min_files=%d batch_min_bytes=%d",
		payload.Program, payload.Language, payload.SpillMode, payload.CacheMode, payload.WriterRequest, payload.WriterEnabled, payload.UnitCount, payload.EdgeCount, payload.SCCCount, payload.BatchCount, payload.BatchMinFiles, payload.BatchMinBytes)
	if compileUnitLogEnabled() {
		for _, unit := range payload.Units {
			firstFile, lastFile := "", ""
			if len(unit.Files) > 0 {
				firstFile = unit.Files[0]
				lastFile = unit.Files[len(unit.Files)-1]
			}
			prog.ProcessInfof("[SSA/unit-plan] unit key=%s path=%s files=%d bytes=%d first=%s last=%s",
				unit.Key, unit.Path, unit.FileCount, unit.Bytes, firstFile, lastFile)
		}
		for _, edge := range payload.Edges {
			prog.ProcessInfof("[SSA/unit-plan] edge from=%s to=%s kind=%s raw=%s", edge.From, edge.To, edge.Kind, edge.Raw)
		}
		for index, scc := range payload.SCCOrder {
			prog.ProcessInfof("[SSA/unit-plan] scc(%d/%d) units=%s", index+1, len(payload.SCCOrder), strings.Join(scc, ","))
		}
		for _, batch := range payload.Batches {
			prog.ProcessInfof("[SSA/unit-plan] batch(%d/%d) scc=%d-%d units=%d files=%d bytes=%d keys=%s",
				batch.Index, payload.BatchCount, batch.SCCStart, batch.SCCEnd, batch.UnitCount, batch.FileCount, batch.Bytes, strings.Join(batch.Units, ","))
		}
	}
	if dir := os.Getenv("YAK_SSA_COMPILE_UNIT_LOG_DIR"); dir != "" {
		target, err := writeCompileUnitPlanLogFile(dir, payload)
		if err != nil {
			prog.ProcessInfof("[SSA/unit-plan] write failed file=%s error=%v", target, err)
			return
		}
		prog.ProcessInfof("[SSA/unit-plan] wrote plan file=%s", target)
	}
}

func writeCompileUnitPlanLogFile(dir string, payload compileUnitPlanLog) (string, error) {
	if dir == "" {
		return "", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	target := filepath.Join(dir, normalizeHeapProfileName(payload.Program)+"-compile-unit-plan.json")
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return target, err
	}
	return target, nil
}

func buildCompileUnitPlanLog(
	program string,
	language string,
	plan *UnitPlan,
	batches []compileUnitExecutionBatch,
	batchMinFiles int,
	batchMinBytes int64,
	spillMode string,
	cacheMode string,
	writerRequested bool,
	writerEnabled bool,
) compileUnitPlanLog {
	unitKeys := make([]string, 0, len(plan.Units))
	for key := range plan.Units {
		unitKeys = append(unitKeys, key)
	}
	sort.Strings(unitKeys)
	units := make([]compileUnitPlanUnitLog, 0, len(unitKeys))
	for _, key := range unitKeys {
		unit := plan.Units[key]
		if unit == nil {
			continue
		}
		files := append([]string(nil), unit.Files...)
		sort.Strings(files)
		units = append(units, compileUnitPlanUnitLog{
			Key:       unit.Key,
			Path:      unit.Path,
			Language:  fmt.Sprintf("%v", unit.Language),
			Files:     files,
			FileCount: len(files),
			Bytes:     unit.Bytes,
		})
	}
	order := make([][]string, 0, len(plan.Order))
	for _, scc := range plan.Order {
		keys := make([]string, 0, len(scc))
		for _, unit := range scc {
			if unit == nil {
				continue
			}
			keys = append(keys, unit.Key)
		}
		sort.Strings(keys)
		order = append(order, keys)
	}
	batchLogs := make([]compileUnitPlanBatchLog, 0, len(batches))
	for index, batch := range batches {
		keys := append([]string(nil), batch.unitKeys...)
		batchLogs = append(batchLogs, compileUnitPlanBatchLog{
			Index:     index + 1,
			SCCStart:  batch.startSCC + 1,
			SCCEnd:    batch.endSCC + 1,
			Units:     keys,
			UnitCount: len(keys),
			FileCount: batch.files,
			Bytes:     batch.bytes,
		})
	}
	return compileUnitPlanLog{
		Program:       program,
		Language:      language,
		SpillMode:     spillMode,
		CacheMode:     cacheMode,
		WriterRequest: writerRequested,
		WriterEnabled: writerEnabled,
		Units:         units,
		Edges:         append([]UnitRef(nil), plan.Edges...),
		SCCOrder:      order,
		Batches:       batchLogs,
		UnitCount:     len(plan.Units),
		EdgeCount:     len(plan.Edges),
		SCCCount:      len(plan.Order),
		BatchCount:    len(batchLogs),
		BatchMinFiles: batchMinFiles,
		BatchMinBytes: batchMinBytes,
	}
}

func recordFilePerformance(
	recorder *diagnostics.Recorder,
	metricName string,
	logLabel string,
	path string,
	duration time.Duration,
) {
	if recorder == nil {
		return
	}

	recorder.RecordDuration(fmt.Sprintf("%s[%s]", metricName, path), duration)
	if duration > 100*time.Millisecond {
		log.Infof("[File Performance] %s: %s, time: %v", logLabel, path, duration)
	}
}

const irSaveHeartbeatInterval = 5 * time.Second

// irSaveProgressCallback returns a progress callback for IR persistence that
// maps the running saved-instruction count onto a [processMin, processMax]
// range), and emits a heartbeat every irSaveHeartbeatInterval while work advances.
func irSaveProgressCallback(prog *ssa.Program, total int, baseSaved int, processMin, processMax float64, setProcess func(float64)) func(int) {
	var mu sync.Mutex
	var index int
	prevP := processMin
	if total > 0 && baseSaved > 0 {
		prevP = processMin + (float64(baseSaved)/float64(total))*(processMax-processMin)
	}
	lastHB := time.Now()
	lastIdxAtHB := 0
	return func(size int) {
		mu.Lock()
		defer mu.Unlock()
		index += size
		effective := baseSaved + index
		var p float64
		if total > 0 {
			p = processMin + (float64(effective)/float64(total))*(processMax-processMin)
		} else {
			p = processMax
		}
		if setProcess != nil {
			setProcess(p)
		}
		if total > 0 && (p-prevP) > 0.0001 {
			prog.ProcessInfof("[SSA/persist] Saving instructions: %d / %d", effective, total)
			prevP = p
		}
		now := time.Now()
		if total > 0 && index > lastIdxAtHB && now.Sub(lastHB) >= irSaveHeartbeatInterval {
			elapsed := now.Sub(lastHB).Seconds()
			if elapsed <= 0 {
				elapsed = 1e-9
			}
			rate := float64(index-lastIdxAtHB) / elapsed
			prog.ProcessInfof("[SSA/persist] IR save heartbeat: %d / %d (~%.0f inst/s over %.0fs)", effective, total, rate, elapsed)
			lastHB = now
			lastIdxAtHB = index
		}
	}
}
