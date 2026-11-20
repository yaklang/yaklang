package aicommon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestCodingConfigProjectSnapshot(t *testing.T) {
	primary, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	secondary, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	for _, managed := range []bool{false, true} {
		snapshot := CodingConfig{WorkspaceID: "project", PrimaryProjectDir: primary, AdditionalProjectDirs: []string{secondary}}
		source := primary
		if managed {
			// The source checkout need not still exist; only the session's actual directories are validated.
			source = filepath.Join(primary, "removed-source")
			snapshot.Worktree = &CodingWorktreeConfig{ID: "tree", SourceProjectDir: source, BaseRef: "origin/main", BaseCommit: "frozen"}
		}
		option := WithCodingConfig(snapshot)
		snapshot.AdditionalProjectDirs[0] = "mutated-input"
		if snapshot.Worktree != nil {
			snapshot.Worktree.BaseCommit = "mutated-input"
		}
		parent := &Config{AgentMode: ypb.AgentModeCoding, Workdir: "/artifacts"}
		require.NoError(t, option(parent))
		require.Equal(t, []string{primary, secondary}, parent.Coding.ProjectDirs())
		require.Equal(t, []string{source, secondary}, parent.Coding.SourceProjectDirs())
		require.Equal(t, "/artifacts", parent.Workdir)
		materials := &PromptMaterials{OSArch: "linux/amd64", WorkingDir: "/artifacts"}
		ApplyAgentPromptContext(materials, parent)
		promptBefore := materials.WorkspaceContext()
		childOption := WithCodingConfig(parent.Coding)
		parent.Coding.AdditionalProjectDirs[0] = "mutated-parent"
		if parent.Coding.Worktree != nil {
			parent.Coding.Worktree.BaseCommit = "mutated-parent"
		}
		child := &Config{AgentMode: ypb.AgentModeCoding}
		require.NoError(t, childOption(child))
		require.Equal(t, secondary, child.Coding.AdditionalProjectDirs[0])
		if managed {
			require.Equal(t, "frozen", child.Coding.Worktree.BaseCommit)
		}
		dirs := child.Coding.ProjectDirs()
		dirs[0], dirs[1] = "mutated-result", "mutated-result"
		sources := child.Coding.SourceProjectDirs()
		sources[0], sources[1] = "mutated-result", "mutated-result"
		require.Equal(t, []string{primary, secondary}, child.Coding.ProjectDirs())
		require.Equal(t, []string{source, secondary}, child.Coding.SourceProjectDirs())
		require.Equal(t, promptBefore, materials.WorkspaceContext(), "prompt snapshot must not follow parent mutations")
		require.NoError(t, WithAgentMode("")(parent))
		require.Equal(t, ypb.AgentModeSecurity, parent.AgentMode)
		require.Equal(t, CodingConfig{}, parent.Coding)
		require.NoError(t, WithCodingConfig(CodingConfig{PrimaryProjectDir: "/missing"})(parent))
		require.Equal(t, CodingConfig{}, parent.Coding, "security ignores coding configuration")
	}
	after, err := os.Getwd()
	require.NoError(t, err)
	require.Equal(t, cwd, after)
}

func TestCodingConfigRejectsIncompleteBinding(t *testing.T) {
	primary, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	for name, snapshot := range map[string]CodingConfig{
		"missing primary":         {},
		"missing secondary":       {PrimaryProjectDir: primary, AdditionalProjectDirs: []string{filepath.Join(primary, "missing")}},
		"relative primary":        {PrimaryProjectDir: "relative"},
		"missing worktree ID":     {PrimaryProjectDir: primary, Worktree: &CodingWorktreeConfig{SourceProjectDir: primary}},
		"missing worktree source": {PrimaryProjectDir: primary, Worktree: &CodingWorktreeConfig{ID: "tree"}},
	} {
		t.Run(name, func(t *testing.T) {
			config := &Config{AgentMode: ypb.AgentModeCoding, Coding: CodingConfig{PrimaryProjectDir: primary}}
			require.Error(t, WithCodingConfig(snapshot)(config))
			require.Equal(t, CodingConfig{PrimaryProjectDir: primary}, config.Coding, "failed replacement must leave the prior snapshot intact")
		})
	}
}
