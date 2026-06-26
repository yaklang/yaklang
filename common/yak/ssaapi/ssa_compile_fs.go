package ssaapi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
)

// heapLogEnabled gates retained-heap phase logging. Set YAK_SSA_HEAP_LOG=1 to print
// GC'd HeapInuse after each compile phase. Set YAK_SSA_HEAP_PROFILE_DIR=<dir> to write
// a heap profile (pprof) after each phase (GC first).
var heapLogEnabled = envFlagEnabled("YAK_SSA_HEAP_LOG")

const compileUnitWriterCacheEnv = "YAK_SSA_COMPILE_UNIT_WRITER_CACHE"
const compileUnitHoldSCCIREnv = "YAK_SSA_COMPILE_UNIT_HOLD_SCC_IR"
const compileUnitBatchMinFilesEnv = "YAK_SSA_COMPILE_UNIT_BATCH_MIN_FILES"
const compileUnitBatchMinBytesEnv = "YAK_SSA_COMPILE_UNIT_BATCH_MIN_BYTES"
const compileUnitBatchMaxFilesEnv = "YAK_SSA_COMPILE_UNIT_BATCH_MAX_FILES"
const ssaCompileAdaptiveGCEnv = "YAK_SSA_COMPILE_ADAPTIVE_GC"
const ssaCompileGOGCEnv = "YAK_SSA_COMPILE_GOGC"
const ssaCompileMemLimitEnv = "YAK_SSA_COMPILE_MEM_LIMIT"

const (
	// Compile-unit split thresholds: ULTRA-SMALL batches
	// Since we can't fully clear memory between batches (Instruction objects
	// hold references to Function/Block/Program), we minimize each batch's
	// footprint so even with accumulation we stay under OOM threshold.
	defaultCompileUnitBatchMinFiles = 32
	defaultCompileUnitBatchMinBytes = 256 * 1024
	defaultCompileUnitBatchMaxFiles = 100
	defaultSSACompileGOGC           = 1000
	defaultSSACompileMemLimit       = 680 * 1024 * 1024
	// Keep this in sync with the SSA IR cache resident fast-path threshold.
	// Below this size, a single compile-unit batch is cheaper in resident mode
	// than forcing the async writer cache.
	compileUnitResidentFastPathMaxBytes = 2 * 1024 * 1024
)

var ssaCompileGCMu sync.Mutex

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
	runtime.GC()
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

func startSSACompileCPUProfile() func() {
	target := strings.TrimSpace(os.Getenv("YAK_SSA_CPU_PROFILE"))
	if target == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "[ssa.cpu] profile mkdir failed %s: %v\n", filepath.Dir(target), err)
		return nil
	}
	f, err := os.Create(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ssa.cpu] profile create failed %s: %v\n", target, err)
		return nil
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		fmt.Fprintf(os.Stderr, "[ssa.cpu] profile start failed %s: %v\n", target, err)
		_ = f.Close()
		return nil
	}
	fmt.Fprintf(os.Stderr, "[ssa.cpu] profile started %s\n", target)
	return func() {
		pprof.StopCPUProfile()
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "[ssa.cpu] profile close failed %s: %v\n", target, err)
			return
		}
		fmt.Fprintf(os.Stderr, "[ssa.cpu] profile saved %s\n", target)
	}
}

func startSSACompileAdaptiveGC(logf func(string, ...any)) func() {
	if raw := strings.TrimSpace(os.Getenv(ssaCompileAdaptiveGCEnv)); raw != "" && !envFlagEnabled(ssaCompileAdaptiveGCEnv) {
		return nil
	}

	gcPercent, setGC := ssaCompileGCPercent()
	memLimit, setMemLimit := ssaCompileMemoryLimit()
	if !setGC && !setMemLimit {
		return nil
	}

	ssaCompileGCMu.Lock()
	var oldGC int
	var oldMemLimit int64
	if setGC {
		oldGC = debug.SetGCPercent(gcPercent)
	}
	if setMemLimit {
		oldMemLimit = debug.SetMemoryLimit(memLimit)
	}
	if logf != nil {
		logf("ssa compile adaptive GC enabled gogc=%s mem_limit=%s", gcPolicyValue(setGC, gcPercent), gcPolicyBytesValue(setMemLimit, memLimit))
	}
	return func() {
		if setMemLimit {
			debug.SetMemoryLimit(oldMemLimit)
		}
		if setGC {
			debug.SetGCPercent(oldGC)
		}
		ssaCompileGCMu.Unlock()
	}
}

func ssaCompileGCPercent() (int, bool) {
	if raw := strings.TrimSpace(os.Getenv(ssaCompileGOGCEnv)); raw != "" {
		switch strings.ToLower(raw) {
		case "0", "false", "no", "off", "disable", "disabled":
			return 0, false
		default:
			if v, err := strconv.Atoi(raw); err == nil && v >= 0 {
				return v, true
			}
		}
	}
	if strings.TrimSpace(os.Getenv("GOGC")) != "" {
		return 0, false
	}
	return defaultSSACompileGOGC, true
}

func ssaCompileMemoryLimit() (int64, bool) {
	if raw := strings.TrimSpace(os.Getenv(ssaCompileMemLimitEnv)); raw != "" {
		switch strings.ToLower(raw) {
		case "0", "false", "no", "off", "disable", "disabled":
			return 0, false
		default:
			if v, err := utils.ToBytes(raw); err == nil && v > 0 {
				return int64(v), true
			}
			if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v > 0 {
				return v, true
			}
		}
	}
	if strings.TrimSpace(os.Getenv("GOMEMLIMIT")) != "" {
		return 0, false
	}
	return defaultSSACompileMemLimit, true
}

func gcPolicyValue(enabled bool, value int) string {
	if !enabled {
		return "unchanged"
	}
	return strconv.Itoa(value)
}

func gcPolicyBytesValue(enabled bool, value int64) string {
	if !enabled {
		return "unchanged"
	}
	return utils.ByteSize(uint64(value))
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

type compileUnitExecutionBatch struct {
	startSCC int
	endSCC   int
	units    []*CompileUnit
	unitKeys []string
	files    int
	bytes    int64
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

func compileUnitBatchThresholds() (int, int64) {
	minFiles := defaultCompileUnitBatchMinFiles
	if raw := strings.TrimSpace(os.Getenv(compileUnitBatchMinFilesEnv)); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			minFiles = v
		}
	}
	if minFiles <= 0 {
		minFiles = 1
	}

	minBytes := int64(defaultCompileUnitBatchMinBytes)
	if raw := strings.TrimSpace(os.Getenv(compileUnitBatchMinBytesEnv)); raw != "" {
		switch strings.ToLower(raw) {
		case "0", "false", "no", "off", "disable", "disabled":
			minBytes = 0
		default:
			if v, err := utils.ToBytes(raw); err == nil {
				minBytes = int64(v)
			} else if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
				minBytes = v
			}
		}
	}
	if minBytes < 0 {
		minBytes = 0
	}
	return minFiles, minBytes
}

