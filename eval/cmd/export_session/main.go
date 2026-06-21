package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"github.com/yaklang/yaklang/eval/harness"
)

func main() {
	var (
		addr        = flag.String("addr", "127.0.0.1:8087", "yaklang gRPC server address")
		coordinator = flag.String("coordinator", "", "Coordinator ID to export")
		casePath    = flag.String("case", "", "Optional ground truth JSON for evaluation")
		projectPath = flag.String("project", "", "Optional project directory for precision heuristics")
		outputDir   = flag.String("output", "results", "Output directory")
		model       = flag.String("model", "", "Model name to record in report")
	)
	flag.Parse()

	if *coordinator == "" {
		fmt.Fprintln(os.Stderr, "Error: --coordinator is required")
		os.Exit(1)
	}

	client, err := harness.NewClient(*addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot connect to gRPC at %s: %v\n", *addr, err)
		os.Exit(1)
	}
	defer client.Close()

	ctx := context.Background()
	events, err := harness.QueryEvents(ctx, client, *coordinator)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Query events failed: %v\n", err)
		os.Exit(1)
	}
	if len(events) == 0 {
		fmt.Fprintf(os.Stderr, "No events found for coordinator %s\n", *coordinator)
		os.Exit(1)
	}

	logDir := filepath.Join(*outputDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Create log dir failed: %v\n", err)
		os.Exit(1)
	}

	base := fmt.Sprintf("%s_%s", *coordinator, time.Now().Format("20060102_150405"))
	rawPath := filepath.Join(logDir, base+"_raw.json")
	if err := harness.SaveEventsJSON(events, rawPath); err != nil {
		fmt.Fprintf(os.Stderr, "Save events failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Raw events: %s (%d events)\n", rawPath, len(events))

	zipPath := filepath.Join(logDir, base+".zip")
	if exported, err := harness.ExportLogs(ctx, client, *coordinator, zipPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: export logs failed: %v\n", err)
	} else {
		fmt.Printf("Logs: %s\n", exported)
	}

	if *casePath == "" {
		return
	}
	gt, err := harness.LoadGroundTruth(*casePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load ground truth failed: %v\n", err)
		os.Exit(1)
	}

	taskResult := buildTaskResult(*coordinator, events)
	result := harness.Evaluate(gt, taskResult, *projectPath)
	result.Model = *model
	if err := harness.Report(result, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Write report failed: %v\n", err)
		os.Exit(1)
	}
}

func buildTaskResult(coordinator string, events []*ypb.AIOutputEvent) *harness.TaskResult {
	result := &harness.TaskResult{
		CoordinatorID: coordinator,
		Events:        events,
	}
	for _, e := range events {
		if e == nil || e.Timestamp == 0 {
			continue
		}
		ts := time.Unix(e.Timestamp, 0)
		if result.StartTime.IsZero() || ts.Before(result.StartTime) {
			result.StartTime = ts
		}
		if ts.After(result.EndTime) {
			result.EndTime = ts
		}
	}
	if !result.StartTime.IsZero() && !result.EndTime.IsZero() {
		result.Duration = result.EndTime.Sub(result.StartTime)
	}
	result.RecomputeStats()
	result.SubtaskMetrics = harness.ComputeSubtaskMetrics(events)
	return result
}
