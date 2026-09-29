package aireact

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/ytoken"
)

// File changes, recommendations and new observations must not rewrite the
// workspace prefix. Exercise both rendered sections and real wire projection.
func TestPromptWorkspaceStableAcrossDirectoryAndDynamicChanges(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	artifacts := filepath.Join(t.TempDir(), "not-created-artifacts")
	r, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(artifacts))
	require.NoError(t, err)
	wd := t.TempDir()
	r.promptManager.workdir = wd
	input := &reactloops.LoopPromptAssemblyInput{
		Nonce: "workspace-test", UserQuery: "inspect the project", Schema: `{"type":"object"}`,
		ExtraCapabilities: "recommended skill alpha; recommended tool alpha", InjectedMemory: "memory alpha",
	}
	r.AddToTimeline("test", "first observation")
	first, err := r.AssembleLoopPrompt(nil, input)
	require.NoError(t, err)
	firstSections := mustLoopPromptSections(t, first.Sections)
	var workspace string
	for _, section := range firstSections {
		for _, child := range section.Children {
			if child.Key == "section.semi_dynamic_1.workspace" {
				workspace = child.Content
			}
		}
	}
	require.Contains(t, workspace, "working dir: "+wd)
	require.Contains(t, workspace, "AI Artifacts dir: "+artifacts)
	require.Contains(t, workspace, "Windows 盘符")
	require.Less(t, ytoken.CalcTokenCount(workspace), 600, "workspace must remain a small coordinates block")
	t.Logf("workspace: %d bytes, %d local tokens", len(workspace), ytoken.CalcTokenCount(workspace))
	require.NoDirExists(t, artifacts, "prompt rendering must not create artifacts")
	gitDir := filepath.Join(wd, ".git", "objects")
	require.NoError(t, os.MkdirAll(gitDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "workspace-scan-sentinel"), []byte(strings.Repeat("data", 1000)), 0600))
	input.ExtraCapabilities = "recommended skill beta; recommended tool beta"
	input.InjectedMemory = "memory beta"
	r.AddToTimeline("test", "second observation")
	second, err := r.AssembleLoopPrompt(nil, input)
	require.NoError(t, err)
	require.NotContains(t, second.Prompt, "workspace-scan-sentinel")
	require.NotContains(t, second.Prompt, "working dir glance:")
	require.Equal(t, 1, strings.Count(second.Prompt, "# Workspace Context"))
	require.Equal(t, 1, strings.Count(second.Prompt, "# Current Time"))
	require.NoDirExists(t, artifacts)
	openAt := strings.Index(first.Prompt, "# Timeline Memory (Open Tail)")
	require.Positive(t, openAt)
	require.Equal(t, first.Prompt[:openAt], second.Prompt[:strings.Index(second.Prompt, "# Timeline Memory (Open Tail)")])
	sections := mustLoopPromptSections(t, second.Sections)
	for _, section := range sections {
		for _, child := range section.Children {
			switch child.Key {
			case "section.semi_dynamic_1.workspace":
				require.Equal(t, workspace, child.Content)
			case "section.dynamic.extra_capabilities":
				require.Contains(t, child.Content, "recommended skill beta")
			case "section.dynamic.current_time":
				require.NotEmpty(t, child.Content)
			}
		}
	}
	restore := aiprojection.SetMinCachableUserSegmentBytesForTest(0)
	defer restore()
	wire1 := aiprojection.ProjectAndObserve("test-model", first.Prompt)
	wire2 := aiprojection.ProjectAndObserve("test-model", second.Prompt)
	require.True(t, wire1.IsHijacked)
	require.True(t, wire2.IsHijacked)
	workspaceMessage := -1
	for i, message := range wire1.Messages {
		encoded, err := json.Marshal(message)
		require.NoError(t, err)
		if strings.Contains(string(encoded), "# Workspace Context") {
			workspaceMessage = i
			require.NotContains(t, string(encoded), "first observation")
			require.NotContains(t, string(encoded), "# Current Time")
		}
	}
	require.GreaterOrEqual(t, workspaceMessage, 0)
	require.Greater(t, len(wire2.Messages), workspaceMessage)
	require.Equal(t, wire1.Messages[:workspaceMessage+1], wire2.Messages[:workspaceMessage+1])
}

func TestPromptWorkspaceConfiguredPathsAreReadOnly(t *testing.T) {
	cfg := &aicommon.Config{}
	require.Empty(t, cfg.GetConfiguredWorkDir())
	require.False(t, cfg.IsWorkDirReady())
	dir := filepath.Join(t.TempDir(), "artifacts")
	cfg.Workdir = dir
	require.Equal(t, dir, cfg.GetConfiguredWorkDir())
	require.False(t, cfg.IsWorkDirReady(), "reading a configured path must not initialize the directory")
	cfg.SetWorkDir(dir)
	cfg.SetWorkDir("replacement-is-ignored")
	require.Equal(t, dir, cfg.GetConfiguredWorkDir())
	require.NoDirExists(t, dir)
	for _, paths := range [][2]string{{`C:\project with spaces`, `D:\AI Artifacts\session`}, {"/srv/project", "/srv/artifacts/session"}} {
		m := &aicommon.PromptMaterials{Workspace: true, WorkingDir: paths[0], AIArtifactsDir: paths[1], WorkingDirGlance: "legacy-tree-must-not-render"}
		text := m.WorkspaceContext()
		require.Contains(t, text, "working dir: "+paths[0])
		require.Contains(t, text, "AI Artifacts dir: "+paths[1])
		require.NotContains(t, text, "legacy-tree")
	}
	require.Empty(t, (&aicommon.PromptMaterials{Workspace: true}).WorkspaceContext())
}
