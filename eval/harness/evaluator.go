package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// GroundTruth defines the expected vulnerabilities for a CVE case.
type GroundTruth struct {
	CVEID       string      `json:"cve_id"`
	ProjectURL  string      `json:"project_url"`
	CommitHash  string      `json:"commit_hash"` // vulnerable version
	Description string      `json:"description"`
	Language    string      `json:"language"` // e.g. "golang", "java", "python"
	Vulns       []VulnEntry `json:"vulns"`
}

// VulnEntry is a single known vulnerability in the ground truth.
type VulnEntry struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"` // e.g. "sqli", "xss", "rce", "ssrf"
	File        string   `json:"file"` // source file path
	Line        int      `json:"line"` // approximate line number
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"` // keywords to match in AI output
}

// EvalResult is the complete evaluation output.
type EvalResult struct {
	CaseID          string           `json:"case_id"`
	CVEID           string           `json:"cve_id"`
	Model           string           `json:"model"`
	CoordinatorID   string           `json:"coordinator_id"`
	Timestamp       time.Time        `json:"timestamp"`
	Duration        time.Duration    `json:"duration"`
	Metrics         Metrics          `json:"metrics"`
	VulnMatches     []VulnMatch      `json:"vuln_matches"`
	ReasoningEvents int              `json:"reasoning_events"`
	ToolCalls       int              `json:"tool_calls"`
	Errors          int              `json:"errors"`
	FinalAnswer     string           `json:"final_answer"`
	SubtaskMetrics  []SubtaskMetrics `json:"subtask_metrics,omitempty"`
}

// Metrics holds computed evaluation metrics.
type Metrics struct {
	Recall           float64 `json:"recall"`              // TP / (TP + FN)
	Precision        float64 `json:"precision"`           // TP / (TP + FP)
	FPR              float64 `json:"false_positive_rate"` // FP / (FP + TN)
	F1Score          float64 `json:"f1_score"`
	ReasoningQuality float64 `json:"reasoning_quality"` // 0-1 score
	DurationSeconds  float64 `json:"duration_seconds"`
	EventCount       int     `json:"event_count"`
	ThoughtCount     int     `json:"thought_count"`
	ToolCallCount    int     `json:"tool_call_count"`
	ErrorCount       int     `json:"error_count"`
	InputTokens      int     `json:"input_tokens"`  // estimated
	OutputTokens     int     `json:"output_tokens"` // estimated
	TotalTokens      int     `json:"total_tokens"`  // estimated
}

// VulnMatch tracks how a ground-truth vuln was matched.
type VulnMatch struct {
	GroundTruth VulnEntry `json:"ground_truth"`
	Found       bool      `json:"found"`
	MatchMethod string    `json:"match_method"` // "keyword", "file_path", "type"
	MatchDetail string    `json:"match_detail"`
}