func compileUnitBatchMaxFiles() int {
	maxFiles := defaultCompileUnitBatchMaxFiles
	if raw := strings.TrimSpace(os.Getenv(compileUnitBatchMaxFilesEnv)); raw != "" {
		switch strings.ToLower(raw) {
		case "0", "false", "no", "off", "disable", "disabled":
			return 0
		default:
			if v, err := strconv.Atoi(raw); err == nil && v > 0 {
				maxFiles = v
			}
		}
	}
	return maxFiles
}

func buildCompileUnitExecutionBatches(order [][]*CompileUnit, minFiles int, minBytes int64) []compileUnitExecutionBatch {
	if len(order) == 0 {
		return nil
	}
	if minFiles <= 0 {
		minFiles = 1
	}
	if minBytes < 0 {
		minBytes = 0
	}
	maxFiles := compileUnitBatchMaxFiles()

	// Calculate total project size
	totalFiles := 0
	totalBytes := int64(0)
	for _, scc := range order {
		for _, unit := range scc {
			if unit != nil {
				totalFiles += len(unit.Files)
				totalBytes += unit.Bytes
			}
		}
	}

	// Estimate desired batch count based on both dimensions
	var estimatedBatchCount int
	if minFiles > 0 && totalFiles > minFiles {
		estimatedBatchCount = max(estimatedBatchCount, (totalFiles+minFiles-1)/minFiles)
	}
	if minBytes > 0 && totalBytes > minBytes {
		estimatedBatchCount = max(estimatedBatchCount, int((totalBytes+minBytes-1)/minBytes))
	}
	if maxFiles > 0 && totalFiles > maxFiles {
		estimatedBatchCount = max(estimatedBatchCount, (totalFiles+maxFiles-1)/maxFiles)
	}
	if estimatedBatchCount <= 1 {
		// Single batch - take everything
		return []compileUnitExecutionBatch{buildSingleBatch(order)}
	}

	// Adaptive target: aim for balanced batches
	targetFilesPerBatch := (totalFiles + estimatedBatchCount - 1) / estimatedBatchCount
	targetBytesPerBatch := (totalBytes + int64(estimatedBatchCount) - 1) / int64(estimatedBatchCount)

	// Use 80% of target as the threshold to allow some headroom for the last batch
	softMinFiles := int(float64(targetFilesPerBatch) * 0.8)
	softMinBytes := int64(float64(targetBytesPerBatch) * 0.8)
	if softMinFiles < 1 {
		softMinFiles = 1
	}

	batches := make([]compileUnitExecutionBatch, 0, estimatedBatchCount)
	current := compileUnitExecutionBatch{startSCC: -1, endSCC: -1}
	flush := func() {
		if len(current.units) == 0 {
			current = compileUnitExecutionBatch{startSCC: -1, endSCC: -1}
			return
		}
		batches = append(batches, current)
		current = compileUnitExecutionBatch{startSCC: -1, endSCC: -1}
	}

	for sccIndex, scc := range order {
		if current.startSCC < 0 {
			current.startSCC = sccIndex
		}
		current.endSCC = sccIndex
		for _, unit := range scc {
			if unit == nil {
				continue
			}
			// Check max files limit before adding unit
			if maxFiles > 0 && current.files+len(unit.Files) > maxFiles && len(current.units) > 0 {
				flush()
				current.startSCC = sccIndex
				current.endSCC = sccIndex
			}
			current.units = append(current.units, unit)
			current.unitKeys = append(current.unitKeys, unit.Key)
			current.files += len(unit.Files)
			current.bytes += unit.Bytes
		}

		// Check if we should flush this batch
		remainingSCCs := len(order) - sccIndex - 1
		remainingBatches := estimatedBatchCount - len(batches) - 1

		shouldFlush := false
		if remainingSCCs == 0 {
			// Last SCC - always flush to close the final batch
			shouldFlush = true
		} else if remainingBatches <= 0 {
			// No more batches planned - continue accumulating
			shouldFlush = false
		} else if compileUnitBatchReadyAdaptive(current, softMinFiles, softMinBytes) {
			// Reached soft threshold - flush
			shouldFlush = true
		}

		if shouldFlush {
			flush()
		}
	}
	flush()
	return batches
}

func buildSingleBatch(order [][]*CompileUnit) compileUnitExecutionBatch {
	batch := compileUnitExecutionBatch{startSCC: 0, endSCC: len(order) - 1}
	for _, scc := range order {
		for _, unit := range scc {
			if unit == nil {
				continue
			}
			batch.units = append(batch.units, unit)
			batch.unitKeys = append(batch.unitKeys, unit.Key)
			batch.files += len(unit.Files)
			batch.bytes += unit.Bytes
		}
	}
	return batch
}

