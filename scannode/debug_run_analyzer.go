package scannode

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/pprof/profile"
)

// DebugRunAnalysis is the structured analysis result for a debug run directory.
type DebugRunAnalysis struct {
	// RunDir is intentionally NOT returned to the frontend (security).
	// It is kept for internal use but not serialized.
	RunDir     string          `json:"-"`
	Status     string          `json:"status"` // running | completed | failed | unknown
	StartedAt  *time.Time      `json:"started_at,omitempty"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
	Duration   string          `json:"duration,omitempty"`
	Phases     []PhaseAnalysis `json:"phases,omitempty"`
	Samples    []SampleSummary `json:"samples,omitempty"`
	Summary    *RunSummary     `json:"summary,omitempty"`
	Errors     []string        `json:"errors,omitempty"`
	Partial    bool            `json:"partial,omitempty"`
}

// PhaseAnalysis is a per-phase summary.
type PhaseAnalysis struct {
	Phase      string     `json:"phase"`  // compile | scan | unknown
	Source     string     `json:"source"` // log_inferred | status_card | unknown
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Duration   string     `json:"duration,omitempty"`
	Status     string     `json:"status,omitempty"`
}

// SampleSummary is a brief view of a 5-minute pprof snapshot.
// Detail fields are populated inline so the frontend can expand a row
// without a second request.
type SampleSummary struct {
	Sequence     int        `json:"sequence"`
	Label        string     `json:"label"` // e.g. "20260805-120030-initial"
	Timestamp    *time.Time `json:"timestamp,omitempty"`
	EndedAt      *time.Time `json:"ended_at,omitempty"`
	DurationMS   int64      `json:"duration_ms,omitempty"`
	Phase        string     `json:"phase,omitempty"`
	PhaseSource  string     `json:"phase_source,omitempty"` // explicit | boundary_inferred | log_inferred | unknown
	HasCPU       bool       `json:"has_cpu"`
	HasHeap      bool       `json:"has_heap"`
	HasGoroutine bool       `json:"has_goroutine"`
	// File paths are internal only, not serialized to JSON
	CPUFile       string `json:"-"`
	HeapFile      string `json:"-"`
	GoroutineFile string `json:"-"`
	// Inline detail (limited to avoid oversized analysis JSON)
	CPUTop         []PprofTopFunction `json:"cpu_top,omitempty"`
	CPUTopYaklang  []PprofTopFunction `json:"cpu_top_yaklang,omitempty"`
	HeapTop        []PprofTopFunction `json:"heap_top,omitempty"`
	HeapTopYaklang []PprofTopFunction `json:"heap_top_yaklang,omitempty"`
	Goroutines     int                `json:"goroutines,omitempty"`
	LogExcerpt     string             `json:"log_excerpt,omitempty"`
	Status         string             `json:"status,omitempty"` // available | partial | error | pending
	DBStats        *DBOpStatsSummary  `json:"db_stats,omitempty"`
	Runtime        *RuntimeStatsSummary `json:"runtime,omitempty"`
}

// RuntimeStatsSummary mirrors runtime-stats/*.runtime.json written by the
// scan-task pprof collector (host vs this process CPU/memory).
type RuntimeStatsSummary struct {
	Timestamp             string  `json:"timestamp,omitempty"`
	NumCPU                int     `json:"num_cpu,omitempty"`
	Load1                 float64 `json:"load1,omitempty"`
	HostCPUPercent        float64 `json:"host_cpu_percent"`
	ProcessCPUPercent     float64 `json:"process_cpu_percent"`
	HostMemTotalBytes     uint64  `json:"host_mem_total_bytes"`
	HostMemUsedBytes      uint64  `json:"host_mem_used_bytes"`
	HostMemAvailableBytes uint64  `json:"host_mem_available_bytes"`
	ProcessRSSBytes       uint64  `json:"process_rss_bytes"`
	ProcessHeapAllocBytes uint64  `json:"process_heap_alloc_bytes"`
	ProcessHeapSysBytes   uint64  `json:"process_heap_sys_bytes"`
	Goroutines            int     `json:"goroutines,omitempty"`
}

// DBOpStatsSummary mirrors ssadb.DBOpStats JSON written to db-stats/*.db.json.
type DBOpStatsSummary struct {
	Dialect    string                       `json:"dialect,omitempty"`
	Ops        map[string]DBOpBucketSummary `json:"ops,omitempty"`
	TotalCount int64                        `json:"total_count"`
	TotalMs    int64                        `json:"total_ms"`
	WindowMs   int64                        `json:"window_ms,omitempty"`
	ErrorCount int64                        `json:"error_count,omitempty"`
}

// DBOpBucketSummary mirrors ssadb.DBOpBucket JSON.
type DBOpBucketSummary struct {
	Count      int64 `json:"count"`
	TotalMs    int64 `json:"total_ms"`
	MinMs      int64 `json:"min_ms,omitempty"`
	MaxMs      int64 `json:"max_ms,omitempty"`
	AvgMs      int64 `json:"avg_ms,omitempty"`
	ErrorCount int64 `json:"error_count,omitempty"`
}

// SampleDetail is the detailed view of a single 5-minute sample.
type SampleDetail struct {
	SampleSummary
	CPUProfile  *PprofTopAnalysis `json:"cpu_profile,omitempty"`
	HeapProfile *PprofTopAnalysis `json:"heap_profile,omitempty"`
	Goroutines  int               `json:"goroutines,omitempty"`
	LogExcerpt  string            `json:"log_excerpt,omitempty"`
}

// PprofTopAnalysis contains top functions from a pprof profile.
type PprofTopAnalysis struct {
	Kind          string             `json:"kind"`
	TopFunctions  []PprofTopFunction `json:"top_functions,omitempty"`
	YaklangTop    []PprofTopFunction `json:"yaklang_top,omitempty"`
	SampleCount   int64              `json:"sample_count"`
	TotalValue    int64              `json:"total_value"`
	SampleUnit    string             `json:"sample_unit,omitempty"`
	ParseError    string             `json:"parse_error,omitempty"`
	TimeNanos     int64              `json:"time_nanos,omitempty"`
	DurationNanos int64              `json:"duration_nanos,omitempty"`
}

// PprofTopFunction is a single function entry.
type PprofTopFunction struct {
	Name      string `json:"name"`
	CumValue  int64  `json:"cum_value"`
	CumPct    string `json:"cum_pct"`
	FlatValue int64  `json:"flat_value"`
	FlatPct   string `json:"flat_pct"`
}

// RunSummary is the overall run summary.
type RunSummary struct {
	TotalDurationMS   int64             `json:"total_duration_ms"`
	CPUProfileFiles   int               `json:"cpu_profile_files"`
	HeapProfileFiles  int               `json:"heap_profile_files"`
	GoroutineFiles    int               `json:"goroutine_files"`
	HasLog            bool              `json:"has_log"`
	HasReport         bool              `json:"has_report"`
	HasSSADB          bool              `json:"has_ssadb"`
	HasCmd            bool              `json:"has_cmd"`
	CompilePhaseFound bool              `json:"compile_phase_found"`
	ScanPhaseFound    bool              `json:"scan_phase_found"`
	DBStatsTotal      *DBOpStatsSummary `json:"db_stats_total,omitempty"`
}

// AnalyzeDebugRun parses a debug run directory and returns structured analysis.
// It tolerates missing files, partial writes, and corrupted pprof data.
func AnalyzeDebugRun(dir string) DebugRunAnalysis {
	return AnalyzeDebugRunWithStatus(dir, "")
}

// AnalyzeDebugRunWithStatus analyzes a debug run directory. When taskStatus
// is non-empty (e.g. "succeeded"/"failed"), it takes precedence over the
// log-derived status because the log may not contain a completion marker.
func AnalyzeDebugRunWithStatus(dir string, taskStatus string) DebugRunAnalysis {
	result := DebugRunAnalysis{
		RunDir: dir,
		Status: "unknown",
	}

	// Check directory exists
	info, err := os.Stat(dir)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("debug dir not accessible: %v", err))
		result.Status = "error"
		return result
	}
	if !info.IsDir() {
		result.Errors = append(result.Errors, "debug path is not a directory")
		result.Status = "error"
		return result
	}

	// Parse log for timing and phase information
	logPath := filepath.Join(dir, "log")
	var logContent string
	if logData, err := os.ReadFile(logPath); err == nil {
		logContent = string(logData)
	} else {
		result.Errors = append(result.Errors, "log file not readable")
	}
	if strings.TrimSpace(logContent) == "" {
		if taskLog := resolveTaskLogByDebugDir(dir); taskLog != "" {
			if logData, err := os.ReadFile(taskLog); err == nil {
				logContent = string(logData)
			}
		}
	}
	if strings.TrimSpace(logContent) != "" {
		result.setTimesFromLog(logContent)
		result.setPhasesFromLog(logContent)
		if taskStatus == "" {
			result.Status = result.determineStatus(logContent)
		}
	}
	if taskStatus != "" {
		result.Status = normalizeTaskStatus(taskStatus)
	}

	// Child collector logs often only contain ERROR/pprof lines and miss the
	// SSA phase markers that live in the node task log. Fold that log in when
	// phase detection came up empty so live/cancel analysis still shows compile.
	if !hasRecognizedDebugPhases(result.Phases) {
		if enriched := enrichLogWithTaskLog(dir, logContent); enriched != logContent && strings.TrimSpace(enriched) != "" {
			logContent = enriched
			result.setTimesFromLog(logContent)
			result.setPhasesFromLog(logContent)
			if taskStatus == "" {
				result.Status = result.determineStatus(logContent)
			} else {
				result.Status = normalizeTaskStatus(taskStatus)
			}
		}
	}

	// Parse pprof samples
	samples := result.collectSamples(dir, logContent)
	result.Samples = samples

	// Build summary
	result.Summary = result.buildSummary(dir, samples)
	result.Partial = len(result.Errors) > 0

	return result
}

// AnalyzeSample returns detailed analysis for a single pprof sample.
func AnalyzeSample(dir string, label string) (SampleDetail, error) {
	cpuDir := filepath.Join(dir, "cpu-pprof")
	memDir := filepath.Join(dir, "memory-pprof")
	goroutineDir := filepath.Join(dir, "goroutine-pprof")

	detail := SampleDetail{}

	// Find matching files by label prefix
	cpuFile := findPprofFile(cpuDir, label, ".cpu.prof")
	heapFile := findPprofFile(memDir, label, ".mem.prof")
	goroutineFile := findPprofFile(goroutineDir, label, ".goroutine.prof")

	detail.CPUFile = cpuFile
	detail.HeapFile = heapFile
	detail.GoroutineFile = goroutineFile
	detail.HasCPU = cpuFile != ""
	detail.HasHeap = heapFile != ""
	detail.HasGoroutine = goroutineFile != ""

	// Parse CPU profile
	if cpuFile != "" {
		detail.CPUProfile = parsePprofFile(cpuFile, "cpu")
		if detail.CPUProfile != nil {
			detail.CPUTop = limitTopFunctions(detail.CPUProfile.TopFunctions, 10)
			detail.CPUTopYaklang = limitTopFunctions(detail.CPUProfile.YaklangTop, 10)
			ts := parseLabelTimestamp(label)
			detail.Label = label
			detail.Timestamp = ts
			applySampleWindow(&detail.SampleSummary, detail.CPUProfile)
		}
	}

	// Parse heap profile
	if heapFile != "" {
		detail.HeapProfile = parsePprofFile(heapFile, "heap")
		if detail.HeapProfile != nil {
			detail.HeapTop = limitTopFunctions(detail.HeapProfile.TopFunctions, 10)
			detail.HeapTopYaklang = limitTopFunctions(detail.HeapProfile.YaklangTop, 10)
		}
	}

	// Count goroutines from goroutine profile
	if goroutineFile != "" {
		detail.Goroutines = countGoroutines(goroutineFile)
	}

	// Extract log excerpt around this sample's timestamp
	logPath := filepath.Join(dir, "log")
	if logData, err := os.ReadFile(logPath); err == nil {
		detail.LogExcerpt = extractLogExcerpt(string(logData), label, 4096)
	}

	detail.DBStats = loadSampleDBStats(dir, label)

	return detail, nil
}

// GenerateDebugZip creates a zip archive of the debug run directory.
func GenerateDebugZip(dir string) (string, error) {
	// Validate dir exists and is a directory
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("debug dir not accessible: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory")
	}

	// Path traversal check: ensure dir is under an allowed base
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	// Strict directory boundary check: resolve and verify the directory
	// is a real directory (not a symlink to escape), and does not contain
	// path traversal sequences.
	cleanDir := filepath.Clean(absDir)
	if cleanDir != absDir {
		return "", fmt.Errorf("invalid path: not clean")
	}
	if !filepath.IsAbs(cleanDir) && strings.HasPrefix(cleanDir, "..") {
		return "", fmt.Errorf("invalid path: relative traversal")
	}

	zipPath := filepath.Join(filepath.Dir(absDir), filepath.Base(absDir)+".zip")
	if err := generateZipTo(absDir, zipPath); err != nil {
		return "", fmt.Errorf("generate zip: %w", err)
	}
	return zipPath, nil
}

// --- Internal helpers ---

func (r *DebugRunAnalysis) setTimesFromLog(logContent string) {
	// Try to find start time from log
	lines := strings.Split(logContent, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Try to parse timestamp from log line
		if t := parseLogTimestamp(line); t != nil {
			if r.StartedAt == nil {
				r.StartedAt = t
			}
			r.FinishedAt = t // last timestamp
		}
	}
	if r.StartedAt != nil && r.FinishedAt != nil {
		r.Duration = formatDuration(r.FinishedAt.Sub(*r.StartedAt))
	}
}

func (r *DebugRunAnalysis) setPhasesFromLog(logContent string) {
	var phases []PhaseAnalysis
	compileStart, scanStart := findPhaseTransitionsInLog(logContent)

	if compileStart != nil {
		var finished *time.Time
		phaseStatus := "completed"
		if scanStart != nil {
			finished = scanStart
		} else if isActiveDebugStatus(r.Status) {
			// Still compiling: leave the window open so later samples map here.
			phaseStatus = "running"
		} else {
			finished = r.FinishedAt
		}
		phases = append(phases, PhaseAnalysis{
			Phase:      "compile",
			Source:     "log_inferred",
			StartedAt:  compileStart,
			FinishedAt: finished,
			Duration:   durationStr(compileStart, finished),
			Status:     phaseStatus,
		})
	}
	if scanStart != nil {
		phases = append(phases, PhaseAnalysis{
			Phase:      "scan",
			Source:     "log_inferred",
			StartedAt:  scanStart,
			FinishedAt: r.FinishedAt,
			Duration:   durationStr(scanStart, r.FinishedAt),
			Status:     r.Status,
		})
	}

	if len(phases) == 0 {
		phases = append(phases, PhaseAnalysis{
			Phase:  "unknown",
			Source: "unknown",
			Status: r.Status,
		})
	}

	r.Phases = phases
}

func isActiveDebugStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "partial", "cancel_requested", "unknown", "":
		return true
	default:
		return false
	}
}

func hasRecognizedDebugPhases(phases []PhaseAnalysis) bool {
	for _, phase := range phases {
		name := strings.ToLower(strings.TrimSpace(phase.Phase))
		if name != "" && name != "unknown" {
			return true
		}
	}
	return false
}

func enrichLogWithTaskLog(debugDir, logContent string) string {
	taskLog := resolveTaskLogByDebugDir(debugDir)
	if taskLog == "" {
		return logContent
	}
	data, err := os.ReadFile(taskLog)
	if err != nil || len(data) == 0 {
		return logContent
	}
	taskText := string(data)
	if strings.TrimSpace(logContent) == "" {
		return taskText
	}
	if strings.Contains(logContent, taskText) {
		return logContent
	}
	return logContent + "\n--- node task log ---\n" + taskText
}

func (r *DebugRunAnalysis) determineStatus(logContent string) string {
	lower := strings.ToLower(logContent)
	if strings.Contains(lower, "code scan done") {
		return "completed"
	}
	if strings.Contains(lower, "code scan failed") || strings.Contains(lower, "panic") {
		return "failed"
	}
	// If log is still being written (file exists but no completion marker)
	if len(logContent) > 0 {
		return "running"
	}
	return "unknown"
}

func (r *DebugRunAnalysis) collectSamples(dir, logContent string) []SampleSummary {
	type fileEntry struct {
		dir      string
		suffix   string
		fileType string
	}

	cpuDir := filepath.Join(dir, "cpu-pprof")
	memDir := filepath.Join(dir, "memory-pprof")
	goroutineDir := filepath.Join(dir, "goroutine-pprof")
	dbStatsDir := filepath.Join(dir, "db-stats")
	runtimeStatsDir := filepath.Join(dir, "runtime-stats")

	entries := []fileEntry{
		{cpuDir, ".cpu.prof", "cpu"},
		{memDir, ".mem.prof", "heap"},
		{goroutineDir, ".goroutine.prof", "goroutine"},
		{dbStatsDir, ".db.json", "db_stats"},
		{runtimeStatsDir, ".runtime.json", "runtime"},
	}

	// Collect all sample labels
	labelMap := make(map[string]*SampleSummary)
	seq := 0

	for _, entry := range entries {
		files, err := os.ReadDir(entry.dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			if !strings.HasSuffix(name, entry.suffix) {
				continue
			}
			// Extract label: filename without suffix
			label := strings.TrimSuffix(name, entry.suffix)
			s, ok := labelMap[label]
			if !ok {
				seq++
				ts := parseLabelTimestamp(label)
				s = &SampleSummary{
					Sequence:  seq,
					Label:     label,
					Timestamp: ts,
				}
				labelMap[label] = s
			}
			switch entry.fileType {
			case "cpu":
				s.HasCPU = true
				s.CPUFile = filepath.Join(entry.dir, name)
			case "heap":
				s.HasHeap = true
				s.HeapFile = filepath.Join(entry.dir, name)
			case "goroutine":
				s.HasGoroutine = true
				s.GoroutineFile = filepath.Join(entry.dir, name)
			case "db_stats":
				s.DBStats = loadDBStatsFile(filepath.Join(entry.dir, name))
			case "runtime":
				s.Runtime = loadRuntimeStatsFile(filepath.Join(entry.dir, name))
			}
		}
	}

	// Assign phases based on timestamps
	if len(r.Phases) > 0 {
		for _, s := range labelMap {
			s.Phase, s.PhaseSource = assignPhase(s.Timestamp, r.Phases)
		}
	}

	// Populate inline details for each sample (limited top lists for UI)
	for _, s := range labelMap {
		s.Status = "available"
		if s.CPUFile != "" {
			if p := parsePprofFile(s.CPUFile, "cpu"); p != nil {
				if p.ParseError != "" {
					s.Status = "partial"
				}
				s.CPUTop = limitTopFunctions(p.TopFunctions, 10)
				s.CPUTopYaklang = limitTopFunctions(p.YaklangTop, 10)
				applySampleWindow(s, p)
			}
		}
		if s.HeapFile != "" {
			if p := parsePprofFile(s.HeapFile, "heap"); p != nil {
				if p.ParseError != "" && s.Status == "available" {
					s.Status = "partial"
				}
				s.HeapTop = limitTopFunctions(p.TopFunctions, 10)
				s.HeapTopYaklang = limitTopFunctions(p.YaklangTop, 10)
				// Heap snapshots are instantaneous; only fill window from CPU.
			}
		}
		if s.GoroutineFile != "" {
			if count, err := parseGoroutineCount(s.GoroutineFile); err != nil {
				s.Status = "partial"
			} else {
				s.Goroutines = count
			}
		}
		if s.DBStats == nil {
			s.DBStats = loadSampleDBStats(dir, s.Label)
		}
		// Log excerpt limited to 500 chars
		if s.Label != "" {
			if logData, err := os.ReadFile(filepath.Join(dir, "log")); err == nil {
				s.LogExcerpt = extractLogExcerpt(string(logData), s.Label, 500)
			} else if taskLog := resolveTaskLogByDebugDir(dir); taskLog != "" {
				if logData, err := os.ReadFile(taskLog); err == nil {
					s.LogExcerpt = extractLogExcerpt(string(logData), s.Label, 500)
				}
			}
		}
	}

	// Convert to sorted slice
	result := make([]SampleSummary, 0, len(labelMap))
	for _, s := range labelMap {
		result = append(result, *s)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Sequence < result[j].Sequence
	})

	return result
}

// limitTopFunctions returns at most n top functions.
func limitTopFunctions(fns []PprofTopFunction, n int) []PprofTopFunction {
	if len(fns) > n {
		return fns[:n]
	}
	return fns
}

func (r *DebugRunAnalysis) buildSummary(dir string, samples []SampleSummary) *RunSummary {
	summary := &RunSummary{}
	if r.StartedAt != nil && r.FinishedAt != nil {
		summary.TotalDurationMS = r.FinishedAt.Sub(*r.StartedAt).Milliseconds()
	}

	for _, s := range samples {
		if s.HasCPU {
			summary.CPUProfileFiles++
		}
		if s.HasHeap {
			summary.HeapProfileFiles++
		}
		if s.HasGoroutine {
			summary.GoroutineFiles++
		}
	}

	// Check for other files
	summary.HasLog = fileExists(filepath.Join(dir, "log"))
	summary.HasReport = fileExists(filepath.Join(dir, "report")) || dirHasFiles(filepath.Join(dir, "report"))
	summary.HasSSADB = fileExists(filepath.Join(dir, "ssadb.db"))
	summary.HasCmd = fileExists(filepath.Join(dir, "cmd.txt"))

	for _, p := range r.Phases {
		if p.Phase == "compile" {
			summary.CompilePhaseFound = true
		}
		if p.Phase == "scan" {
			summary.ScanPhaseFound = true
		}
	}

	summary.DBStatsTotal = aggregateDBStats(samples)

	return summary
}

func loadSampleDBStats(dir, label string) *DBOpStatsSummary {
	if dir == "" || label == "" {
		return nil
	}
	return loadDBStatsFile(filepath.Join(dir, "db-stats", label+".db.json"))
}

func loadDBStatsFile(path string) *DBOpStatsSummary {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return nil
	}
	var stats DBOpStatsSummary
	if err := json.Unmarshal(raw, &stats); err != nil {
		return nil
	}
	return &stats
}

func loadRuntimeStatsFile(path string) *RuntimeStatsSummary {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return nil
	}
	var stats RuntimeStatsSummary
	if err := json.Unmarshal(raw, &stats); err != nil {
		return nil
	}
	return &stats
}

func aggregateDBStats(samples []SampleSummary) *DBOpStatsSummary {
	ops := make(map[string]DBOpBucketSummary)
	total := DBOpStatsSummary{Ops: ops}
	any := false
	for _, s := range samples {
		if s.DBStats == nil {
			continue
		}
		any = true
		if total.Dialect == "" {
			total.Dialect = s.DBStats.Dialect
		}
		total.TotalCount += s.DBStats.TotalCount
		total.TotalMs += s.DBStats.TotalMs
		total.ErrorCount += s.DBStats.ErrorCount
		for kind, bucket := range s.DBStats.Ops {
			cur := ops[kind]
			cur.Count += bucket.Count
			cur.TotalMs += bucket.TotalMs
			cur.ErrorCount += bucket.ErrorCount
			if bucket.MaxMs > cur.MaxMs {
				cur.MaxMs = bucket.MaxMs
			}
			if bucket.MinMs > 0 && (cur.MinMs == 0 || bucket.MinMs < cur.MinMs) {
				cur.MinMs = bucket.MinMs
			}
			ops[kind] = cur
		}
	}
	if !any {
		return nil
	}
	for kind, bucket := range ops {
		if bucket.Count > 0 {
			bucket.AvgMs = bucket.TotalMs / bucket.Count
		}
		ops[kind] = bucket
	}
	total.Ops = ops
	return &total
}

func parsePprofFile(path string, kind string) *PprofTopAnalysis {
	f, err := os.Open(path)
	if err != nil {
		return &PprofTopAnalysis{Kind: kind, ParseError: fmt.Sprintf("open: %v", err)}
	}
	defer f.Close()

	p, err := profile.Parse(f)
	if err != nil {
		return &PprofTopAnalysis{Kind: kind, ParseError: fmt.Sprintf("parse: %v", err)}
	}

	return buildPprofTopAnalysis(p, kind)
}

func buildPprofTopAnalysis(p *profile.Profile, kind string) *PprofTopAnalysis {
	result := &PprofTopAnalysis{Kind: kind}
	if p == nil || len(p.Sample) == 0 {
		result.ParseError = "profile contains no samples"
		return result
	}

	result.TimeNanos = p.TimeNanos
	result.DurationNanos = p.DurationNanos

	sampleTypeIdx := 0
	for i, st := range p.SampleType {
		if st.Type == "samples" || st.Type == "cpu" || st.Type == "alloc_space" || st.Type == "inuse_space" || st.Type == "alloc_objects" || st.Type == "inuse_objects" {
			sampleTypeIdx = i
			break
		}
	}

	funcMap := make(map[string]*pprofFuncStats)
	var totalValue int64

	for _, sample := range p.Sample {
		if sampleTypeIdx >= len(sample.Value) {
			continue
		}
		value := sample.Value[sampleTypeIdx]
		totalValue += value

		if len(sample.Location) > 0 {
			loc := sample.Location[len(sample.Location)-1]
			for _, line := range loc.Line {
				if line.Function != nil {
					name := line.Function.Name
					fs, ok := funcMap[name]
					if !ok {
						fs = &pprofFuncStats{name: name}
						funcMap[name] = fs
					}
					fs.flat += value
				}
			}
		}

		seen := make(map[string]bool)
		for _, loc := range sample.Location {
			for _, line := range loc.Line {
				if line.Function != nil {
					name := line.Function.Name
					if !seen[name] {
						seen[name] = true
						fs, ok := funcMap[name]
						if !ok {
							fs = &pprofFuncStats{name: name}
							funcMap[name] = fs
						}
						fs.cum += value
					}
				}
			}
		}
	}

	var stats []*pprofFuncStats
	for _, fs := range funcMap {
		stats = append(stats, fs)
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].cum != stats[j].cum {
			return stats[i].cum > stats[j].cum
		}
		return stats[i].name < stats[j].name
	})

	result.TopFunctions = pprofTopFromStats(stats, totalValue, 20, nil)
	result.YaklangTop = pprofTopFromStats(stats, totalValue, 20, isYaklangPprofFunc)
	result.SampleCount = int64(len(p.Sample))
	result.TotalValue = totalValue
	if len(p.SampleType) > sampleTypeIdx {
		result.SampleUnit = p.SampleType[sampleTypeIdx].Unit
	}
	return result
}

type pprofFuncStats struct {
	name string
	flat int64
	cum  int64
}

func pprofTopFromStats(
	stats []*pprofFuncStats,
	totalValue int64,
	n int,
	pred func(string) bool,
) []PprofTopFunction {
	out := make([]PprofTopFunction, 0, n)
	for _, fs := range stats {
		if pred != nil && !pred(fs.name) {
			continue
		}
		var cumPct, flatPct string
		if totalValue > 0 {
			cumPct = fmt.Sprintf("%.2f%%", float64(fs.cum)*100/float64(totalValue))
			flatPct = fmt.Sprintf("%.2f%%", float64(fs.flat)*100/float64(totalValue))
		}
		out = append(out, PprofTopFunction{
			Name: fs.name, CumValue: fs.cum, CumPct: cumPct, FlatValue: fs.flat, FlatPct: flatPct,
		})
		if len(out) >= n {
			break
		}
	}
	return out
}

const yaklangPprofPrefix = "github.com/yaklang/yaklang"

func isYaklangPprofFunc(name string) bool {
	return strings.Contains(name, yaklangPprofPrefix)
}

// applySampleWindow sets EndedAt / DurationMS from the CPU profile duration.
func applySampleWindow(s *SampleSummary, p *PprofTopAnalysis) {
	if s == nil || p == nil || p.DurationNanos <= 0 {
		return
	}
	s.DurationMS = p.DurationNanos / int64(time.Millisecond)
	var start time.Time
	if s.Timestamp != nil {
		start = *s.Timestamp
	} else if p.TimeNanos > 0 {
		start = time.Unix(0, p.TimeNanos).UTC()
		s.Timestamp = &start
	} else {
		return
	}
	end := start.Add(time.Duration(p.DurationNanos))
	s.EndedAt = &end
}

func countGoroutines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	p, err := profile.Parse(f)
	if err != nil {
		return 0
	}
	return len(p.Sample)
}

func parseGoroutineCount(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	p, err := profile.Parse(f)
	if err != nil {
		return 0, err
	}
	return len(p.Sample), nil
}

func findPprofFile(dir, label, suffix string) string {
	files, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	// Exact match first
	target := label + suffix
	for _, f := range files {
		if f.Name() == target {
			return filepath.Join(dir, f.Name())
		}
	}
	// Prefix match
	for _, f := range files {
		name := f.Name()
		if strings.HasPrefix(name, label) && strings.HasSuffix(name, suffix) {
			return filepath.Join(dir, name)
		}
	}
	return ""
}

func parseLogTimestamp(line string) *time.Time {
	// Try common log timestamp formats
	formats := []string{
		"2006/01/02 15:04:05",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.000000",
	}
	for _, fmt := range formats {
		// Scan the whole line for the timestamp because node logs can be
		// prefixed with ANSI color codes or [INFO].
		for i := 0; i+len(fmt) <= len(line); i++ {
			t, err := time.ParseInLocation(fmt, line[i:i+len(fmt)], time.Local)
			if err == nil {
				return &t
			}
		}
	}
	return nil
}

func parseLabelTimestamp(label string) *time.Time {
	// Labels are like "20260805-120030-initial" or "20260805-120030"
	// The timestamp part is "20260805-120030" (15 chars: date + "-" + time)
	if len(label) >= 15 {
		tsPart := label[:15]
		if t, err := time.ParseInLocation("20060102-150405", tsPart, time.Local); err == nil {
			return &t
		}
	}
	return nil
}

func findPhaseTransitionsInLog(logContent string) (compileStart, scanStart *time.Time) {
	lines := strings.Split(logContent, "\n")
	for _, line := range lines {
		lower := strings.ToLower(line)
		ts := parseLogTimestamp(line)

		if compileStart == nil && ts != nil {
			if looksLikeCompilePhaseLine(lower) {
				compileStart = ts
			}
		}
		if scanStart == nil && ts != nil {
			if looksLikeScanPhaseLine(lower) {
				scanStart = ts
			}
		}
		// Also check for "start to scan code" or "start to compile"
		if strings.Contains(lower, "start to scan") && scanStart == nil {
			if t := parseLogTimestamp(line); t != nil {
				scanStart = t
			}
		}
		if strings.Contains(lower, "start to compile") && compileStart == nil {
			if t := parseLogTimestamp(line); t != nil {
				compileStart = t
			}
		}
	}
	return
}

func looksLikeCompilePhaseLine(lower string) bool {
	if strings.Contains(lower, `"id":"ssa-phase"`) && strings.Contains(lower, `"data":"compile"`) {
		return true
	}
	if strings.Contains(lower, "ssa 任务阶段: compile") || strings.Contains(lower, "ssa 任务阶段：compile") {
		return true
	}
	if (strings.Contains(lower, "compile") || strings.Contains(lower, "load-program")) &&
		(strings.Contains(lower, "phase") || strings.Contains(lower, "\u9636\u6bb5")) {
		return true
	}
	return false
}

func looksLikeScanPhaseLine(lower string) bool {
	if strings.Contains(lower, `"id":"ssa-phase"`) && strings.Contains(lower, `"data":"scan"`) {
		return true
	}
	if strings.Contains(lower, "ssa 任务阶段: scan") || strings.Contains(lower, "ssa 任务阶段：scan") {
		return true
	}
	if (strings.Contains(lower, "scan") || strings.Contains(lower, "ingest")) &&
		(strings.Contains(lower, "phase") || strings.Contains(lower, "\u9636\u6bb5")) {
		// Avoid treating compile-phase lines that also mention scan tooling.
		if strings.Contains(lower, "compile") && !strings.Contains(lower, `"data":"scan"`) {
			return false
		}
		return true
	}
	return false
}

func assignPhase(ts *time.Time, phases []PhaseAnalysis) (string, string) {
	if ts == nil {
		return "unknown", "unknown"
	}
	for _, p := range phases {
		if p.StartedAt == nil {
			continue
		}
		finish := p.FinishedAt
		if finish == nil {
			// Current phase
			if ts.After(*p.StartedAt) || ts.Equal(*p.StartedAt) {
				return p.Phase, p.Source
			}
			continue
		}
		if (ts.After(*p.StartedAt) || ts.Equal(*p.StartedAt)) && ts.Before(*finish) {
			return p.Phase, p.Source
		}
	}
	return "unknown", "unknown"
}

func extractLogExcerpt(logContent string, label string, maxLen int) string {
	// Try to find the timestamp from the label and extract surrounding log
	ts := parseLabelTimestamp(label)
	if ts == nil {
		if len(logContent) > maxLen {
			return logContent[:maxLen]
		}
		return logContent
	}
	tsStr := ts.Format("2006/01/02 15:04:05")
	lines := strings.Split(logContent, "\n")
	var startIdx = -1
	for i, line := range lines {
		if strings.Contains(line, tsStr) {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		if len(logContent) > maxLen {
			return logContent[:maxLen]
		}
		return logContent
	}
	// Extract ~20 lines around the timestamp
	start := startIdx - 5
	if start < 0 {
		start = 0
	}
	end := startIdx + 15
	if end > len(lines) {
		end = len(lines)
	}
	excerpt := strings.Join(lines[start:end], "\n")
	if len(excerpt) > maxLen {
		excerpt = excerpt[:maxLen]
	}
	return excerpt
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func dirHasFiles(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	return len(entries) > 0
}

func durationStr(start, end *time.Time) string {
	if start == nil || end == nil {
		return ""
	}
	return formatDuration(end.Sub(*start))
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", m, s)
}

// --- Zip generation using archive/zip ---

func generateZipTo(absDir, zipPath string) error {
	f, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("create zip: %w", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	return filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if path == absDir {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(absDir, path)
		w, err := zw.Create(rel)
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return nil // skip unreadable
		}
		defer src.Close()
		_, err = io.Copy(w, src)
		return err
	})
}

// normalizeTaskStatus maps Legion job statuses to debug analysis statuses.
func normalizeTaskStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "completed", "ok":
		return "completed"
	case "failed", "error":
		return "failed"
	case "cancelled", "canceled", "timed_out", "lost":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

// resolveNodeTaskLogFallback locates the node-side per-task log when the debug
// directory log is empty (older runs before log tee was enabled). The debug
// dir layout is <node>/debug-runs/debug/<task>_<attempt>, while task logs are
// written to <node>/logs/<task>_<subtask>_<attempt>.log.
func resolveNodeTaskLogFallback(firstFile string) string {
	if firstFile == "" {
		return ""
	}
	// firstFile = <node>/debug-runs/debug/<task>_<attempt>/cpu-pprof/xxx.cpu.prof
	debugRunDir := filepath.Dir(filepath.Dir(firstFile))
	debugRunsRoot := filepath.Dir(debugRunDir)
	nodeRoot := filepath.Dir(debugRunsRoot)
	logsDir := filepath.Join(nodeRoot, "logs")

	base := filepath.Base(debugRunDir)
	lastUnderscore := strings.LastIndex(base, "_")
	if lastUnderscore <= 0 {
		return ""
	}
	taskID := base[:lastUnderscore]
	attemptID := base[lastUnderscore+1:]
	if taskID == "" || attemptID == "" {
		return ""
	}

	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, "_"+attemptID+".log") {
			continue
		}
		if strings.HasPrefix(name, taskID+"_") {
			return filepath.Join(logsDir, name)
		}
	}
	return ""
}

// mergeTaskLogIntoDebugDir copies the node-side per-task log into
// debugDir/log when that file is empty or absent, so the debug zip and
// analysis contain the scan stdout/stderr even if the child profiler did
// not redirect logs.
func mergeTaskLogIntoDebugDir(debugDir string) {
	if debugDir == "" {
		return
	}
	taskLog := resolveTaskLogByDebugDir(debugDir)
	if taskLog == "" {
		return
	}
	data, err := os.ReadFile(taskLog)
	if err != nil || len(data) == 0 {
		return
	}
	debugLog := filepath.Join(debugDir, "log")
	if existing, err := os.ReadFile(debugLog); err == nil && len(existing) > 0 {
		// Keep child collector content and append task log after a separator.
		out := append(append(existing, '\n'), []byte("--- node task log ---\n")...)
		out = append(out, data...)
		_ = os.WriteFile(debugLog, out, 0o644)
		return
	}
	_ = os.WriteFile(debugLog, data, 0o644)
}

// resolveTaskLogByDebugDir derives the node task log path from a debug run
// directory without requiring a pprof file to exist first.
func resolveTaskLogByDebugDir(debugDir string) string {
	if debugDir == "" {
		return ""
	}
	base := filepath.Base(debugDir)
	lastUnderscore := strings.LastIndex(base, "_")
	if lastUnderscore <= 0 {
		return ""
	}
	taskID := base[:lastUnderscore]
	attemptID := base[lastUnderscore+1:]
	if taskID == "" || attemptID == "" {
		return ""
	}
	// debugDir = <node>/debug-runs/debug/<base>
	debugRoot := filepath.Dir(debugDir)
	debugRunsRoot := filepath.Dir(debugRoot)
	nodeRoot := filepath.Dir(debugRunsRoot)
	logsDir := filepath.Join(nodeRoot, "logs")

	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, taskID+"_") && strings.HasSuffix(name, "_"+attemptID+".log") {
			return filepath.Join(logsDir, name)
		}
	}
	return ""
}
