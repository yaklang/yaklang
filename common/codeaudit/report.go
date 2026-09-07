package codeaudit

import (
	"fmt"
	"time"
)

// Report is the unified output structure for all audit tools.
type Report struct {
	Tool      string         `json:"tool"`       // e.g. "codeaudit/probe", "codeaudit/config_audit"
	Target    string         `json:"target"`     // scanned root directory absolute path
	Status    string         `json:"status"`     // "ok" or "partial"
	Framework string         `json:"framework"`  // specific framework name or language
	Summary   string         `json:"summary"`    // human-readable summary
	Findings  []Finding      `json:"findings"`
	Artifacts map[string]any `json:"artifacts"`  // tool-specific output
	Meta      Meta           `json:"meta"`
}

// Finding represents a single audit finding.
type Finding struct {
	ID             string     `json:"id"`              // rule ID, e.g. "spring.actuator.exposed"
	Severity       string     `json:"severity"`        // critical/high/medium/low
	Title          string     `json:"title"`
	Recommendation string     `json:"recommendation"`
	Evidence       []Evidence `json:"evidence"`
}

// Evidence holds the location and snippet of a finding.
type Evidence struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

// Meta holds metadata about the scan.
type Meta struct {
	DurationMs   int64 `json:"duration_ms"`
	FilesScanned int   `json:"files_scanned"`
}

// NewReport creates an initial report.
func NewReport(toolPath, target, framework string) *Report {
	return &Report{
		Tool:      toolPath,
		Target:    target,
		Status:    "ok",
		Framework: framework,
		Findings:  []Finding{},
		Artifacts: map[string]any{},
		Meta:      Meta{DurationMs: 0, FilesScanned: 0},
	}
}

// AddFinding appends a finding to the report.
func (r *Report) AddFinding(id, severity, title, recommendation string, evidence []Evidence) {
	r.Findings = append(r.Findings, Finding{
		ID:             id,
		Severity:       severity,
		Title:          title,
		Recommendation: recommendation,
		Evidence:       evidence,
	})
}

// Finish sets metadata and returns the report.
func (r *Report) Finish(startMs int64, filesScanned int, opts *ProbeOptions) *Report {
	r.Meta.DurationMs = time.Now().UnixMilli() - startMs
	r.Meta.FilesScanned = filesScanned
	if opts != nil && opts.DedupeFindings {
		r.dedupeFindings(opts.MaxFindingsPerRule)
	}
	if r.Summary == "" {
		r.Summary = fmt.Sprintf("completed with %d finding(s)", len(r.Findings))
	}
	return r
}

// dedupeFindings deduplicates findings by rule ID + file path.
func (r *Report) dedupeFindings(maxPerRule int) {
	if maxPerRule <= 0 {
		maxPerRule = 5
	}
	seen := map[string]bool{}
	counts := map[string]int{}
	out := []Finding{}
	for _, f := range r.Findings {
		key := f.ID
		if len(f.Evidence) > 0 {
			key = f.ID + "|" + f.Evidence[0].File
		}
		if seen[key] {
			continue
		}
		if counts[f.ID] >= maxPerRule {
			continue
		}
		counts[f.ID]++
		seen[key] = true
		out = append(out, f)
	}
	r.Findings = out
}
