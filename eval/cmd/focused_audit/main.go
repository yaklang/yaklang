package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/eval/harness"
)

func main() {
	var (
		model       = flag.String("model", "memfit-minimax-m3-thinking", "AI model name")
		aiService   = flag.String("service", "aibalance", "AI provider service")
		casePath    = flag.String("case", "", "Path to ground truth JSON file")
		projectPath = flag.String("project", "", "Path to vulnerable project directory")
		outputDir   = flag.String("output", "results", "Output directory for reports")
		maxIter     = flag.Int("max-iter", 15, "Max ReAct iterations")
		reviewMode  = flag.String("review", "yolo", "Review policy: yolo, manual, ai")
		grpcAddr    = flag.String("addr", "127.0.0.1:8087", "yaklang gRPC server address")
		prompt      = flag.String("prompt", "", "Custom audit prompt")
		focusMode   = flag.String("focus-mode", "code_security_audit", "AI focus mode loop name (empty for direct ReAct)")
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
	if projectDir != "" {
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
	taskCfg := harness.TaskConfig{
		Model:             *model,
		AIService:         *aiService,
		UserQuery:         *prompt,
		FocusMode:         *focusMode,
		ProgramName:       progName,
		ScanTargetPath:    projectDir,
		ReActMaxIteration: *maxIter,
		ReviewPolicy:      *reviewMode,
		SessionID:         sessionID,
	}

	fmt.Printf("Starting focused audit: model=%s session=%s\n", *model, sessionID)
	fmt.Printf("Prompt: %s\n", *prompt)
	taskResult, err := harness.RunTask(ctx, client, taskCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Task failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Done: %.1fs | %d events | %d thoughts | %d tools | ~%d tokens\n",
		taskResult.Duration.Seconds(), taskResult.EventCount, taskResult.ThoughtCount, taskResult.ToolCallCount, taskResult.TokenUsage.TotalTokens)

	// Evaluate against ground truth
	result := harness.Evaluate(gt, taskResult)
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
