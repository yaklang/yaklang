package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const reportSchemaVersion = 1

type metricDirection string

const (
	directionLower   metricDirection = "lower"
	directionHigher  metricDirection = "higher"
	directionNeutral metricDirection = "neutral"
)

type metric struct {
	Name        string          `json:"name"`
	Value       float64         `json:"value"`
	Unit        string          `json:"unit"`
	Direction   metricDirection `json:"direction"`
	Area        string          `json:"area"`
	Description string          `json:"description,omitempty"`
}

type check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Area   string `json:"area"`
	Detail string `json:"detail,omitempty"`
}

type commandArtifact struct {
	Name       string `json:"name"`
	Command    string `json:"command"`
	ExitCode   int    `json:"exit_code"`
	OutputFile string `json:"output_file,omitempty"`
}

type report struct {
	SchemaVersion int               `json:"schema_version"`
	CreatedAt     time.Time         `json:"created_at"`
	Profile       string            `json:"profile"`
	Revisions     map[string]string `json:"revisions"`
	System        map[string]any    `json:"system"`
	Config        map[string]any    `json:"config"`
	Metrics       []metric          `json:"metrics"`
	Checks        []check           `json:"checks"`
	Artifacts     []commandArtifact `json:"artifacts,omitempty"`
}

func newReport(profile string) *report {
	return &report{
		SchemaVersion: reportSchemaVersion,
		CreatedAt:     time.Now(),
		Profile:       profile,
		Revisions:     make(map[string]string),
		System:        make(map[string]any),
		Config:        make(map[string]any),
	}
}

func (r *report) addMetric(m metric) {
	if math.IsNaN(m.Value) || math.IsInf(m.Value, 0) {
		return
	}
	r.Metrics = append(r.Metrics, m)
}

func (r *report) addCheck(name, status, area, detail string) {
	r.Checks = append(r.Checks, check{Name: name, Status: status, Area: area, Detail: detail})
}

func (r *report) normalize() {
	sort.Slice(r.Metrics, func(i, j int) bool { return r.Metrics[i].Name < r.Metrics[j].Name })
	sort.Slice(r.Checks, func(i, j int) bool { return r.Checks[i].Name < r.Checks[j].Name })
}

func writeReport(path string, r *report) error {
	r.normalize()
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return nil
}

func readReport(path string) (*report, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r report
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("decode report %s: %w", path, err)
	}
	if r.SchemaVersion != reportSchemaVersion {
		return nil, fmt.Errorf("unsupported report schema %d in %s", r.SchemaVersion, path)
	}
	return &r, nil
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	if p <= 0 {
		return ordered[0]
	}
	if p >= 1 {
		return ordered[len(ordered)-1]
	}
	index := int(math.Ceil(float64(len(ordered))*p)) - 1
	if index < 0 {
		index = 0
	}
	return ordered[index]
}

func median(values []float64) float64 {
	return percentile(values, 0.5)
}