func compileUnitBatchReadyAdaptive(batch compileUnitExecutionBatch, softMinFiles int, softMinBytes int64) bool {
	if len(batch.units) == 0 {
		return false
	}
	// Both dimensions must reach threshold (AND logic for better balance)
	filesReady := softMinFiles <= 1 || batch.files >= softMinFiles
	bytesReady := softMinBytes <= 0 || batch.bytes >= softMinBytes
	return filesReady && bytesReady
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func compileUnitBatchReady(batch compileUnitExecutionBatch, minFiles int, minBytes int64) bool {
	if len(batch.units) == 0 {
		return false
	}
	if minFiles <= 1 && minBytes <= 0 {
		return true
	}
	// Use OR logic for backward compatibility (deprecated path)
	if minFiles > 0 && batch.files >= minFiles {
		return true
	}
	if minBytes > 0 && batch.bytes >= minBytes {
		return true
	}
	return false
}

func compileUnitWriterCacheEnabled(requested bool, batches []compileUnitExecutionBatch, projectBytes int64) bool {
	if !requested {
		return false
	}
	if len(batches) > 1 {
		return true
	}
	return projectBytes > compileUnitResidentFastPathMaxBytes
}

type SaveFolder struct {
	name string
	path []string
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

func buildFileContent(
	prog *ssa.Program,
	builder *ssa.FunctionBuilder,
	fileContent *ssareducer.FileContent,
	ast ssa.FrontAST,
	enableFilePerfLog bool,
	filePerfRecorder *diagnostics.Recorder,
) {
	path := fileContent.Path
	fileBuildStart := time.Now()
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("parse [%s] error %v  ", path, r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
		if enableFilePerfLog {
			recordFilePerformance(filePerfRecorder, "Build", "Build", path, time.Since(fileBuildStart))
		}
	}()

	if err := prog.Build(ast, fileContent.Editor, builder); err != nil {
		log.Errorf("parse %#v failed: %v", path, err)
	} else {
		// Drop duplicate *MemEditor ref from the slice; IR still holds editors via memedit.Range where needed.
		fileContent.Editor = nil
	}
}

func collectCompileTargets(
	prog *ssa.Program,
	fileContents []*ssareducer.FileContent,
	handlerFileSet map[string]struct{},
) []*ssareducer.FileContent {
	targets := make([]*ssareducer.FileContent, 0, len(handlerFileSet))
	for _, fileContent := range fileContents {
		if fileContent == nil {
			continue
		}
		if fileContent.Status == ssareducer.FileStatusFsError {
			log.Errorf("skip file: %s with fs error: %v", fileContent.Path, fileContent.Err)
			prog.ProcessInfof("skip  file: %s with fs error: %v", fileContent.Path, fileContent.Err)
			continue
		}

		if _, needsCompile := handlerFileSet[fileContent.Path]; !needsCompile {
			if fileContent.Editor != nil {
				prog.PushEditor(fileContent.Editor)
				prog.PopEditor(true)
			}
			continue
		}

		if fileContent.Editor == nil {
			log.Errorf("skip file: %s due to nil editor", fileContent.Path)
			prog.ProcessInfof("skip  file: %s due to nil editor", fileContent.Path)
			continue
		}
		if prog.ShouldVisit(fileContent.Editor.GetUrl()) {
			log.Debugf("parse file %s done skip in main build", fileContent.Path)
			continue
		}

		targets = append(targets, fileContent)
	}
	return targets
}

func (c *Config) parseProjectWithFSUnits(
	filesystem filesys_interface.FileSystem,
	processCallback func(float64, string, ...any),
) (*Program, error) {
	var calculateTime, preHandlerTime, parseTime, finishTime, saveTime time.Duration
	overallStart := time.Now()
	defer func() {
		log.Debugf("calculate time: %v", calculateTime)
		log.Debugf("pre-handler time: %v", preHandlerTime)
		log.Debugf("parse time (unit build): %v", parseTime)
		log.Debugf("finish time (f4 Finish+metadata): %v", finishTime)
		log.Debugf("save time: %v", saveTime)
		log.Debugf("ssa.compile.phase_segments: %v", calculateTime+preHandlerTime+parseTime+finishTime+saveTime)
		log.Debugf("ssa.compile.wall: %v", time.Since(overallStart))
	}()
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("parse project error: %s", r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	var wg sync.WaitGroup
	compilePhase := "f0_scan"
	programName := c.GetProgramName()
	programPath := c.programPath
	process := 0.0
	start := time.Now()

	log.Debugf("ssa.compile.phase enter %s", compilePhase)
	processCallback(0.0, fmt.Sprintf("[%s] parse project in fs: %v, path: %v", compilePhase, filesystem, c.GetCodeSource().ToJSONString()))
	processCallback(0.0, fmt.Sprintf("[%s] calculate total size of project", compilePhase))

	folder2Save := make([][]string, 0)
	if programName != "" {
		folder2Save = append(folder2Save, []string{"/", programName})
	}

	sourceFilesystem := filesystem
	filesystem = c.swapLanguageFs(filesystem)
	scanResult, err := ScanProjectFiles(ScanConfig{
		ProgramName:     programName,
		ProgramPath:     programPath,
		FileSystem:      filesystem,
		ExcludeFunc:     c.excludeFile,
		CheckLanguage:   c.checkLanguage,
		CheckPreHandler: c.checkLanguagePreHandler,
		Context:         c.ctx,
	})
	if err != nil {
		return nil, err
	}
	folder2Save = append(folder2Save, scanResult.Folders...)
	handlerTotal := scanResult.HandlerTotal
	handlerFilesMap := scanResult.HandlerFilesMap
	handlerFiles := scanResult.HandlerFiles
	handlerFileSet := make(map[string]struct{}, len(handlerFiles))
	for _, handlerFile := range handlerFiles {
		handlerFileSet[handlerFile] = struct{}{}
	}
	preHandlerTotal := scanResult.PreHandlerTotal
	preHandlerFiles := scanResult.PreHandlerFiles
	if preHandlerTotal < handlerTotal {
		preHandlerTotal = handlerTotal
		preHandlerFiles = handlerFiles
	}
	calculateTime = time.Since(start)
	c.Config.SetCompileProjectBytes(scanResult.HandlerBytes)

	plan := buildCompileUnitPlan(c.GetLanguage(), filesystem, preHandlerFiles)
	if len(plan.Order) == 0 {
		unit := &CompileUnit{Key: "unit:all", Path: ".", Files: append([]string(nil), preHandlerFiles...), Language: c.GetLanguage()}
		plan = &UnitPlan{Units: map[string]*CompileUnit{unit.Key: unit}, Order: [][]*CompileUnit{{unit}}}
	}
	batchMinFiles, batchMinBytes := compileUnitBatchThresholds()
	batches := buildCompileUnitExecutionBatches(plan.Order, batchMinFiles, batchMinBytes)
	writerCacheRequested := envFlagEnabled(compileUnitWriterCacheEnv)
	writerCacheEnabled := compileUnitWriterCacheEnabled(writerCacheRequested, batches, scanResult.HandlerBytes)
	c.Config.SetCompileUnitSplit(writerCacheEnabled)
	if writerCacheRequested && !writerCacheEnabled && len(batches) <= 1 {
		payload := buildCompileUnitPlanLog(
			programName,
			fmt.Sprintf("%v", c.GetLanguage()),
			plan,
			batches,
			batchMinFiles,
			batchMinBytes,
			"legacy-fallback",
			"resident-fast-path",
			writerCacheRequested,
			writerCacheEnabled,
		)
		if target, err := writeCompileUnitPlanLogFile(os.Getenv("YAK_SSA_COMPILE_UNIT_LOG_DIR"), payload); err != nil {
			processCallback(0, "[f0_scan] compile unit single-batch fallback plan write failed: %v", err)
		} else if target != "" {
			processCallback(0, "[f0_scan] compile unit single-batch fallback wrote plan: %s", target)
		}
		processCallback(0, "[f0_scan] compile unit graph built units=%d edges=%d scc=%d batches=%d; fallback to legacy compile because writer cache is not needed",
			len(plan.Units), len(plan.Edges), len(plan.Order), len(batches))
		return c.parseProjectWithFSLegacy(sourceFilesystem, processCallback)
	}

	prog, builder, err := c.init(filesystem, handlerTotal)
	if err != nil {
		return nil, err
	}
	if rec := c.DiagnosticsRecorder(); rec != nil {
		prog.SetDiagnosticsRecorder(rec)
	}
	prog.ProcessInfof = func(s string, v ...any) {
		msg := s
		if len(v) > 0 {
			msg = fmt.Sprintf(s, v...)
		}
		if compilePhase != "" {
			msg = fmt.Sprintf("[%s] %s", compilePhase, msg)
		}
		processCallback(process, msg)
	}
	if stopAdaptiveGC := startSSACompileAdaptiveGC(prog.ProcessInfof); stopAdaptiveGC != nil {
		defer stopAdaptiveGC()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, folder := range folder2Save {
			prog.SaveFolder(folder)
		}
	}()

	if c.isStop() {
		return nil, ErrContextCancel
	}
	if (handlerTotal + preHandlerTotal) == 0 {
		return nil, ErrNoFoundCompiledFile
	}
	prog.ProcessInfof("calculate total size of project finish preHandler(len:%d) build(len:%d)", preHandlerTotal, handlerTotal)
	defer c.LanguageBuilder.Clearup()

	prog.ProcessInfof("compile unit graph built units=%d edges=%d scc=%d", len(plan.Units), len(plan.Edges), len(plan.Order))
	holdSCCIR := envFlagEnabled(compileUnitHoldSCCIREnv)
	spillMode := "auto"
	if holdSCCIR {
		spillMode = "held"
	}
	cacheMode := "none"
	if prog.Cache != nil {
		cacheMode = prog.Cache.InstructionCacheMode()
	}
	prog.ProcessInfof("compile unit execution batches built batches=%d min_files=%d min_bytes=%d writer_requested=%v writer_enabled=%v cache=%s",
		len(batches), batchMinFiles, batchMinBytes, writerCacheRequested, writerCacheEnabled, cacheMode)
	logCompileUnitPlan(prog, fmt.Sprintf("%v", c.GetLanguage()), plan, batches, batchMinFiles, batchMinBytes, spillMode, cacheMode, writerCacheRequested, writerCacheEnabled)

	var astErr error
	astParseErrLogged := 0
	astParseErrSuppressed := false
	const maxAstParseErrLogs = 20
	enableFilePerfLog := c.Config != nil && c.Config.GetCompileFilePerformanceLog()
	if enableFilePerfLog && c.filePerformanceRecorder == nil {
		c.filePerformanceRecorder = diagnostics.NewRecorder()
	}
	filePerfRecorder := c.filePerformanceRecorder
	preHandlerBuildsFiles := languagePreHandlerBuildsFiles(c.GetLanguage())
	preHandlerNum := 0
	preHandlerProcess := func() {
		preHandlerNum++
		process = (float64(preHandlerNum) / float64(preHandlerTotal)) * 0.4
		if process > 0.4 {
			process = 0.4
		}
	}

	compilePhase = "f1_units"
	log.Debugf("ssa.compile.phase enter %s", compilePhase)
	unitStart := time.Now()
	prog.SetPreHandler(true)
	prog.ProcessInfof("unit compile start scc=%d batches=%d", len(plan.Order), len(batches))
	for batchIndex, batch := range batches {
		if c.isStop() {
			return nil, ErrContextCancel
		}
		if holdSCCIR && prog.Cache != nil {
			prog.Cache.DisableInstructionSpill()
		}
		unitKeys := batch.unitKeys
		if compileUnitLogEnabled() {
			prog.ProcessInfof("compile unit batch(%d/%d) scc=%d-%d units=%d files=%d bytes=%d spill=%s keys=%s",
				batchIndex+1, len(batches), batch.startSCC+1, batch.endSCC+1, len(batch.units), batch.files, batch.bytes, spillMode, strings.Join(unitKeys, ","))
		} else {
			prog.ProcessInfof("compile unit batch(%d/%d) scc=%d-%d units=%d files=%d bytes=%d spill=%s",
				batchIndex+1, len(batches), batch.startSCC+1, batch.endSCC+1, len(batch.units), batch.files, batch.bytes, spillMode)
		}
		for _, unit := range batch.units {
			if unit == nil {
				continue
			}
			prog.BeginCompileUnit(unit.Key)
			unitCanceled := false
			ch := c.GetFileHandler(filesystem, unit.Files, handlerFilesMap)
			for fileContent := range ch {
				if fileContent == nil {
					continue
				}
				func(fileContent *ssareducer.FileContent) {
					defer fileContent.Release()
					fileASTStart := time.Now()
					if fileContent.Status == ssareducer.FileStatusFsError {
						log.Errorf("skip file: %s with fs error: %v", fileContent.Path, fileContent.Err)
						prog.ProcessInfof("skip  file: %s with fs error: %v", fileContent.Path, fileContent.Err)
						return
					}
					if fileContent.Status == ssareducer.FileParseASTError {
						if astParseErrLogged < maxAstParseErrLogs {
							log.Warnf("parse Ast file: %s error: %s", fileContent.Path, fileContent.Err)
							astParseErrLogged++
						} else if !astParseErrSuppressed {
							astParseErrSuppressed = true
							log.Warnf("too many AST parse errors; suppressing further per-file logs (limit=%d)", maxAstParseErrLogs)
						}
						astErr = utils.Errorf("parse Ast file: %s error: %s", fileContent.Path, fileContent.Err)
					}
					editor := prog.CreateEditor(fileContent.Content, fileContent.Path)
					fileContent.Editor = editor
					fileContent.Content = nil
					if fileContent.Err != nil {
						prog.ProcessInfof("file %s parse ast error: %v", fileContent.Path, fileContent.Err)
						astErr = utils.JoinErrors(astErr,
							utils.Errorf("pre-handler parse file %s error: %v", fileContent.Path, fileContent.Err),
						)
					}
					preHandlerProcess()
					if language := c.LanguageBuilder; language != nil {
						func() {
							defer func() {
								if r := recover(); r != nil {
									log.Errorf("pre-handler parse [%s] error %v  ", fileContent.Path, r)
									utils.PrintCurrentGoroutineRuntimeStack()
								}
							}()
							language.InitHandler(builder)
							err = language.PreHandlerProject(filesystem, fileContent.AST, builder, editor)
							if err != nil {
								log.Errorf("pre-handler parse [%s] error %v", fileContent.Path, err)
							}
						}()
					}
					if preHandlerBuildsFiles {
						ssa.ReleaseASTRoot(fileContent.AST)
					}
					if _, needBuild := handlerFilesMap[fileContent.Path]; needBuild {
						_, needsCompile := handlerFileSet[fileContent.Path]
						switch {
						case needsCompile && fileContent.AST != nil && !preHandlerBuildsFiles:
							ast := fileContent.AST
							path := fileContent.Path
							prog.RegisterFileBuild(path, editor, builder, func(fileBuilder *ssa.FunctionBuilder) {
								fileBuildStart := time.Now()
								defer func() {
									if enableFilePerfLog && filePerfRecorder != nil {
										fileBuildTime := time.Since(fileBuildStart)
										filePerfRecorder.RecordDuration(fmt.Sprintf("Build[%s]", path), fileBuildTime)
										if fileBuildTime > 100*time.Millisecond {
											log.Infof("[File Performance] Build: %s, time: %v", path, fileBuildTime)
										}
									}
								}()
								if err := c.LanguageBuilder.BuildFromAST(ast, fileBuilder); err != nil {
									log.Errorf("file build [%s] failed: %v", path, err)
								}
							})
						case !needsCompile && fileContent.Editor != nil:
							rootEditor := fileContent.Editor
							prog.RegisterDeferredBuild(ssa.DeferredBuildKindHelper, "extra-file:"+rootEditor.GetUrl(), func() {
								prog.PushEditor(rootEditor)
								prog.PopEditor(true)
							})
						}
					}
					fileContent.AST = nil
					if enableFilePerfLog {
						recordFilePerformance(filePerfRecorder, "AST", "AST parse", fileContent.Path, time.Since(fileASTStart))
					}
				}(fileContent)
				if c.isStop() {
					unitCanceled = true
					break
				}
			}
			prog.EndCompileUnit()
			if unitCanceled {
				return nil, ErrContextCancel
			}
		}
		if c.isStop() {
			return nil, ErrContextCancel
		}
		if language := c.LanguageBuilder; language != nil {
			language.AfterPreHandlerProject(builder)
			language.Clearup()
		}
		prog.SetPreHandler(false)
		if holdSCCIR && prog.Cache != nil {
			prog.Cache.EnableInstructionSpill()
		}
		compilePhase = "f3_unit_build"
		unitBuildStart := time.Now()
		if !prog.RunDeferredBuildsForUnits(unitKeys, func(index int, total int) bool {
			return !c.isStop()
		}) {
			return nil, ErrContextCancel
		}
		if c.isStop() {
			return nil, ErrContextCancel
		}
		prog.LazyBuildForUnits(unitKeys)
		if c.isStop() {
			return nil, ErrContextCancel
		}
		// Capture pre-flush metrics for comparison
		preFlushIR := 0
		preFlushPersisted := 0
		preFlushFuncs := 0
		preFlushBlueprints := 0
		if prog.Cache != nil {
			preFlushIR = prog.Cache.CountInstruction()
			preFlushPersisted = prog.Cache.InstructionPersistedCount()
		}
		if prog.Funcs != nil {
			preFlushFuncs = prog.Funcs.Len()
		}
		if prog.Blueprint != nil {
			preFlushBlueprints = prog.Blueprint.Len()
		}
		preFlushHeap := captureHeapMetrics()

		if prog.Cache != nil && writerCacheEnabled {
			prog.Cache.FlushCompileUnit(strings.Join(unitKeys, ","))

			// Check memory pressure after flush
			prog.CheckMemoryPressure(batchIndex+1, len(batches))

			// Measure post-flush to verify release
			postFlushIR := prog.Cache.CountInstruction()
			postFlushPersisted := prog.Cache.InstructionPersistedCount()
			postFlushFuncs := 0
			postFlushBlueprints := 0
			if prog.Funcs != nil {
				postFlushFuncs = prog.Funcs.Len()
			}
			if prog.Blueprint != nil {
				postFlushBlueprints = prog.Blueprint.Len()
			}
			postFlushHeap := captureHeapMetrics()
			releasedEditors := prog.Cache.CountReleasedEditors()

			prog.ProcessInfof(
				"compile unit batch(%d/%d) cache flushed scc=%d-%d units=%s mode=%s resident_ir=%d→%d(Δ%+d) persisted_ir=%d→%d(Δ%+d) heap_mb=%.1f→%.1f(Δ%+.1f) funcs=%d→%d(Δ%+d) blueprints=%d→%d(Δ%+d) editors_released=%d upstreams=%d cost=%v",
				batchIndex+1,
				len(batches),
				batch.startSCC+1,
				batch.endSCC+1,
				strings.Join(unitKeys, ","),
				prog.Cache.InstructionCacheMode(),
				preFlushIR, postFlushIR, postFlushIR-preFlushIR,
				preFlushPersisted, postFlushPersisted, postFlushPersisted-preFlushPersisted,
				preFlushHeap, postFlushHeap, postFlushHeap-preFlushHeap,
				preFlushFuncs, postFlushFuncs, postFlushFuncs-preFlushFuncs,
				preFlushBlueprints, postFlushBlueprints, postFlushBlueprints-preFlushBlueprints,
				releasedEditors,
				prog.UpStream.Len(),
				time.Since(unitBuildStart),
			)
	} else if prog.Cache != nil {
		// Writer cache not enabled: do NOT aggressively clear stores or
		// program-level structures here. Sources/instructions/editors are
		// still needed for index building, Ref() lookups, GetEditorByHash,
		// and Show().ForEachAllFile after compile; they persist to DB via
		// the normal TTL/eviction path and the final SaveToDatabase flush.
		// Aggressive clearing (incl. AggressiveClearMemory which wipes
		// editorStack/UpStream/deferredBuilds) drops un-persisted state and
		// breaks downstream FromDatabase/Ref/Show.
		preFlushIR := prog.Cache.CountInstruction()
		preFlushHeap := captureHeapMetrics()

		clearedItems := 0
		releasedFuncs := 0

		postFlushHeap := captureHeapMetrics()

		prog.ProcessInfof(
			"compile unit batch(%d/%d) non-writer cleared scc=%d-%d units=%s mode=%s writer_enabled=%v cleared=%d released_funcs=%d resident_ir=%d funcs=%d blueprints=%d heap_mb=%.1f→%.1f(Δ%+.1f) upstreams=%d cost=%v",
			batchIndex+1,
			len(batches),
			batch.startSCC+1,
			batch.endSCC+1,
			strings.Join(unitKeys, ","),
			prog.Cache.InstructionCacheMode(),
			writerCacheEnabled,
			clearedItems,
			releasedFuncs,
			preFlushIR,
			prog.Funcs.Len(),
			prog.Blueprint.Len(),
			preFlushHeap,
			postFlushHeap,
			postFlushHeap-preFlushHeap,
			prog.UpStream.Len(),
			time.Since(unitBuildStart),
		)
	}
		if compileUnitLogEnabled() {
			prog.ProcessInfof("compile unit batch(%d/%d) build+flush finished units=%s cost=%v", batchIndex+1, len(batches), strings.Join(unitKeys, ","), time.Since(unitBuildStart))
		}
		parseTime += time.Since(unitBuildStart)
		logPhaseHeap(fmt.Sprintf("unit_batch_%03d", batchIndex+1))
		runtime.GC()
		prog.SetPreHandler(true)
		compilePhase = "f1_units"
	}
	preHandlerTime = time.Since(unitStart) - parseTime
	if astErr != nil && c.GetCompileStrictMode() {
		return nil, utils.Errorf("pre-handler parse project error: %v", astErr)
	}

	compilePhase = "f4_finish"
	log.Debugf("ssa.compile.phase enter %s", compilePhase)
	finishStart := time.Now()
	process = 0.88
	prog.SetPreHandler(false)
	if !prog.RunDeferredBuildsWithCallback(func(index int, total int) bool {
		return !c.isStop()
	}) {
		return nil, ErrContextCancel
	}
	if c.isStop() {
		return nil, ErrContextCancel
	}
	prog.Finish()
	if baseProgramName := c.GetBaseProgramName(); baseProgramName != "" {
		prog.BaseProgramName = baseProgramName
	}
	if len(c.fileHashMap) > 0 {
		prog.FileHashMap = c.fileHashMap
	}
	if c.GetEnableIncrementalCompile() && prog.FileHashMap == nil {
		prog.FileHashMap = make(map[string]int)
	}
	if prog.DatabaseKind != ssa.ProgramCacheMemory {
		prog.ProcessInfof("[SSA/persist] program %s saving program metadata (ir_program)", prog.Name)
		metaStart := time.Now()
		prog.UpdateToDatabaseWithWG(&wg)
		since := time.Since(metaStart)
		log.Infof("program %s save to database cost: %s", prog.Name, since)
		prog.ProcessInfof("[SSA/persist] program %s program metadata saved, cost %v", prog.Name, since)
	}
	finishTime = time.Since(finishStart)
	logPhaseHeap("f4_finish")

	compilePhase = "f5_save_db"
	log.Debugf("ssa.compile.phase enter %s", compilePhase)
	saveStart := time.Now()
	remaining := prog.Cache.CountInstruction()
	persisted := prog.Cache.InstructionPersistedCount()
	total := remaining + persisted
	process = 0.90
	if prog.DatabaseKind != ssa.ProgramCacheMemory {
		prog.ProcessInfof("[SSA/persist] program %s flushing IR cache (remaining=%d persisted=%d total=%d) to database",
			prog.Name, remaining, persisted, total)
	} else {
		prog.ProcessInfof("[SSA/persist] program %s finishing cache instruction(len:%d) (memory only, not saved)", prog.Name, remaining)
	}
	if err := prog.Cache.SaveToDatabase(irSaveProgressCallback(prog, total, persisted, 0.90, 1.0, func(p float64) {
		process = p
	})); err != nil {
		return nil, utils.Errorf("persist IR to database failed: %w", err)
	}
	saveTime = time.Since(saveStart)
	if prog.DatabaseKind != ssa.ProgramCacheMemory {
		prog.ProcessInfof("[SSA/persist] program %s IR cache flush finished, cost %v", prog.Name, saveTime)
	}
	logPhaseHeap("f5_save_db")

	compilePhase = "f6_wait"
	wg.Wait()
	logPhaseHeap("f6_wait")

	if enableFilePerfLog && filePerfRecorder != nil {
		snapshots := filePerfRecorder.Snapshot()
		if len(snapshots) > 0 {
			table := diagnostics.FormatPerformanceTable("File Compilation Performance Summary", snapshots)
			fmt.Println(table)
		} else {
			fmt.Println("File Performance: no data recorded")
		}
	}
	p := NewProgram(prog, c)
	SaveConfig(c, p)
	SetProgramCache(p)
	return p, nil
}

// parseProjectWithFS compiles a whole project from a filesystem.
//
// Pipeline: parallel read/ParseAST is inside f1 only (FilesHandler -> channel). One goroutine
// consumes the channel (PreHandlerProject) and fills fileContents. f3 walks targets and Build
// sequentially — it does not consume the AST channel. Observability: log=info prints
// [ssa.compile.summary]; log=debug prints ssa.compile.phase enter f1_pre_handler / f3_main_build / …
// and ProcessInfof lines are prefixed with the current phase tag.
func (c *Config) parseProjectWithFS(
	filesystem filesys_interface.FileSystem,
	processCallback func(float64, string, ...any),
) (*Program, error) {
	if stopCPUProfile := startSSACompileCPUProfile(); stopCPUProfile != nil {
		defer stopCPUProfile()
	}
	if os.Getenv("YAK_SSA_LEGACY_PROJECT_COMPILE") != "" {
		return c.parseProjectWithFSLegacy(filesystem, processCallback)
	}
	return c.parseProjectWithFSUnits(filesystem, processCallback)
}

func (c *Config) parseProjectWithFSLegacy(
	filesystem filesys_interface.FileSystem,
	processCallback func(float64, string, ...any),
) (*Program, error) {
	var calculateTime, preHandlerTime, parseTime, finishTime, saveTime time.Duration
	overallStart := time.Now()
	defer func() {
		log.Debugf("calculate time: %v", calculateTime)
		log.Debugf("pre-handler time: %v", preHandlerTime)
		log.Debugf("parse time (main build f3): %v", parseTime)
		log.Debugf("finish time (f4 Finish+metadata): %v", finishTime)
		log.Debugf("save time: %v", saveTime)
		log.Debugf("ssa.compile.phase_segments: %v", calculateTime+preHandlerTime+parseTime+finishTime+saveTime)
		log.Debugf("ssa.compile.wall: %v", time.Since(overallStart))
	}()

	defer func() {
		if r := recover(); r != nil {
			//err = utils.Errorf("parse [%s] error %v  ", path, r)
			log.Errorf("parse project error: %s", r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	wg := sync.WaitGroup{}

	// compilePhase labels UI / callback messages and debug logs so operators can align htop with f1/f3/f5.
	compilePhase := "f0_scan"

	programName := c.GetProgramName()
	programPath := c.programPath
	preHandlerTotal := 0
	handlerTotal := 0
	preHandlerFiles := make([]string, 0)
	handlerFilesMap := make(map[string]struct{})
	handlerFiles := make([]string, 0)
	handlerFileSet := make(map[string]struct{})
	start := time.Now()

	log.Debugf("ssa.compile.phase enter %s", compilePhase)
	processCallback(0.0, fmt.Sprintf("[%s] parse project in fs: %v, path: %v", compilePhase, filesystem, c.GetCodeSource().ToJSONString()))
	processCallback(0.0, fmt.Sprintf("[%s] calculate total size of project", compilePhase))

	folder2Save := make([][]string, 0)
	if programName != "" {
		folder2Save = append(folder2Save, []string{"/", programName})
	}

	filesystem = c.swapLanguageFs(filesystem)
	// get total size
	// scan project files
	scanResult, err := ScanProjectFiles(ScanConfig{
		ProgramName:     programName,
		ProgramPath:     programPath,
		FileSystem:      filesystem,
		ExcludeFunc:     c.excludeFile,
		CheckLanguage:   c.checkLanguage,
		CheckPreHandler: c.checkLanguagePreHandler,
		Context:         c.ctx,
	})
	if err != nil {
		return nil, err
	}

	folder2Save = append(folder2Save, scanResult.Folders...)
	handlerTotal = scanResult.HandlerTotal
	handlerFiles = scanResult.HandlerFiles
	handlerFileSet = scanResult.HandlerFileSet
	preHandlerTotal = scanResult.PreHandlerTotal
	preHandlerFiles = scanResult.PreHandlerFiles
	handlerFilesMap = scanResult.HandlerFilesMap
	calculateTime = time.Since(start)
	if err != nil {
		return nil, err
	}
	// Feed the total compile-input bytes into the adaptive IR cache policy.
	// This is runtime tuning input, not persistent project metadata.
	c.Config.SetCompileProjectBytes(scanResult.HandlerBytes)
	if restoreGC := c.applyLargeProjectGCPercent(); restoreGC != nil {
		defer restoreGC()
	}
	c.Config.SetCompileUnitSplit(false)

	prog, builder, err := c.init(filesystem, handlerTotal)
	if err != nil {
		return nil, err
	}
	if rec := c.DiagnosticsRecorder(); rec != nil {
		prog.SetDiagnosticsRecorder(rec)
	}

	wg.Add(1)
	go func() {
		wg.Done()
		for _, folder := range folder2Save {
			_ = folder
			prog.SaveFolder(folder)
		}
	}()

	process := 0.0
	prog.ProcessInfof = func(s string, v ...any) {
		msg := s
		if len(v) > 0 {
			msg = fmt.Sprintf(s, v...)
		}
		if compilePhase != "" {
			msg = fmt.Sprintf("[%s] %s", compilePhase, msg)
		}
		processCallback(process, msg)
	}
	if stopAdaptiveGC := startSSACompileAdaptiveGC(prog.ProcessInfof); stopAdaptiveGC != nil {
		defer stopAdaptiveGC()
	}

	if c.isStop() {
		return nil, ErrContextCancel
	}
	if (handlerTotal + preHandlerTotal) == 0 {
		return nil, ErrNoFoundCompiledFile
	}
	if preHandlerTotal < handlerTotal {
		preHandlerTotal = handlerTotal
		preHandlerFiles = handlerFiles
	}
	prog.ProcessInfof("calculate total size of project finish preHandler(len:%d) build(len:%d)", preHandlerTotal, handlerTotal)
	defer c.LanguageBuilder.Clearup()

	var AstErr error
	astParseErrLogged := 0
	astParseErrSuppressed := false
	const maxAstParseErrLogs = 20
	enableFilePerfLog := c.Config != nil && c.Config.GetCompileFilePerformanceLog()
	// 创建文件性能 recorder
	if enableFilePerfLog && c.filePerformanceRecorder == nil {
		c.filePerformanceRecorder = diagnostics.NewRecorder()
	}
	filePerfRecorder := c.filePerformanceRecorder
	// When pre-handler already emits file skeletons and schedules remaining file
	// work, the shared pipeline must not capture the whole file AST in another
	// closure.
	preHandlerBuildsFiles := c.LanguageBuilder != nil && c.LanguageBuilder.UsesDeferredFileBuild()
	// pre handler  0-40%
	f1 := func() error {
		if prog.Cache != nil {
			prog.Cache.DisableInstructionSpill()
		}
		preHandlerNum := 0
		preHandlerProcess := func() {
			preHandlerNum++
			process = 0 + (float64(preHandlerNum)/float64(preHandlerTotal))*0.4
			if process > 0.4 {
				process = 0.4
			}
		}
		prog.SetPreHandler(true)
		prog.ProcessInfof("pre-handler parse project in fs: %v, path: %v", filesystem, c.GetCodeSource().ToJSONString())
		start = time.Now()

		ch := c.GetFileHandler(
			filesystem, preHandlerFiles, handlerFilesMap,
		)
		for fileContent := range ch {
			if fileContent == nil {
				continue
			}
			func(fileContent *ssareducer.FileContent) {
				defer fileContent.Release()
				fileASTStart := time.Now()
				if fileContent.Status == ssareducer.FileStatusFsError {
					log.Errorf("skip file: %s with fs error: %v", fileContent.Path, fileContent.Err)
					prog.ProcessInfof("skip  file: %s with fs error: %v", fileContent.Path, fileContent.Err)
					return
				}

				if fileContent.Status == ssareducer.FileParseASTError {
					if astParseErrLogged < maxAstParseErrLogs {
						log.Warnf("parse Ast file: %s error: %s", fileContent.Path, fileContent.Err)
						astParseErrLogged++
					} else if !astParseErrSuppressed {
						astParseErrSuppressed = true
						log.Warnf("too many AST parse errors; suppressing further per-file logs (limit=%d)", maxAstParseErrLogs)
					}
					AstErr = utils.Errorf("parse Ast file: %s error: %s", fileContent.Path, fileContent.Err)
					// continue
				}

				editor := prog.CreateEditor(fileContent.Content, fileContent.Path)
				// editor := prog.CreateEditor([]byte{}, fileContent.Path)

				fileContent.Editor = editor
				fileContent.Content = nil

				if fileContent.Err != nil {
					prog.ProcessInfof("file %s parse ast error: %v", fileContent.Path, fileContent.Err)
					AstErr = utils.JoinErrors(AstErr,
						utils.Errorf("pre-handler parse file %s error: %v", fileContent.Path, fileContent.Err),
					)
				}

				preHandlerProcess() // notify the process
				// handler
				if language := c.LanguageBuilder; language != nil {
					func() {
						defer func() {
							if r := recover(); r != nil {
								log.Errorf("pre-handler parse [%s] error %v  ", fileContent.Path, r)
								utils.PrintCurrentGoroutineRuntimeStack()
							}
						}()
						language.InitHandler(builder)
						err = language.PreHandlerProject(filesystem, fileContent.AST, builder, editor)
						if err != nil {
							log.Errorf("pre-handler parse [%s] error %v", fileContent.Path, err)
						}
					}()
				}
				if preHandlerBuildsFiles {
					ssa.ReleaseASTRoot(fileContent.AST)
				}
				if _, needBuild := handlerFilesMap[fileContent.Path]; needBuild {
					_, needsCompile := handlerFileSet[fileContent.Path]
					switch {
					case needsCompile && fileContent.AST != nil && !preHandlerBuildsFiles:
						ast := fileContent.AST
						path := fileContent.Path
						prog.RegisterFileBuild(path, editor, builder, func(fileBuilder *ssa.FunctionBuilder) {
							fileBuildStart := time.Now()
							defer func() {
								if enableFilePerfLog && filePerfRecorder != nil {
									fileBuildTime := time.Since(fileBuildStart)
									filePerfRecorder.RecordDuration(fmt.Sprintf("Build[%s]", path), fileBuildTime)
									if fileBuildTime > 100*time.Millisecond {
										log.Infof("[File Performance] Build: %s, time: %v", path, fileBuildTime)
									}
								}
							}()
							if err := c.LanguageBuilder.BuildFromAST(ast, fileBuilder); err != nil {
								log.Errorf("file build [%s] failed: %v", path, err)
							}
						})
					case !needsCompile && fileContent.Editor != nil:
						rootEditor := fileContent.Editor
						prog.RegisterDeferredBuild(ssa.DeferredBuildKindHelper, "extra-file:"+rootEditor.GetUrl(), func() {
							prog.PushEditor(rootEditor)
							prog.PopEditor(true)
						})
					}
				}
				// Once skeleton + deferred tasks are registered (pass1), drop the file
				// AST root reference. For self-registering languages the body subtrees
				// are detached, so the rest of the parse tree becomes collectable here.
				fileContent.AST = nil
				if enableFilePerfLog {
					recordFilePerformance(filePerfRecorder, "AST", "AST parse", fileContent.Path, time.Since(fileASTStart))
				}
			}(fileContent)
		}
		preHandlerTime = time.Since(start)
		if AstErr != nil && c.GetCompileStrictMode() {
			return utils.Errorf("pre-handler parse project error: %v", AstErr)
		}
		if c.isStop() {
			return ErrContextCancel
		}
		return nil
	}

	f2 := func() error {
		if language := c.LanguageBuilder; language != nil {
			language.AfterPreHandlerProject(builder)
			// Release ANTLR caches early: main build doesn't require the pre-handler
			// parsing caches, and keeping them can cause huge heap + GC overhead.
			language.Clearup()
		}
		prog.ProcessInfof("pre-handler parse project finish")
		return nil
	}

	f3 := func() error {
		if prog.Cache != nil {
			prog.Cache.EnableInstructionSpill()
		}
		process = 0.4 // 40%
		prog.ProcessInfof("deferred build start")
		prog.SetPreHandler(false)
		start = time.Now()
		deferredBuildTotal := prog.DeferredBuildCount()
		completed := prog.RunDeferredBuildsWithCallback(func(index int, total int) bool {
			if total <= 0 {
				return !c.isStop()
			}
			// Reserve [0.88, 0.90) for program metadata (f4) and [0.90, 1.0] for IR flush (f5).
			process = 0.4 + (float64(index)/float64(total))*0.48
			if process > 0.88 {
				process = 0.88
			}
			prog.ProcessInfof("deferred build progress(%d/%d)", index, total)
			return !c.isStop()
		})
		if deferredBuildTotal == 0 {
			process = 0.88
		}
		if !completed {
			return ErrContextCancel
		}
		process = 0.88
		parseTime = time.Since(start)
		if c.isStop() {
			return ErrContextCancel
		}
		return nil
	}

	f4 := func() error {
		f4Start := time.Now()
		defer func() { finishTime = time.Since(f4Start) }()
		process = 0.88
		prog.Finish()
		// 在保存到数据库之前，设置增量编译信息（如果存在）
		if baseProgramName := c.GetBaseProgramName(); baseProgramName != "" {
			prog.BaseProgramName = baseProgramName
		}
		if len(c.fileHashMap) > 0 {
			prog.FileHashMap = c.fileHashMap
		}
		// 如果启用了增量编译，确保 IsOverlay 被设置
		// 即使没有 baseProgramName 和 fileHashMap（第一次增量编译），也设置一个空的 FileHashMap 作为标记
		// 这样 UpdateToDatabaseWithWG 会设置 IsOverlay = true
		if c.GetEnableIncrementalCompile() && prog.FileHashMap == nil {
			prog.FileHashMap = make(map[string]int)
		}
		if prog.DatabaseKind != ssa.ProgramCacheMemory { // save program
			prog.ProcessInfof("[SSA/persist] program %s saving program metadata (ir_program)", prog.Name)
			metaStart := time.Now()
			prog.UpdateToDatabaseWithWG(&wg)
			since := time.Since(metaStart)
			log.Infof("program %s save to database cost: %s", prog.Name, since)
			prog.ProcessInfof("[SSA/persist] program %s program metadata saved, cost %v", prog.Name, since)
		}
		process = 0.90
		return nil
	}

	f5 := func() error {
		saveStart := time.Now()
		remaining := prog.Cache.CountInstruction()
		persisted := prog.Cache.InstructionPersistedCount()
		total := remaining + persisted
		process = 0.90
		if prog.DatabaseKind != ssa.ProgramCacheMemory {
			prog.ProcessInfof("[SSA/persist] program %s flushing IR cache (remaining=%d persisted=%d total=%d) to database",
				prog.Name, remaining, persisted, total)
		} else {
			prog.ProcessInfof("[SSA/persist] program %s finishing cache instruction(len:%d) (memory only, not saved)", prog.Name, remaining)
		}

		if err := prog.Cache.SaveToDatabase(irSaveProgressCallback(prog, total, persisted, 0.90, 1.0, func(p float64) {
			process = p
		})); err != nil {
			return utils.Errorf("persist IR to database failed: %w", err)
		}
		saveTime = time.Since(saveStart)
		if prog.DatabaseKind != ssa.ProgramCacheMemory {
			prog.ProcessInfof("[SSA/persist] program %s IR cache flush finished, cost %v", prog.Name, saveTime)
		}
		return nil
	}
	f6 := func() error {
		wg.Wait()
		return nil
	}
	wrapPhase := func(phase string, fn func() error) func() error {
		return func() error {
			compilePhase = phase
			log.Debugf("ssa.compile.phase enter %s", compilePhase)
			stepErr := fn()
			logPhaseHeap(phase)
			return stepErr
		}
	}
	phaseSteps := []func() error{
		wrapPhase("f1_pre_handler", f1),
		wrapPhase("f2_after_pre", f2),
		wrapPhase("f3_main_build", f3),
		wrapPhase("f4_finish", f4),
		wrapPhase("f5_save_db", f5),
		wrapPhase("f6_wait", f6),
	}
	if rec := c.DiagnosticsRecorder(); rec != nil {
		err = rec.Track("ParseProjectWithFS", phaseSteps...)
	} else {
		for _, step := range phaseSteps {
			if err = step(); err != nil {
				break
			}
		}
	}
	if err != nil {
		return nil, err
	}

	// wall := time.Since(overallStart)
	// totalCompile := calculateTime + preHandlerTime + parseTime + finishTime + saveTime
	// log.Infof(
	// 	"[ssa.compile.summary] program=%s handler_files=%d wall=%s scan=%s pre_handler=%s main_build=%s finish=%s save_instructions=%s phase_sum=%s",
	// 	prog.Name,
	// 	len(handlerFilesMap),
	// 	wall,
	// 	calculateTime,
	// 	preHandlerTime,
	// 	parseTime,
	// 	finishTime,
	// 	saveTime,
	// 	totalCompile,
	// )

	// 输出文件性能汇总表格
	if enableFilePerfLog && filePerfRecorder != nil {
		snapshots := filePerfRecorder.Snapshot()
		if len(snapshots) > 0 {
			table := diagnostics.FormatPerformanceTable("File Compilation Performance Summary", snapshots)
			fmt.Println(table)
		} else {
			fmt.Println("File Performance: no data recorded")
		}
	}
	return NewProgram(prog, c), nil
}

const irSaveHeartbeatInterval = 5 * time.Second

// irSaveProgressCallback builds a SaveToDatabase progress func: updates optional
// compile bar in [processMin, processMax], logs delta steps (>0.0001 on that
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
