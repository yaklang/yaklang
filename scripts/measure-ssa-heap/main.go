// measure-ssa-heap compiles a local project in the current process and reports
// process-local heap metrics. Pair with:
//
//	YAK_SSA_HEAP_LOG=1              — phase retained heap (ssa_compile_fs)
//	YAK_SSA_HEAP_PROFILE_DIR=<dir>  — heap.pb.gz after each compile phase
//	YAK_SSA_LEGACY_TOPLEVEL=1       — legacy whole-file AST closure (A/B)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type compileStats struct {
	Label          string  `json:"label"`
	Mode           string  `json:"mode"`
	Path           string  `json:"path"`
	ProgramName    string  `json:"program_name"`
	CompileSeconds float64 `json:"compile_seconds"`
	Funcs          int     `json:"funcs"`
	Blueprints     int     `json:"blueprints"`
	SubPrograms    int     `json:"sub_programs"`
	Instructions   int     `json:"instructions"`
	RootBuildTasks int     `json:"root_build_tasks"`
	LineCount      int     `json:"line_count"`
	HeapInuseEndMB float64 `json:"heap_inuse_end_mb"`
	HeapObjectsEnd uint64  `json:"heap_objects_end"`
	RssEndMB       float64 `json:"rss_end_mb"`
}

func readProcRSSKB() uint64 {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, line := range splitLines(data) {
		if len(line) > 6 && string(line[:6]) == "VmRSS:" {
			var kb uint64
			_, _ = fmt.Sscanf(string(line), "VmRSS:%d", &kb)
			return kb
		}
	}
	return 0
}

func splitLines(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, c := range b {
		if c == '\n' {
			out = append(out, b[start:i])
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
}

func logMem(tag string) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	rss := readProcRSSKB()
	fmt.Fprintf(os.Stderr,
		"[measure] %-20s go_HeapInuse=%6.1fMB go_HeapAlloc=%6.1fMB rss=%6.1fMB heap_objects=%d\n",
		tag,
		float64(m.HeapInuse)/1024/1024,
		float64(m.HeapAlloc)/1024/1024,
		float64(rss)/1024,
		m.HeapObjects,
	)
}

func collectStats(prog *ssa.Program) compileStats {
	st := compileStats{}
	if prog == nil {
		return st
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	st.RootBuildTasks = app.RootBuildCount()
	st.LineCount = app.LineCount
	st.Funcs = app.Funcs.Len()
	st.Blueprints = app.Blueprint.Len()
	st.SubPrograms = app.UpStream.Len()
	if app.Cache != nil {
		st.Instructions = app.Cache.CountInstruction()
	}
	return st
}

func main() {
	path := flag.String("path", "", "project root directory")
	label := flag.String("label", "", "run label for logs")
	lang := flag.String("language", "java", "ssa language")
	profileDir := flag.String("profile-dir", "", "write phase heap profiles (sets YAK_SSA_HEAP_PROFILE_DIR)")
	statsOut := flag.String("stats-out", "", "write compile stats JSON to this file")
	flag.Parse()
	if *path == "" {
		fmt.Fprintln(os.Stderr, "usage: measure-ssa-heap -path <dir> [-label name] [-profile-dir dir] [-stats-out file.json]")
		os.Exit(2)
	}

	mode := "skeleton"
	if os.Getenv("YAK_SSA_LEGACY_TOPLEVEL") != "" {
		mode = "legacy"
	}
	if *label == "" {
		*label = mode
	}
	if *profileDir != "" {
		_ = os.MkdirAll(*profileDir, 0o755)
		_ = os.Setenv("YAK_SSA_HEAP_PROFILE_DIR", *profileDir)
	}
	if os.Getenv("YAK_SSA_HEAP_LOG") == "" {
		_ = os.Setenv("YAK_SSA_HEAP_LOG", "1")
	}

	fmt.Fprintf(os.Stderr, "[measure] start label=%s mode=%s path=%s profile_dir=%s\n",
		*label, mode, *path, os.Getenv("YAK_SSA_HEAP_PROFILE_DIR"))

	runtime.GC()
	logMem("before_compile")
	diagnostics.LogHeapSnapshot("measure_before", true)

	progName := fmt.Sprintf("heap-measure-%s-%d", *label, time.Now().UnixNano())
	language := ssaconfig.Language(*lang)
	start := time.Now()
	progs, err := ssaapi.ParseProjectFromPath(*path,
		ssaapi.WithLanguage(language),
		ssaapi.WithMemory(),
		ssaapi.WithProgramName(progName),
		ssaapi.WithFilePerformanceLog(true),
	)
	elapsed := time.Since(start)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[measure] compile error: %v\n", err)
		os.Exit(1)
	}

	var ssaProg *ssa.Program
	if len(progs) > 0 && progs[0] != nil {
		ssaProg = progs[0].Program
	}
	stats := collectStats(ssaProg)
	stats.Label = *label
	stats.Mode = mode
	stats.Path = *path
	stats.ProgramName = progName
	stats.CompileSeconds = elapsed.Seconds()

	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	stats.HeapInuseEndMB = float64(m.HeapInuse) / 1024 / 1024
	stats.HeapObjectsEnd = m.HeapObjects
	stats.RssEndMB = float64(readProcRSSKB()) / 1024

	logMem("after_compile")
	diagnostics.LogHeapSnapshot("measure_after", true)
	fmt.Fprintf(os.Stderr, "[measure] stats funcs=%d blueprints=%d instructions=%d root_builds=%d sub_programs=%d lines=%d compile=%v\n",
		stats.Funcs, stats.Blueprints, stats.Instructions, stats.RootBuildTasks, stats.SubPrograms, stats.LineCount, elapsed)
	fmt.Fprintf(os.Stderr, "[measure] done label=%s program=%s\n", *label, progName)

	if *statsOut != "" {
		_ = os.MkdirAll(filepath.Dir(*statsOut), 0o755)
		b, _ := json.MarshalIndent(stats, "", "  ")
		if err := os.WriteFile(*statsOut, b, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "[measure] write stats: %v\n", err)
		}
	}
}
