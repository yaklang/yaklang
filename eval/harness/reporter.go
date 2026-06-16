package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Report writes evaluation results to JSON and Markdown files.
func Report(result EvalResult, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outputDir, err)
	}

	// Write JSON report.
	jsonPath := filepath.Join(outputDir, fmt.Sprintf("%s_%s.json", result.CaseID, time.Now().Format("20060102_150405")))
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
		return fmt.Errorf("write json: %w", err)
	}

	// Write Markdown summary.
	mdPath := filepath.Join(outputDir, fmt.Sprintf("%s_%s.md", result.CaseID, time.Now().Format("20060102_150405")))
	md := formatMarkdown(result)
	if err := os.WriteFile(mdPath, []byte(md), 0644); err != nil {
		return fmt.Errorf("write markdown: %w", err)
	}

	fmt.Printf("Report written: %s\n  JSON: %s\n  MD:   %s\n", result.CaseID, jsonPath, mdPath)
	return nil
}

func formatMarkdown(r EvalResult) string {
	m := r.Metrics
	s := fmt.Sprintf(`# Eval Report: %s

**CVE**: %s | **Model**: %s | **Date**: %s  
**Duration**: %.1fs | **Session**: %s

## Metrics

| Metric | Value |
|--------|-------|
| Recall | %.2f%% |
| Precision | %.2f%% |
| F1 Score | %.2f%% |
| FPR | %.2f%% |
| Reasoning Quality | %.2f |
| Events | %d |
| Thoughts | %d |
| Tool Calls | %d |
| Errors | %d |
| Input Tokens (est.) | %d |
| Output Tokens (est.) | %d |
| Total Tokens (est.) | %d |

## Vulnerability Matches

`, r.CVEID, r.CVEID, r.Model, r.Timestamp.Format(time.RFC3339),
		m.DurationSeconds, r.CoordinatorID,
		m.Recall*100, m.Precision*100, m.F1Score*100, m.FPR*100,
		m.ReasoningQuality, m.EventCount, m.ThoughtCount, m.ToolCallCount, m.ErrorCount,
		m.InputTokens, m.OutputTokens, m.TotalTokens)

	for _, v := range r.VulnMatches {
		status := "NOT FOUND"
		if v.Found {
			status = fmt.Sprintf("FOUND (%s: %s)", v.MatchMethod, v.MatchDetail)
		}
		s += fmt.Sprintf("- **%s** [%s] %s: %s — %s\n",
			v.GroundTruth.ID, v.GroundTruth.Type, v.GroundTruth.File, v.GroundTruth.Description, status)
	}

	if r.FinalAnswer != "" {
		s += fmt.Sprintf("\n## Final Answer (truncated)\n\n```\n%s\n```\n", truncate(r.FinalAnswer, 2000))
	}

	return s
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}
