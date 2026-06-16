package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

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
		maxIter     = flag.Int("max-iter", 50, "Max ReAct iterations")
		reviewMode  = flag.String("review", "yolo", "Review policy: yolo, manual, ai")
		grpcAddr    = flag.String("addr", "127.0.0.1:8087", "yaklang gRPC server address")
		runs        = flag.Int("runs", 1, "Number of runs for reproducibility measurement")
		skipCompile = flag.Bool("skip-compile", false, "Skip SSA compilation")
		autoClone   = flag.Bool("auto-clone", true, "Auto clone project from ground truth project_url if --project is omitted")
	)
	flag.Parse()

	if *casePath == "" {
		fmt.Fprintln(os.Stderr, "Error: --case is required")
		flag.Usage()
		os.Exit(1)
	}

	gt, err := harness.LoadGroundTruth(*casePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading ground truth: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Ground truth: %s (%d vulns)\n", gt.CVEID, len(gt.Vulns))

	// Ensure project directory: use --project, or auto-clone from ground truth.
	projectDir := *projectPath
	if projectDir == "" && *autoClone {
		projectDir, err = harness.EnsureProject("", gt.ProjectURL, gt.CommitHash, gt.CVEID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error preparing project: %v\n", err)
			fmt.Fprintln(os.Stderr, "Hint: provide --project <dir> or set project_url/commit_hash in ground truth")
			os.Exit(1)
		}
		fmt.Printf("Project ready: %s\n", projectDir)
	}

	// SSA Compilation
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

	// Zero-hint prompt: only generic instruction, no vuln type hints.
	// Project path is passed via AttachedResourceInfo (code_audit_target_path),
	// not in the prompt text, so the AI receives it through the standard interface.
	prompt := "Perform a whitebox security audit on this project."

	var results []harness.EvalResult
	for i := 0; i < *runs; i++ {
		if *runs > 1 {
			fmt.Printf("\n=== Run %d/%d ===\n", i+1, *runs)
		}

		// Each run gets a fresh session ID to isolate memory
		sessionID := fmt.Sprintf("eval-%s-%s", gt.CVEID, uuid.New().String()[:8])

		taskCfg := harness.TaskConfig{
			Model:             *model,
			AIService:         *aiService,
			UserQuery:         prompt,
			FocusMode:         "code_security_audit",
			ProgramName:       progName,
			ScanTargetPath:    projectDir,
			ReActMaxIteration: *maxIter,
			ReviewPolicy:      *reviewMode,
			SessionID:         sessionID,
		}

		fmt.Printf("Starting: focus=code_security_audit model=%s session=%s\n", *model, sessionID)
		taskResult, err := harness.RunTask(ctx, client, taskCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Task failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Done: %.1fs | %d events | %d thoughts | %d tools\n",
			taskResult.Duration.Seconds(), taskResult.EventCount, taskResult.ThoughtCount, taskResult.ToolCallCount)

		if taskResult.CoordinatorID != "" {
			logDir := filepath.Join(*outputDir, "logs")
			os.MkdirAll(logDir, 0755)

			// Save full live stream events (includes reasoning streams omitted by ExportAILogs).
			streamPath := filepath.Join(logDir, fmt.Sprintf("%s_%d_stream.json", gt.CVEID, i+1))
			if err := harness.SaveEventsJSON(taskResult.Events, streamPath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: save stream events failed: %v\n", err)
			} else {
				fmt.Printf("Stream events: %s\n", streamPath)
			}

			// Also query persisted events for comparison/backup.
			rawEvents, err := harness.QueryEvents(ctx, client, taskResult.CoordinatorID)
			if err == nil && len(rawEvents) > 0 {
				rawPath := filepath.Join(logDir, fmt.Sprintf("%s_%d_raw.json", gt.CVEID, i+1))
				if err := harness.SaveEventsJSON(rawEvents, rawPath); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: save raw events failed: %v\n", err)
				} else {
					fmt.Printf("Raw events: %s (%d events)\n", rawPath, len(rawEvents))
				}
			}

			logPath := filepath.Join(logDir, fmt.Sprintf("%s_%d.zip", gt.CVEID, i+1))
			zipPath, err := harness.ExportLogs(ctx, client, taskResult.CoordinatorID, logPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: export logs failed: %v\n", err)
			} else {
				fmt.Printf("Logs: %s\n", zipPath)
			}
		}

		evalResult := harness.Evaluate(gt, taskResult)
		evalResult.Model = *model
		results = append(results, evalResult)

		fmt.Printf("Recall: %.0f%% | Precision: %.0f%% | F1: %.0f%%\n",
			evalResult.Metrics.Recall*100, evalResult.Metrics.Precision*100, evalResult.Metrics.F1Score*100)
		for _, m := range evalResult.VulnMatches {
			status := "NOT FOUND"
			if m.Found {
				status = "FOUND"
			}
			fmt.Printf("  [%s] %s: %s\n", status, m.GroundTruth.ID, m.GroundTruth.Description)
		}

		if err := harness.Report(evalResult, *outputDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: report failed: %v\n", err)
		}
	}

	if *runs > 1 {
		printReproducibilitySummary(results)
	}
}

func printReproducibilitySummary(results []harness.EvalResult) {
	fmt.Println("\n=== Reproducibility Summary ===")
	var recalls, fprs, durations []float64
	for _, r := range results {
		recalls = append(recalls, r.Metrics.Recall)
		fprs = append(fprs, r.Metrics.FPR)
		durations = append(durations, r.Metrics.DurationSeconds)
	}
	fmt.Printf("Recall:    mean=%.0f%% std=%.0f%%\n", mean(recalls)*100, std(recalls)*100)
	fmt.Printf("FPR:       mean=%.0f%% std=%.0f%%\n", mean(fprs)*100, std(fprs)*100)
	fmt.Printf("Duration:  mean=%.1fs std=%.1fs\n", mean(durations), std(durations))
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func std(v []float64) float64 {
	if len(v) < 2 {
		return 0
	}
	m := mean(v)
	s := 0.0
	for _, x := range v {
		d := x - m
		s += d * d
	}
	return s / float64(len(v))
}

var _ = time.Now
