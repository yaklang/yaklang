package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/yak/kotlin/kotlin2ssa"
	"github.com/yaklang/yaklang/common/yak/lua/lua2ssa"
	"github.com/yaklang/yaklang/common/yak/rust/rust2ssa"
	"github.com/yaklang/yaklang/common/yak/scala/scala2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type parseFunc func(string, ...*ssa.AntlrCache) (any, error)

type fileResult struct {
	path    string
	status  string
	elapsed time.Duration
	message string
}

var skipDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true,
	"build": true, "target": true, "out": true,
	".gradle": true, ".bloop": true, ".metals": true, ".bsp": true,
	"node_modules": true, "vendor": true,
}

func main() {
	var (
		lang    = flag.String("lang", "", "lua, rust, kotlin, or scala")
		root    = flag.String("root", "", "project root")
		budget  = flag.Duration("budget", 10*time.Second, "single-file budget")
		out     = flag.String("out", "", "optional TSV output path")
		exclude = flag.String("exclude", "", "comma-separated path substrings to skip")
	)
	flag.Parse()
	excluded := make([]string, 0)
	for _, item := range strings.Split(*exclude, ",") {
		if item = strings.TrimSpace(item); item != "" {
			excluded = append(excluded, item)
		}
	}
	if *lang == "" || *root == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/ssa-lang-corpus -lang <lua|rust|kotlin|scala> -root <dir> [-budget 10s] [-out result.tsv]")
		os.Exit(2)
	}
	parse, exts, err := languageParser(*lang)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var files []string
	err = filepath.Walk(*root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if skipDirs[info.Name()] && path != *root {
				return filepath.SkipDir
			}
			return nil
		}
		for _, pattern := range excluded {
			if strings.Contains(path, pattern) {
				return nil
			}
		}
		for _, ext := range exts {
			if strings.EqualFold(filepath.Ext(path), ext) {
				files = append(files, path)
				break
			}
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	sort.Strings(files)

	results := make([]fileResult, 0, len(files))
	for _, path := range files {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			results = append(results, fileResult{path: path, status: "READ_FAIL", message: readErr.Error()})
			continue
		}
		start := time.Now()
		var parseErr error
		var panicValue any
		func() {
			defer func() { panicValue = recover() }()
			if strings.EqualFold(*lang, "kotlin") && strings.EqualFold(filepath.Ext(path), ".kts") {
				_, parseErr = kotlin2ssa.ScriptFrontend(string(raw))
				return
			}
			_, parseErr = parse(string(raw))
		}()
		elapsed := time.Since(start)
		status := "PASS"
		message := ""
		switch {
		case panicValue != nil:
			status = "PANIC"
			message = fmt.Sprint(panicValue)
		case parseErr != nil:
			status = "FAIL"
			message = strings.ReplaceAll(parseErr.Error(), "\n", " ")
		case elapsed > *budget:
			status = "TIMEOUT"
			message = fmt.Sprintf("elapsed=%s budget=%s", elapsed, *budget)
		}
		results = append(results, fileResult{path: path, status: status, elapsed: elapsed, message: message})
	}

	var outFile *os.File
	if *out != "" {
		outFile, err = os.Create(*out)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		defer outFile.Close()
	}
	for _, result := range results {
		line := fmt.Sprintf("%s\t%s\t%d\t%s\t%s\n", *lang, result.status, result.elapsed.Milliseconds(), result.path, result.message)
		fmt.Print(line)
		if outFile != nil {
			_, _ = outFile.WriteString(line)
		}
	}

	counts := map[string]int{}
	durations := make([]time.Duration, 0, len(results))
	for _, result := range results {
		counts[result.status]++
		durations = append(durations, result.elapsed)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	percentile := func(p float64) time.Duration {
		if len(durations) == 0 {
			return 0
		}
		index := int(float64(len(durations)-1) * p)
		return durations[index]
	}
	var total time.Duration
	for _, d := range durations {
		total += d
	}
	fmt.Fprintf(os.Stderr, "SUMMARY\t%s\tfiles=%d\tpass=%d\tfail=%d\tpanic=%d\ttimeout=%d\tp50=%s\tp95=%s\ttotal=%s\n",
		*lang, len(results), counts["PASS"], counts["FAIL"], counts["PANIC"], counts["TIMEOUT"],
		percentile(0.50), percentile(0.95), total)
}

func languageParser(lang string) (parseFunc, []string, error) {
	switch strings.ToLower(lang) {
	case "lua":
		return func(src string, caches ...*ssa.AntlrCache) (any, error) {
			return lua2ssa.Frontend(src, caches...)
		}, []string{".lua"}, nil
	case "rust":
		return func(src string, caches ...*ssa.AntlrCache) (any, error) {
			return rust2ssa.Frontend(src, caches...)
		}, []string{".rs"}, nil
	case "kotlin":
		return func(src string, caches ...*ssa.AntlrCache) (any, error) {
			return kotlin2ssa.Frontend(src, caches...)
		}, []string{".kt", ".kts"}, nil
	case "scala":
		return func(src string, caches ...*ssa.AntlrCache) (any, error) {
			return scala2ssa.Frontend(src, caches...)
		}, []string{".scala"}, nil
	default:
		return nil, nil, fmt.Errorf("unsupported language %q", lang)
	}
}
