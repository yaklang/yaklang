package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"
)

type profileConfig struct {
	Name            string
	Requests        int
	Concurrency     int
	BodyBytes       int
	SeedRows        int
	Repetitions     int
	QueryHz         int
	QuerySamples    int
	ScenarioTimeout time.Duration
	BenchTime       string
	BenchCount      int
}

type captureOptions struct {
	backendDir     string
	frontendDir    string
	outputPath     string
	artifactDir    string
	gomaxprocs     int
	memoryLimitGiB int64
	includeRace    bool
	skipFrontend   bool
	skipBenchmarks bool
	profile        profileConfig
}

type commandResult struct {
	command  string
	output   string
	exitCode int
	err      error
}

func runCapture(args []string) error {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}
	fs := newFlagSet("capture")
	profileName := fs.String("profile", "smoke", "smoke, standard, or stress")
	outputPath := fs.String("out", "", "output JSON report (default: reports/mitm-perf/<timestamp>/report.json)")
	frontendDir := fs.String("frontend-dir", filepath.Clean(filepath.Join(workingDirectory, "../../ts/yakit-plugin-history")), "Yakit frontend worktree")
	gomaxprocs := fs.Int("gomaxprocs", 4, "maximum Go scheduler processors")
	memoryLimitGiB := fs.Int64("memory-limit-gib", 4, "Go memory soft limit in GiB")
	includeRace := fs.Bool("include-race", false, "run the focused MITM V2 race gate")
	skipFrontend := fs.Bool("skip-frontend", false, "skip the Vitest frontend benchmark")
	skipBenchmarks := fs.Bool("skip-benchmarks", false, "skip Go microbenchmarks")
	requests := fs.Int("requests", 0, "override requests per integration scenario")
	concurrency := fs.Int("concurrency", 0, "override integration request concurrency")
	bodyBytes := fs.Int("body-bytes", 0, "override mock response body size")
	seedRows := fs.Int("seed-rows", -1, "override rows pre-seeded into the temporary project DB")
	repetitions := fs.Int("repetitions", 0, "override integration scenario repetitions")
	if err := fs.Parse(args); err != nil {
		return err
	}

	profile, err := resolveProfile(*profileName)
	if err != nil {
		return err
	}
	if *requests > 0 {
		profile.Requests = *requests
	}
	if *concurrency > 0 {
		profile.Concurrency = *concurrency
	}
	if *bodyBytes > 0 {
		profile.BodyBytes = *bodyBytes
	}
	if *seedRows >= 0 {
		profile.SeedRows = *seedRows
	}
	if *repetitions > 0 {
		profile.Repetitions = *repetitions
	}
	if *gomaxprocs < 1 {
		return fmt.Errorf("-gomaxprocs must be positive")
	}
	if *memoryLimitGiB < 1 {
		return fmt.Errorf("-memory-limit-gib must be positive")
	}

	backendDir, err := filepath.Abs(workingDirectory)
	if err != nil {
		return err
	}
	frontDir, err := filepath.Abs(*frontendDir)
	if err != nil {
		return err
	}
	if *outputPath == "" {
		stamp := time.Now().Format("20060102-150405")
		*outputPath = filepath.Join(backendDir, "reports", "mitm-perf", stamp, "report.json")
	}
	out, err := filepath.Abs(*outputPath)
	if err != nil {
		return err
	}
	artifactDir := filepath.Join(filepath.Dir(out), "artifacts")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return err
	}

	options := captureOptions{
		backendDir:     backendDir,
		frontendDir:    frontDir,
		outputPath:     out,
		artifactDir:    artifactDir,
		gomaxprocs:     *gomaxprocs,
		memoryLimitGiB: *memoryLimitGiB,
		includeRace:    *includeRace,
		skipFrontend:   *skipFrontend,
		skipBenchmarks: *skipBenchmarks,
		profile:        profile,
	}

	runtime.GOMAXPROCS(options.gomaxprocs)
	debug.SetMemoryLimit(options.memoryLimitGiB << 30)
	report := newReport(profile.Name)
	report.System["goos"] = runtime.GOOS
	report.System["goarch"] = runtime.GOARCH
	report.System["logical_cpus"] = runtime.NumCPU()
	report.System["gomaxprocs"] = runtime.GOMAXPROCS(0)
	report.System["go_version"] = runtime.Version()
	report.System["memory_limit_gib"] = options.memoryLimitGiB
	report.System["cpu_measurement"] = cpuMeasurementSource
	report.Config["bench_time"] = profile.BenchTime
	report.Config["bench_count"] = profile.BenchCount
	report.Config["scenario_timeout_seconds"] = profile.ScenarioTimeout.Seconds()
	report.Config["race_enabled"] = options.includeRace
	report.Config["frontend_benchmark_enabled"] = !options.skipFrontend
	report.Config["go_microbenchmarks_enabled"] = !options.skipBenchmarks
	report.Revisions["backend"] = gitRevision(options.backendDir)
	report.Revisions["backend_dirty"] = strconv.FormatBool(gitDirty(options.backendDir))
	report.Revisions["backend_state"] = gitStateFingerprint(options.backendDir)
	frontendRevision := gitRevision(options.frontendDir)
	if frontendRevision != "unknown" {
		report.Revisions["frontend"] = frontendRevision
		report.Revisions["frontend_dirty"] = strconv.FormatBool(gitDirty(options.frontendDir))
		report.Revisions["frontend_state"] = gitStateFingerprint(options.frontendDir)
	}

	fmt.Printf("[mitm-perf] integration probe: profile=%s requests=%d concurrency=%d body=%d seed=%d repeats=%d\n",
		profile.Name, profile.Requests, profile.Concurrency, profile.BodyBytes, profile.SeedRows, profile.Repetitions)
	probeErr := runProbe(report, profile)
	if probeErr != nil {
		report.addCheck("probe.completed", "fail", "mitm_pipeline", probeErr.Error())
	} else {
		report.addCheck("probe.completed", "pass", "mitm_pipeline", "")
	}

	if !options.skipBenchmarks {
		runGoBenchmarks(report, options)
	} else {
		report.addCheck("go_microbenchmarks", "skipped", "benchmark", "disabled by -skip-benchmarks")
	}
	if !options.skipFrontend {
		runFrontendBenchmarks(report, options)
	} else {
		report.addCheck("frontend_benchmark", "skipped", "frontend_refresh", "disabled by -skip-frontend")
	}
	if options.includeRace {
		runRaceGate(report, options)
	} else {
		report.addCheck("race.mitmv2_query_write", "skipped", "concurrency", "run capture with -include-race")
	}

	if err := writeReport(options.outputPath, report); err != nil {
		return err
	}
	fmt.Printf("[mitm-perf] report: %s\n", options.outputPath)
	if probeErr != nil {
		return fmt.Errorf("integration probe failed: %w", probeErr)
	}
	return nil
}

