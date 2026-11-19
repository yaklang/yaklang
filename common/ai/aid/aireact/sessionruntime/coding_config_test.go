package sessionruntime

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestCodingConfigTransportRoundTrip(t *testing.T) {
	primary, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	secondary, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	checkout := filepath.Join(primary, "checkout", "app")
	for _, managed := range []bool{false, true} {
		params := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: "workspace",
			CodingWorkingDirectory: primary, CodingWorkspaceRoots: []string{primary, secondary}, CodingSourceRoots: []string{primary, secondary}}
		var tree *schema.AICodingWorktree
		if managed {
			params.CodingWorktreeID, params.CodingWorkingDirectory, params.CodingWorkspaceRoots[0] = "tree", checkout, checkout
			tree = &schema.AICodingWorktree{WorkspaceID: "workspace", WorktreeID: "tree", SourceDirectory: primary, WorkingDirectory: checkout, BaseRef: "origin/main", BaseCommit: "frozen"}
		}
		config, err := codingConfigFromStartParams(params, tree)
		require.NoError(t, err)
		require.Equal(t, params.CodingWorkingDirectory, config.PrimaryProjectDir)
		require.Equal(t, []string{secondary}, config.AdditionalProjectDirs)
		if managed {
			require.Equal(t, &aicommon.CodingWorktreeConfig{ID: "tree", SourceProjectDir: primary, BaseRef: "origin/main", BaseCommit: "frozen"}, config.Worktree)
		} else {
			require.Nil(t, config.Worktree)
		}
		params.CodingWorkspaceRoots[1], params.CodingSourceRoots[1] = "mutated-request", "mutated-request"
		restored := &ypb.AIStartParams{CodingCreateWorktree: true, CodingWorktreeBaseRef: "HEAD", CodingWorktreeID: "stale"}
		applyCodingConfigToStartParams(restored, &aicommon.Config{AgentMode: ypb.AgentModeCoding, Coding: config})
		require.Equal(t, []string{primary, secondary}, restored.CodingSourceRoots)
		require.Equal(t, secondary, restored.CodingWorkspaceRoots[1])
		require.Equal(t, params.CodingWorktreeID, restored.CodingWorktreeID)
		require.False(t, restored.CodingCreateWorktree)
		require.Empty(t, restored.CodingWorktreeBaseRef)
		again, err := codingConfigFromStartParams(restored, tree)
		require.NoError(t, err)
		require.Equal(t, config, again)
		applyCodingConfigToStartParams(restored, &aicommon.Config{AgentMode: ypb.AgentModeSecurity})
		require.Empty(t, restored.CodingWorktreeID)
		require.Empty(t, restored.CodingWorkingDirectory)
		require.Empty(t, restored.CodingWorkspaceRoots)
		require.Empty(t, restored.CodingSourceRoots)
	}
}

func TestCodingConfigTransportRejectsInconsistentDirectories(t *testing.T) {
	for name, mutate := range map[string]func(*ypb.AIStartParams){
		"primary mismatch":   func(p *ypb.AIStartParams) { p.CodingWorkingDirectory = "/elsewhere" },
		"missing source":     func(p *ypb.AIStartParams) { p.CodingSourceRoots = nil },
		"secondary remapped": func(p *ypb.AIStartParams) { p.CodingWorkspaceRoots[1] = "/another-checkout" },
		"local remapped":     func(p *ypb.AIStartParams) { p.CodingWorktreeID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			params := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorktreeID: "tree", CodingWorkingDirectory: "/checkout",
				CodingWorkspaceRoots: []string{"/checkout", "/secondary"}, CodingSourceRoots: []string{"/original", "/secondary"}}
			mutate(params)
			_, err := convertStartParamsToReActConfig(params, nil)
			require.Error(t, err)
		})
	}
	params := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: "workspace", CodingWorktreeID: "tree", CodingWorkingDirectory: "/checkout",
		CodingWorkspaceRoots: []string{"/checkout"}, CodingSourceRoots: []string{"/original"}}
	_, err := codingConfigFromStartParams(params, &schema.AICodingWorktree{WorktreeID: "other-tree"})
	require.ErrorContains(t, err, "does not match")
	params.AgentMode = ypb.AgentModeSecurity
	config, err := codingConfigFromStartParams(params, nil)
	require.NoError(t, err)
	require.Equal(t, aicommon.CodingConfig{}, config)
}
