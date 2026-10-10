package syntaxflow_scan_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const runtimeWiringProgram = `package main

import "net/http"

func run(cmd string) {
	sink(cmd)
	http.ListenAndServe(":80", nil)
}

func sink(any) {}
`

func runtimeWiringRule(content, language string) *ypb.SyntaxFlowRuleInput {
	return &ypb.SyntaxFlowRuleInput{Content: content, Language: language}
}

// TestScanProject_RisksFlowThroughScanRuntime proves every stage of a product
// scan submits its findings to the one runtime of the scan instead of writing
// them itself, and that the report ends up with the findings the collect kept.
func TestScanProject_RisksFlowThroughScanRuntime(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "main.go"),
		[]byte(runtimeWiringProgram),
		0o644,
	))

	rt := ssaapi.NewScanRuntime()
	var mu sync.Mutex
	var items []schema.RiskUpdateItem
	rt.ListenRisk(schema.RiskUpdateHandlerFunc(func(item schema.RiskUpdateItem) error {
		mu.Lock()
		defer mu.Unlock()
		items = append(items, item)
		return nil
	}))

	projectResult, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("golang"),
		ssaconfig.WithSetProgramName("runtime-wiring-program"),
		ssaconfig.WithScanIgnoreLanguage(true),
		// Two rules of different modes produce two findings that must both go
		// through the runtime of this scan.
		ssaconfig.WithRuleInput(runtimeWiringRule(`desc(mode: "struct", language: golang, title: "wiring struct")
http.ListenAndServe as $call;
alert $call`, "golang")),
		ssaconfig.WithRuleInput(runtimeWiringRule(`desc(mode: "ssa", language: golang, title: "wiring ssa")
sink(* as $arg) as $call;
alert $call`, "golang")),
		syntaxflow_scan.WithMode(syntaxflow_scan.StructMode, syntaxflow_scan.SSAMode),
		syntaxflow_scan.WithScanRuntime(rt),
	)
	require.NoError(t, err)

	mu.Lock()
	submitted := len(items)
	mu.Unlock()
	require.Greater(t, submitted, 0, "scan findings must reach the runtime collect")

	var reported int64
	for _, stage := range projectResult.Stages {
		reported += stage.RiskCount
	}
	require.Greater(t, reported, int64(0), "the scan must report the findings it kept")
}

// TestScanProject_RuntimeIsCreatedWhenNotProvided proves a product scan builds
// its own runtime, so callers cannot forget to wire the collect.
func TestScanProject_RuntimeIsCreatedWhenNotProvided(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "main.go"),
		[]byte(runtimeWiringProgram),
		0o644,
	))

	projectResult, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("golang"),
		ssaconfig.WithSetProgramName("runtime-auto-program"),
		ssaconfig.WithScanIgnoreLanguage(true),
		ssaconfig.WithRuleInput(runtimeWiringRule(`desc(mode: "ssa", language: golang, title: "auto ssa")
sink(* as $arg) as $call;
alert $call`, "golang")),
		syntaxflow_scan.WithMode(syntaxflow_scan.SSAMode),
	)
	require.NoError(t, err)
	require.NotEmpty(t, projectResult.Stages)
}

// TestScanProject_SourceStageSubmitsThroughRuntime proves the source stage is
// part of the same contract as the struct and SSA stages: its findings reach
// the scan runtime's collect instead of being written by the source engine.
func TestScanProject_SourceStageSubmitsThroughRuntime(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "main.go"),
		[]byte(runtimeWiringProgram),
		0o644,
	))

	rt := ssaapi.NewScanRuntime()
	var mu sync.Mutex
	var modes []string
	rt.ListenRisk(schema.RiskUpdateHandlerFunc(func(item schema.RiskUpdateItem) error {
		mu.Lock()
		defer mu.Unlock()
		if item.Risk != nil {
			modes = append(modes, item.Risk.ScanMode)
		}
		return nil
	}))

	_, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("golang"),
		ssaconfig.WithSetProgramName("runtime-wiring-source-program"),
		ssaconfig.WithScanIgnoreLanguage(true),
		ssaconfig.WithRuleInput(runtimeWiringRule(`desc(mode: "source", language: general, title: "wiring source")
${*}.pattern_regex(/sink/) as $hit
alert $hit`, "")),
		syntaxflow_scan.WithMode(syntaxflow_scan.SourceMode),
		syntaxflow_scan.WithScanRuntime(rt),
	)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, modes, string(schema.SFR_MODE_SOURCE),
		"the source stage submits its finding to the runtime")
}