func resolveProfile(name string) (profileConfig, error) {
	switch name {
	case "smoke":
		return profileConfig{
			Name: "smoke", Requests: 40, Concurrency: 4, BodyBytes: 32 * 1024, SeedRows: 1000,
			Repetitions: 1, QueryHz: 2, QuerySamples: 3, ScenarioTimeout: 30 * time.Second,
			BenchTime: "100ms", BenchCount: 1,
		}, nil
	case "standard":
		return profileConfig{
			Name: "standard", Requests: 200, Concurrency: 8, BodyBytes: 64 * 1024, SeedRows: 20_000,
			Repetitions: 3, QueryHz: 2, QuerySamples: 10, ScenarioTimeout: 60 * time.Second,
			BenchTime: "500ms", BenchCount: 3,
		}, nil
	case "stress":
		return profileConfig{
			Name: "stress", Requests: 1000, Concurrency: 16, BodyBytes: 64 * 1024, SeedRows: 100_000,
			Repetitions: 3, QueryHz: 2, QuerySamples: 20, ScenarioTimeout: 2 * time.Minute,
			BenchTime: "1s", BenchCount: 5,
		}, nil
	default:
		return profileConfig{}, fmt.Errorf("unknown profile %q", name)
	}
}

func runGoBenchmarks(r *report, options captureOptions) {
	benchmarks := []struct {
		name           string
		area           string
		packageArg     string
		pattern        string
		minimumMetrics int
	}{
		{
			name: "trafficguard", area: "trafficguard", packageArg: "./common/crep/trafficguard",
			pattern: `^BenchmarkScan(Clean32K|Clean256K|Hit32K|HTTPFlowReq32K|Clean1M)$`, minimumMetrics: 20,
		},
		{
			name: "replacer", area: "plugin_replacer", packageArg: "./common/yakgrpc/yakit",
			pattern: `^BenchmarkMITMReplacerColorPath$`, minimumMetrics: 6,
		},
	}
	allPassed := true
	for _, benchmark := range benchmarks {
		args := []string{
			"test", benchmark.packageArg, "-run", "^$", "-bench", benchmark.pattern, "-benchmem",
			"-benchtime", options.profile.BenchTime, "-count", strconv.Itoa(options.profile.BenchCount),
			"-cpu", strconv.Itoa(options.gomaxprocs),
		}
		artifact := filepath.Join(options.artifactDir, "go-bench-"+benchmark.name+".txt")
		result := runExternalCommand(options.backendDir, options.commandEnv(), "go", args...)
		_ = os.WriteFile(artifact, []byte(result.output), 0o644)
		r.Artifacts = append(r.Artifacts, commandArtifact{
			Name: benchmark.name, Command: result.command, ExitCode: result.exitCode, OutputFile: artifact,
		})
		if result.exitCode != 0 {
			allPassed = false
			r.addCheck("benchmark."+benchmark.name, "fail", benchmark.area, fmt.Sprintf("exit=%d; see %s", result.exitCode, artifact))
			continue
		}
		metrics := parseGoBenchmarkOutput(result.output, benchmark.name, benchmark.area)
		if len(metrics) < benchmark.minimumMetrics {
			allPassed = false
			r.addCheck("benchmark."+benchmark.name, "fail", benchmark.area,
				fmt.Sprintf("parsed %d metrics, expected at least %d", len(metrics), benchmark.minimumMetrics))
			continue
		}
		r.addCheck("benchmark."+benchmark.name, "pass", benchmark.area, "")
		for _, item := range metrics {
			r.addMetric(item)
		}
	}
	r.addCheck("go_microbenchmarks", passFail(allPassed), "benchmark", "")
}

