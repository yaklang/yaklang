package codeaudit

import (
	"encoding/json"
	"testing"
)

func TestReport_JSON(t *testing.T) {
	report := NewReport("codeaudit/test", "/path/to/project", "java")
	report.AddFinding("test.finding", "high", "Test Finding", "test recommendation",
		[]Evidence{{File: "/path/to/file.java", Line: 10, Snippet: "password = secret"}})
	report.Summary = "test summary"
	report.Artifacts = map[string]any{"key": "value"}
	report.Meta = Meta{DurationMs: 100, FilesScanned: 5}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("failed to marshal report: %v", err)
	}

	var parsed Report
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	if parsed.Tool != "codeaudit/test" {
		t.Errorf("expected tool 'codeaudit/test', got %q", parsed.Tool)
	}
	if parsed.Target != "/path/to/project" {
		t.Errorf("expected target '/path/to/project', got %q", parsed.Target)
	}
	if parsed.Framework != "java" {
		t.Errorf("expected framework 'java', got %q", parsed.Framework)
	}
	if len(parsed.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(parsed.Findings))
	}
	if parsed.Findings[0].ID != "test.finding" {
		t.Errorf("expected finding id 'test.finding', got %q", parsed.Findings[0].ID)
	}
	if parsed.Findings[0].Evidence[0].Line != 10 {
		t.Errorf("expected line 10, got %d", parsed.Findings[0].Evidence[0].Line)
	}
}

func TestReport_DedupeFindings(t *testing.T) {
	report := NewReport("codeaudit/test", "/test", "java")
	report.AddFinding("rule1", "high", "Finding 1", "rec",
		[]Evidence{{File: "/a.java"}})
	report.AddFinding("rule1", "high", "Finding 1 (dup)", "rec",
		[]Evidence{{File: "/a.java"}})
	report.AddFinding("rule1", "high", "Finding 2", "rec",
		[]Evidence{{File: "/b.java"}})
	report.AddFinding("rule2", "medium", "Finding 3", "rec",
		[]Evidence{{File: "/c.java"}})

	opts := DefaultProbeOptions()
	report.Finish(0, 10, opts)

	// Should have 3 unique findings (rule1+a deduped, rule1+b kept, rule2+c)
	if len(report.Findings) != 3 {
		t.Errorf("expected 3 findings after dedup, got %d", len(report.Findings))
	}
}