// Evaluate compares AI Agent output against ground truth and computes metrics.
// Optional projectDir enables a more accurate precision heuristic by only
// counting source files under the target project as reported findings.
func Evaluate(gt GroundTruth, taskResult *TaskResult, projectDir ...string) EvalResult {
	result := EvalResult{
		CaseID:          gt.CVEID,
		CVEID:           gt.CVEID,
		Timestamp:       time.Now(),
		Duration:        taskResult.Duration,
		ReasoningEvents: taskResult.ThoughtCount,
		ToolCalls:       taskResult.ToolCallCount,
		Errors:          taskResult.ErrorCount,
		FinalAnswer:     taskResult.FinalAnswer,
		CoordinatorID:   taskResult.CoordinatorID,
		SubtaskMetrics:  taskResult.SubtaskMetrics,
	}

	// Build searchable text only from model-produced findings/results. Tool
	// outputs often contain the vulnerable source code itself; counting those as
	// "found" makes an interrupted reconnaissance run look like a successful
	// audit.
	allText := extractEvaluationText(taskResult.Events)

	matches := make([]VulnMatch, 0, len(gt.Vulns))
	tp := 0
	for _, vuln := range gt.Vulns {
		m := VulnMatch{
			GroundTruth: vuln,
			Found:       false,
		}
		// Check keyword matches.
		for _, kw := range vuln.Keywords {
			if strings.Contains(strings.ToLower(allText), strings.ToLower(kw)) {
				m.Found = true
				m.MatchMethod = "keyword"
				m.MatchDetail = fmt.Sprintf("matched keyword: %q", kw)
				break
			}
		}
		// Check file path match.
		if !m.Found && vuln.File != "" {
			if strings.Contains(strings.ToLower(allText), strings.ToLower(vuln.File)) {
				m.Found = true
				m.MatchMethod = "file_path"
				m.MatchDetail = fmt.Sprintf("matched file: %q", vuln.File)
			}
		}
		if m.Found {
			tp++
		}
		matches = append(matches, m)
	}

	result.VulnMatches = matches

	// Compute metrics.
	total := len(gt.Vulns)
	if total > 0 {
		result.Metrics.Recall = float64(tp) / float64(total)
	}
	// Precision requires counting false positives (reported vulns not in ground truth).
	// When projectDir is provided, count distinct source files under the target project
	// that are mentioned together with vulnerability indicators. Otherwise fall back to
	// the legacy keyword heuristic.
	var projectRoot string
	if len(projectDir) > 0 {
		projectRoot = projectDir[0]
	}
	reportedVulns := countReportedFindings(allText, projectRoot)
	if reportedVulns > 0 {
		denominator := reportedVulns
		if tp > denominator {
			denominator = tp
		}
		result.Metrics.Precision = float64(tp) / float64(denominator)
	}
	if result.Metrics.Recall+result.Metrics.Precision > 0 {
		result.Metrics.F1Score = 2 * result.Metrics.Recall * result.Metrics.Precision / (result.Metrics.Recall + result.Metrics.Precision)
	}
	result.Metrics.DurationSeconds = taskResult.Duration.Seconds()
	result.Metrics.EventCount = taskResult.EventCount
	result.Metrics.ThoughtCount = taskResult.ThoughtCount
	result.Metrics.ToolCallCount = taskResult.ToolCallCount
	result.Metrics.ErrorCount = taskResult.ErrorCount
	result.Metrics.InputTokens = taskResult.TokenUsage.InputTokens
	result.Metrics.OutputTokens = taskResult.TokenUsage.OutputTokens
	result.Metrics.TotalTokens = taskResult.TokenUsage.TotalTokens

	// Reasoning quality: simple heuristic based on thought/tool ratio and error rate.
	if taskResult.EventCount > 0 {
		thoughtRatio := float64(taskResult.ThoughtCount) / float64(taskResult.EventCount)
		errorRatio := float64(taskResult.ErrorCount) / float64(taskResult.EventCount)
		result.Metrics.ReasoningQuality = clamp(thoughtRatio*0.6+(1-errorRatio)*0.4, 0, 1)
	}

	return result
}