func runFrontendBenchmarks(r *report, options captureOptions) {
	benchmarkFile := filepath.Join(options.frontendDir,
		"app/renderer/src/main/src/pages/mitm/MITMManual/__test__/MITMManual.perf.bench.ts")
	vitest := filepath.Join(options.frontendDir, "node_modules", ".bin", "vitest")
	if !fileExists(benchmarkFile) || !fileExists(vitest) {
		r.addCheck("frontend_benchmark", "skipped", "frontend_refresh", "benchmark file or node_modules/.bin/vitest is missing")
		return
	}
	jsonOutput := filepath.Join(options.artifactDir, "frontend-benchmark.json")
	textOutput := filepath.Join(options.artifactDir, "frontend-benchmark.txt")
	args := []string{
		"bench", benchmarkFile, "--run", "--single-thread", "--reporter=json", "--outputFile=" + jsonOutput,
	}
	result := runExternalCommand(options.frontendDir, options.commandEnv(), vitest, args...)
	_ = os.WriteFile(textOutput, []byte(result.output), 0o644)
	r.Artifacts = append(r.Artifacts, commandArtifact{
		Name: "frontend", Command: result.command, ExitCode: result.exitCode, OutputFile: textOutput,
	})
	if result.exitCode != 0 {
		r.addCheck("frontend_benchmark", "fail", "frontend_refresh", fmt.Sprintf("exit=%d; see %s", result.exitCode, textOutput))
		return
	}
	metrics, err := parseVitestBenchmarkReport(jsonOutput)
	if err != nil {
		r.addCheck("frontend_benchmark", "fail", "frontend_refresh", err.Error())
		return
	}
	if len(metrics) < 12 {
		r.addCheck("frontend_benchmark", "fail", "frontend_refresh", fmt.Sprintf("parsed %d metrics, expected at least 12", len(metrics)))
		return
	}
	for _, item := range metrics {
		r.addMetric(item)
	}
	r.addCheck("frontend_benchmark", "pass", "frontend_refresh", "")
}

