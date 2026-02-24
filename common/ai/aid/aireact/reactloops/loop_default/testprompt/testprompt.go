package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type testCase struct {
	Index   int
	Name    string
	Path    string
	Content string
}

type runResult struct {
	Case      testCase
	Query     string
	StartedAt time.Time
	Duration  time.Duration
	ExitCode  int
	Output    string
	Err       error
}

var queryPattern = regexp.MustCompile(`(?i)^test(\d+)$`)
var filePattern = regexp.MustCompile(`^(\d+)-.*\.txt$`)

func main() {
	query := flag.String("query", "test1", "Run single query, e.g. test1")
	queryAll := flag.Bool("query-all", false, "Run all test prompts")
	reportPath := flag.String("report", "", "Optional markdown report path")
	flag.Parse()

	repoRoot, err := locateRepoRoot()
	if err != nil {
		fatalf("failed to locate repository root: %v", err)
	}

	promptDir := filepath.Join(repoRoot, "common/ai/aid/aireact/reactloops/loop_default/testprompt")
	cases, err := loadTestCases(promptDir)
	if err != nil {
		fatalf("failed to load test prompts: %v", err)
	}
	if len(cases) == 0 {
		fatalf("no test prompt files found in %s", promptDir)
	}

	var targets []testCase
	if *queryAll {
		targets = cases
	} else {
		targets = []testCase{resolveByQuery(*query, cases)}
	}

	results := make([]runResult, 0, len(targets))
	for _, tc := range targets {
		fmt.Printf("\n=== Running %s (%02d) ===\n", tc.Name, tc.Index)
		res := runSingleCase(repoRoot, tc)
		results = append(results, res)
	}

	reportFile := *reportPath
	if strings.TrimSpace(reportFile) == "" {
		reportFile = filepath.Join(promptDir, fmt.Sprintf("test-report-%s.md", time.Now().Format("20060102-150405")))
	}
	if err := writeMarkdownReport(reportFile, results, *queryAll); err != nil {
		fatalf("failed to write report: %v", err)
	}

	fmt.Printf("\nReport generated: %s\n", reportFile)

	for _, res := range results {
		if res.ExitCode != 0 {
			os.Exit(1)
		}
	}
}

func locateRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	cur := wd
	for {
		if isRepoRoot(cur) {
			return cur, nil
		}
		next := filepath.Dir(cur)
		if next == cur {
			break
		}
		cur = next
	}
	return "", errors.New("cannot find repository root containing go.mod and common/yak/cmd/yak.go")
}

func isRepoRoot(dir string) bool {
	goMod := filepath.Join(dir, "go.mod")
	yakCmd := filepath.Join(dir, "common/yak/cmd/yak.go")
	if _, err := os.Stat(goMod); err != nil {
		return false
	}
	if _, err := os.Stat(yakCmd); err != nil {
		return false
	}
	return true
}

func loadTestCases(promptDir string) ([]testCase, error) {
	entries, err := os.ReadDir(promptDir)
	if err != nil {
		return nil, err
	}

	var cases []testCase
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".txt") {
			continue
		}
		matches := filePattern.FindStringSubmatch(name)
		if len(matches) != 2 {
			continue
		}

		idx, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}
		path := filepath.Join(promptDir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s failed: %w", path, err)
		}
		content := strings.TrimSpace(string(raw))
		if content == "" {
			continue
		}
		cases = append(cases, testCase{
			Index:   idx,
			Name:    name,
			Path:    path,
			Content: content,
		})
	}

	sort.Slice(cases, func(i, j int) bool {
		if cases[i].Index == cases[j].Index {
			return cases[i].Name < cases[j].Name
		}
		return cases[i].Index < cases[j].Index
	})
	return cases, nil
}

func resolveByQuery(query string, cases []testCase) testCase {
	trimmed := strings.TrimSpace(query)
	matches := queryPattern.FindStringSubmatch(trimmed)
	if len(matches) != 2 {
		return cases[0]
	}
	n, err := strconv.Atoi(matches[1])
	if err != nil {
		return cases[0]
	}
	for _, tc := range cases {
		if tc.Index == n {
			return tc
		}
	}
	return cases[0]
}

func runSingleCase(repoRoot string, tc testCase) runResult {
	start := time.Now()
	yakCommand := fmt.Sprintf("aim.InvokeReAct(%s)", strconv.Quote(tc.Content))
	cmd := exec.Command("go", "run", "common/yak/cmd/yak.go", "-c", yakCommand)
	cmd.Dir = repoRoot

	var buf bytes.Buffer
	writer := io.MultiWriter(os.Stdout, &buf)
	cmd.Stdout = writer
	cmd.Stderr = writer

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}

	return runResult{
		Case:      tc,
		Query:     fmt.Sprintf("test%d", tc.Index),
		StartedAt: start,
		Duration:  time.Since(start),
		ExitCode:  exitCode,
		Output:    buf.String(),
		Err:       err,
	}
}

func writeMarkdownReport(path string, results []runResult, isAll bool) error {
	if len(results) == 0 {
		return errors.New("empty results")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("# AI ReAct Test Prompt Report\n\n")
	b.WriteString(fmt.Sprintf("- GeneratedAt: %s\n", time.Now().Format(time.RFC3339)))
	if isAll {
		b.WriteString("- Mode: query-all\n")
	} else {
		b.WriteString("- Mode: query\n")
	}
	b.WriteString(fmt.Sprintf("- Total: %d\n\n", len(results)))

	b.WriteString("## Summary\n\n")
	b.WriteString("| Query | File | ExitCode | Duration |\n")
	b.WriteString("| --- | --- | ---: | ---: |\n")
	for _, res := range results {
		b.WriteString(fmt.Sprintf("| %s | %s | %d | %s |\n",
			res.Query,
			res.Case.Name,
			res.ExitCode,
			res.Duration.Truncate(time.Millisecond).String(),
		))
	}
	b.WriteString("\n## Details\n")
	for _, res := range results {
		b.WriteString(fmt.Sprintf("\n### %s - %s\n\n", res.Query, res.Case.Name))
		b.WriteString(fmt.Sprintf("- PromptFile: `%s`\n", res.Case.Path))
		b.WriteString(fmt.Sprintf("- ExitCode: %d\n", res.ExitCode))
		b.WriteString(fmt.Sprintf("- Duration: %s\n", res.Duration.Truncate(time.Millisecond)))
		if res.Err != nil {
			b.WriteString(fmt.Sprintf("- Error: `%v`\n", res.Err))
		}
		b.WriteString("\n#### Prompt\n\n```text\n")
		b.WriteString(res.Case.Content)
		b.WriteString("\n```\n")
		b.WriteString("\n#### Output\n\n```text\n")
		b.WriteString(strings.TrimSpace(res.Output))
		b.WriteString("\n```\n")
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