// extractAllText concatenates all event content into a single searchable string.
func extractAllText(events []*ypb.AIOutputEvent) string {
	var sb strings.Builder
	for _, e := range events {
		if len(e.Content) > 0 {
			sb.Write(e.Content)
			sb.WriteByte('\n')
		}
		if len(e.StreamDelta) > 0 {
			sb.Write(e.StreamDelta)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// extractEvaluationText concatenates only the parts of the event stream that
// represent the agent's own security findings or final conclusions.
func extractEvaluationText(events []*ypb.AIOutputEvent) string {
	var sb strings.Builder
	for _, e := range events {
		if e == nil {
			continue
		}
		if e.Type == "structured" {
			switch e.NodeId {
			case "result", "final", "code_audit_finding":
				sb.Write(e.Content)
				sb.WriteByte('\n')
				continue
			}
			if e.NodeId == "timeline_item" {
				entryType := strings.ToLower(gjson.GetBytes(e.Content, "entry_type").String())
				content := string(e.Content)
				lower := strings.ToLower(content)
				if strings.Contains(entryType, "finding") ||
					strings.Contains(lower, "add_finding") ||
					strings.Contains(lower, "conclude_finding") ||
					strings.Contains(lower, "verified_vuln") {
					sb.WriteString(content)
					sb.WriteByte('\n')
				}
			}
		}
		if e.Type == "memory_build" {
			content := string(e.Content)
			if strings.Contains(strings.ToLower(content), "finding") {
				sb.WriteString(content)
				sb.WriteByte('\n')
			}
		}
	}
	return sb.String()
}

// ExtractFindings extracts vulnerability findings from AI Agent events.
// Looks for evidence_ops, verify, and memory_build events that contain findings.
func ExtractFindings(events []*ypb.AIOutputEvent) []string {
	var findings []string
	for _, e := range events {
		if e.Type == "structured" && e.NodeId == "timeline_item" {
			content := string(e.Content)
			if strings.Contains(content, "evidence_ops") ||
				strings.Contains(content, "verify") ||
				strings.Contains(content, "add_finding") ||
				strings.Contains(content, "finding") {
				findings = append(findings, content)
			}
		}
		if e.Type == "memory_build" {
			findings = append(findings, string(e.Content))
		}
	}
	return findings
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// vulnIndicatorKeywords is used by countReportedFindings to decide whether a
// mentioned source file is likely a reported finding.
var vulnIndicatorKeywords = []string{
	"vulnerability", "vuln", "injection", "command injection", "sql injection",
	"code injection", "xss", "ssrf", "rce", "overflow", "traversal",
	"exec.Command", "exec.CommandContext", "osascript", "bcrypt",
	"unsafe", "sink", "source", "taint", "dataflow",
}

// countReportedFindings estimates the number of reported vulnerabilities.
//
// When projectRoot is provided, it extracts absolute source-file paths under the
// target project that appear near vulnerability keywords. This avoids the old
// behavior of counting every keyword occurrence in the entire event stream,
// which produced near-zero precision on long runs.
//
// When projectRoot is empty, it falls back to the legacy keyword-count
// heuristic for backwards compatibility.
func countReportedFindings(text, projectRoot string) int {
	lower := strings.ToLower(text)

	// Legacy fallback: no project root available.
	if projectRoot == "" {
		count := 0
		for _, kw := range vulnIndicatorKeywords {
			count += strings.Count(lower, kw)
		}
		if count == 0 {
			count = 1
		}
		return count
	}

	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		absRoot = projectRoot
	}

	// Match absolute paths under the project root. The regex is permissive so
	// that file paths with various trailing punctuation are captured.
	escaped := regexp.QuoteMeta(absRoot)
	re := regexp.MustCompile(escaped + `/([^\s:"'<>|&;,$]+)`)

	reportedFiles := make(map[string]struct{})
	for _, m := range re.FindAllString(text, -1) {
		p := strings.TrimRight(m, ":;,.!?'") // clean trailing punctuation
		if !looksLikeSourceFile(p) {
			continue
		}
		// Require a vulnerability indicator somewhere near the path (within the
		// same match is enough; the surrounding text is checked for context).
		start := maxInt(strings.Index(text, p)-200, 0)
		end := minInt(strings.Index(text, p)+len(p)+200, len(text))
		ctx := strings.ToLower(text[start:end])
		if hasVulnIndicator(ctx) {
			reportedFiles[p] = struct{}{}
		}
	}

	if len(reportedFiles) == 0 {
		return 1
	}
	return len(reportedFiles)
}

func looksLikeSourceFile(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	sourceExts := map[string]bool{
		".go": true, ".java": true, ".kt": true, ".scala": true,
		".py": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
		".c": true, ".cc": true, ".cpp": true, ".cxx": true, ".h": true,
		".cs": true, ".php": true, ".rb": true, ".rs": true, ".swift": true,
		".m": true, ".mm": true,
	}
	return sourceExts[ext]
}

func hasVulnIndicator(text string) bool {
	lower := strings.ToLower(text)
	for _, kw := range vulnIndicatorKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// LoadGroundTruth reads a ground truth JSON file.
func LoadGroundTruth(path string) (GroundTruth, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return GroundTruth{}, err
	}
	var gt GroundTruth
	err = json.Unmarshal(data, &gt)
	return gt, err
}