func runRaceGate(r *report, options captureOptions) {
	artifact := filepath.Join(options.artifactDir, "race-mitmv2-query-write.txt")
	args := []string{
		"test", "-p=1", "-race", "./common/yakgrpc",
		"-run", `^TestQueryHTTPFlowsConcurrentWrite$`, "-count=1", "-timeout=90s",
	}
	env := append(options.commandEnv(), "MOCKEY_CHECK_GCFLAGS=false")
	result := runExternalCommandWithTimeout(20*time.Minute, options.backendDir, env, "go", args...)
	_ = os.WriteFile(artifact, []byte(result.output), 0o644)
	r.Artifacts = append(r.Artifacts, commandArtifact{
		Name: "race_mitmv2_query_write", Command: result.command, ExitCode: result.exitCode, OutputFile: artifact,
	})
	status := "pass"
	detail := ""
	if result.exitCode != 0 || strings.Contains(result.output, "WARNING: DATA RACE") {
		status = "fail"
		detail = fmt.Sprintf("exit=%d data_race=%t; see %s", result.exitCode,
			strings.Contains(result.output, "WARNING: DATA RACE"), artifact)
	}
	r.addCheck("race.mitmv2_query_write", status, "concurrency", detail)
}

func (o captureOptions) commandEnv() []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env,
		"GOMAXPROCS="+strconv.Itoa(o.gomaxprocs),
		"GOMEMLIMIT="+strconv.FormatInt(o.memoryLimitGiB, 10)+"GiB",
		"CI=1",
		"NO_COLOR=1",
		"MITM_PERF_PROFILE="+o.profile.Name,
	)
	return env
}

func runExternalCommand(directory string, env []string, name string, args ...string) commandResult {
	return runExternalCommandWithTimeout(10*time.Minute, directory, env, name, args...)
}

func runExternalCommandWithTimeout(timeout time.Duration, directory string, env []string, name string, args ...string) commandResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = directory
	command.Env = env
	raw, err := command.CombinedOutput()
	exitCode := 0
	if err != nil {
		exitCode = 1
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		}
	}
	if ctx.Err() != nil {
		err = ctx.Err()
		exitCode = 124
	}
	return commandResult{
		command: strings.Join(append([]string{name}, args...), " "),
		output:  string(raw), exitCode: exitCode, err: err,
	}
}

type metricSamples struct {
	template metric
	values   []float64
}

var benchmarkCPUSuffix = regexp.MustCompile(`-\d+$`)

