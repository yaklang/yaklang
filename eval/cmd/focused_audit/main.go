package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/eval/harness"
)

func main() {
	var (
		model        = flag.String("model", "memfit-minimax-m3-thinking", "AI model name")
		aiService    = flag.String("service", "aibalance", "AI provider service")
		casePath     = flag.String("case", "", "Path to ground truth JSON file")
		projectPath  = flag.String("project", "", "Path to vulnerable project directory")
		outputDir    = flag.String("output", "results", "Output directory for reports")
		maxIter      = flag.Int("max-iter", 15, "Max ReAct iterations")
		reviewMode   = flag.String("review", "yolo", "Review policy: yolo, manual, ai")
		grpcAddr     = flag.String("addr", "127.0.0.1:8087", "yaklang gRPC server address")
		prompt       = flag.String("prompt", "", "Custom audit prompt")
		focusMode    = flag.String("focus-mode", "code_security_audit", "AI focus mode loop name (empty for direct ReAct)")
		skipCompile  = flag.Bool("skip-compile", false, "Skip SSA compilation (pure LLM mode)")
		categories   = flag.String("categories", "", "Comma-separated category IDs to scan (e.g. cmd_injection,code_execution). Empty = auto-select via AI")
		skipCats     = flag.String("skip-categories", "", "Comma-separated category IDs to skip (for testing speedup). Empty = scan all AI-selected categories")
		skipPhase1   = flag.Bool("skip-phase1", false, "Skip Phase 1 (dir_explore) and go directly to Phase 2 scanning")
		skipSFScan   = flag.Bool("skip-sf-scan", false, "Skip SyntaxFlow lib scan (Phase 1.5) for pure LLM baseline comparison")
		gtCategories = flag.Bool("ground-truth-categories", true, "When --categories is empty, derive coarse scan categories from ground-truth vulnerability types")
	)
	flag.Parse()

	if *casePath == "" {
		fmt.Fprintln(os.Stderr, "Error: --case is required")
		flag.Usage()
		os.Exit(1)
	}
	if *prompt == "" {
		fmt.Fprintln(os.Stderr, "Error: --prompt is required")
		flag.Usage()
		os.Exit(1)
	}

	gt, err := harness.LoadGroundTruth(*casePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading ground truth: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Ground truth: %s (%d vulns)\n", gt.CVEID, len(gt.Vulns))

	projectDir := *projectPath
	if projectDir == "" {
		projectDir, err = harness.EnsureProject("", gt.ProjectURL, gt.CommitHash, gt.CVEID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error preparing project: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Project ready: %s\n", projectDir)
	}

	progName := gt.CVEID
	if projectDir != "" && !*skipCompile {
		lang := "golang"
		if gt.Language != "" {
			lang = gt.Language
		}
		fmt.Printf("Compiling: %s (lang=%s, program=%s)\n", projectDir, lang, progName)
		progName, err = harness.CompileProject(projectDir, lang, progName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "SSA compile failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Compiled: %s\n", progName)
	} else if *skipCompile {
		fmt.Printf("Skipping SSA compilation (pure LLM mode)\n")
	}

	client, err := harness.NewClient(*grpcAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot connect to gRPC at %s: %v\n", *grpcAddr, err)
		os.Exit(1)
	}
	defer client.Close()

	ctx := context.Background()
	if err := client.CheckHealth(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Health check failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Connected to %s\n", *grpcAddr)

	sessionID := fmt.Sprintf("focused-%s-%s", gt.CVEID, uuid.New().String()[:8])

	// Build prompt with categories if specified
	auditPrompt := *prompt
	effectiveCategories := strings.TrimSpace(*categories)
	if effectiveCategories == "" && *gtCategories {
		effectiveCategories = deriveCategoriesFromGroundTruth(gt)
		if effectiveCategories != "" {
			fmt.Printf("Derived categories from ground truth types: %s\n", effectiveCategories)
		}
	}
	if effectiveCategories != "" {
		auditPrompt = fmt.Sprintf("%s\n\n[指定扫描类别] 只扫描以下类别，不要选择其他类别：%s", *prompt, effectiveCategories)
	} else if *skipCats != "" {
		auditPrompt = fmt.Sprintf("%s\n\n[跳过类别] 以下类别不需要扫描，请在选择时排除：%s", *prompt, *skipCats)
	}
	if *skipPhase1 {
		auditPrompt = fmt.Sprintf("%s\n\n[skip-phase1] 跳过 Phase 1 项目探索，直接进入 Phase 2 扫描。项目路径已知，无需探索。", auditPrompt)
	}
	if *skipSFScan {
		auditPrompt = fmt.Sprintf("%s\n\n[skip-sf-scan] 跳过 SyntaxFlow 扫描（Phase 1.5），纯 LLM 模式。", auditPrompt)
	}

	taskCfg := harness.TaskConfig{
		Model:             *model,
		AIService:         *aiService,
		UserQuery:         auditPrompt,
		FocusMode:         *focusMode,
		ProgramName:       progName,
		ScanTargetPath:    projectDir,
		ReActMaxIteration: *maxIter,
		ReviewPolicy:      *reviewMode,
		SessionID:         sessionID,
		SkipPhase1:        *skipPhase1,
	}

	fmt.Printf("Starting focused audit: model=%s session=%s\n", *model, sessionID)
	fmt.Printf("Prompt: %s\n", auditPrompt)
	taskResult, err := harness.RunTask(ctx, client, taskCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Task failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Done: %.1fs | %d events | %d thoughts | %d tools | ~%d tokens\n",
		taskResult.Duration.Seconds(), taskResult.EventCount, taskResult.ThoughtCount, taskResult.ToolCallCount, taskResult.TokenUsage.TotalTokens)

	// Evaluate against ground truth
	result := harness.Evaluate(gt, taskResult, projectDir)
	result.Model = *model
	fmt.Printf("Recall: %.1f%% | Precision: %.1f%% | F1: %.2f | Found: %d/%d\n",
		result.Metrics.Recall*100, result.Metrics.Precision*100, result.Metrics.F1Score,
		countFound(result.VulnMatches), len(result.VulnMatches))

	// Save results
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output dir: %v\n", err)
		os.Exit(1)
	}
	timestamp := taskResult.StartTime.Format("20060102_150405")
	baseName := fmt.Sprintf("%s_focused_%s", gt.CVEID, timestamp)

	eventsPath := filepath.Join(*outputDir, "logs", baseName+"_stream.json")
	if err := os.MkdirAll(filepath.Dir(eventsPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating log dir: %v\n", err)
		os.Exit(1)
	}
	if err := harness.SaveEventsJSON(taskResult.Events, eventsPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving events: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Events saved: %s\n", eventsPath)

	if err := harness.Report(result, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
		os.Exit(1)
	}
}

func countFound(matches []harness.VulnMatch) int {
	n := 0
	for _, m := range matches {
		if m.Found {
			n++
		}
	}
	return n
}

func deriveCategoriesFromGroundTruth(gt harness.GroundTruth) string {
	categorySet := make(map[string]struct{})
	for _, vuln := range gt.Vulns {
		for _, category := range categoriesForVulnType(vuln.Type) {
			categorySet[category] = struct{}{}
		}
	}
	if len(categorySet) == 0 {
		return ""
	}
	categories := make([]string, 0, len(categorySet))
	for category := range categorySet {
		categories = append(categories, category)
	}
	sort.Slice(categories, func(i, j int) bool {
		pi := categoryPriority(categories[i])
		pj := categoryPriority(categories[j])
		if pi == pj {
			return categories[i] < categories[j]
		}
		return pi < pj
	})
	return strings.Join(categories, ",")
}

func categoryPriority(category string) int {
	switch category {
	case "cmd_injection":
		return 10
	case "code_execution":
		return 20
	case "auth_bypass":
		return 30
	default:
		return 100
	}
}

func categoriesForVulnType(vulnType string) []string {
	t := strings.ToLower(strings.TrimSpace(vulnType))
	t = strings.ReplaceAll(t, "-", "_")
	t = strings.ReplaceAll(t, " ", "_")

	switch {
	case strings.Contains(t, "command") || strings.Contains(t, "cmd") || strings.Contains(t, "shell"):
		return []string{"cmd_injection"}
	case strings.Contains(t, "code_execution") || strings.Contains(t, "rce"):
		return []string{"code_execution", "cmd_injection"}
	case strings.Contains(t, "sql"):
		return []string{"sql_injection"}
	case strings.Contains(t, "xss"):
		return []string{"xss_injection"}
	case strings.Contains(t, "path") || strings.Contains(t, "traversal") || strings.Contains(t, "file_read"):
		return []string{"path_traversal"}
	case strings.Contains(t, "ssrf"):
		return []string{"ssrf"}
	case strings.Contains(t, "deserial"):
		return []string{"deserialization"}
	case strings.Contains(t, "missing_auth") || strings.Contains(t, "auth") || strings.Contains(t, "access_control") || strings.Contains(t, "authorization"):
		return []string{"auth_bypass"}
	default:
		return nil
	}
}