func parseGoBenchmarkOutput(output, suite, area string) []metric {
	collected := make(map[string]*metricSamples)
	scanner := bufio.NewScanner(strings.NewReader(output))
	pendingBenchmark := ""
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		if strings.HasPrefix(fields[0], "Benchmark") {
			pendingBenchmark = benchmarkCPUSuffix.ReplaceAllString(fields[0], "")
		}
		if pendingBenchmark == "" {
			continue
		}
		parsedMeasurement := false
		for index := 1; index < len(fields); index++ {
			unit := fields[index]
			direction := directionNeutral
			switch unit {
			case "ns/op", "B/op", "allocs/op":
				direction = directionLower
			case "MB/s":
				direction = directionHigher
			default:
				continue
			}
			value, err := strconv.ParseFloat(fields[index-1], 64)
			if err != nil {
				continue
			}
			key := "go." + sanitizeMetricPart(suite) + "." + sanitizeMetricPart(pendingBenchmark) + "." + sanitizeMetricPart(unit)
			entry := collected[key]
			if entry == nil {
				entry = &metricSamples{template: metric{Name: key, Unit: unit, Direction: direction, Area: area}}
				collected[key] = entry
			}
			entry.values = append(entry.values, value)
			parsedMeasurement = true
		}
		if parsedMeasurement {
			pendingBenchmark = ""
		}
	}
	metrics := make([]metric, 0, len(collected))
	for _, entry := range collected {
		item := entry.template
		item.Value = median(entry.values)
		metrics = append(metrics, item)
	}
	return metrics
}

func parseVitestBenchmarkReport(path string) ([]metric, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload struct {
		TestResults map[string][]struct {
			Name string  `json:"name"`
			Hz   float64 `json:"hz"`
			Mean float64 `json:"mean"`
			P99  float64 `json:"p99"`
		} `json:"testResults"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	var metrics []metric
	for suite, results := range payload.TestResults {
		for _, result := range results {
			prefix := "frontend." + sanitizeMetricPart(suite) + "." + sanitizeMetricPart(result.Name)
			metrics = append(metrics,
				metric{Name: prefix + ".mean_ms", Value: result.Mean, Unit: "ms/op", Direction: directionLower, Area: "frontend_manual"},
				metric{Name: prefix + ".p99_ms", Value: result.P99, Unit: "ms/op", Direction: directionLower, Area: "frontend_manual"},
				metric{Name: prefix + ".throughput", Value: result.Hz, Unit: "ops/s", Direction: directionHigher, Area: "frontend_manual"},
			)
		}
	}
	if len(metrics) == 0 {
		return nil, fmt.Errorf("Vitest benchmark report contains no results: %s", path)
	}
	return metrics, nil
}

func sanitizeMetricPart(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	lastUnderscore := false
	for _, char := range value {
		valid := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9'
		if valid {
			builder.WriteRune(char)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(builder.String(), "_")
}

func gitRevision(directory string) string {
	result := runExternalCommand(directory, os.Environ(), "git", "rev-parse", "--short", "HEAD")
	if result.exitCode != 0 {
		return "unknown"
	}
	return strings.TrimSpace(result.output)
}

func gitDirty(directory string) bool {
	result := runExternalCommand(directory, os.Environ(), "git", "status", "--porcelain")
	return result.exitCode == 0 && strings.TrimSpace(result.output) != ""
}

func gitStateFingerprint(directory string) string {
	diff := runExternalCommand(directory, os.Environ(), "git", "diff", "--binary", "HEAD", "--")
	if diff.exitCode != 0 {
		return "unknown"
	}
	untracked := runExternalCommand(directory, os.Environ(), "git", "ls-files", "--others", "--exclude-standard", "-z")
	if untracked.exitCode != 0 {
		return "unknown"
	}
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("tracked\x00"))
	_, _ = hasher.Write([]byte(diff.output))
	paths := strings.Split(untracked.output, "\x00")
	sort.Strings(paths)
	for _, relativePath := range paths {
		if relativePath == "" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(relativePath)))
		if err != nil {
			return "unknown"
		}
		_, _ = hasher.Write([]byte("untracked\x00" + relativePath + "\x00"))
		_, _ = hasher.Write(raw)
	}
	return fmt.Sprintf("%x", hasher.Sum(nil))[:16]
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
