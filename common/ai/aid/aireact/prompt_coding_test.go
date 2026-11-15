package aireact

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestCodingPromptAndChildConfig(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{true: "native", false: "text"}[native], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			source, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			secondary, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			artifacts := filepath.Join(t.TempDir(), "artifacts")
			cwd, err := os.Getwd()
			require.NoError(t, err)
			react, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(artifacts), aicommon.WithCodingMode(true, "workspace-a", source), aicommon.WithCodingDirectories("managed-tree", []string{"/original/project", secondary}, []string{source, secondary}))
			require.NoError(t, err)
			result, err := react.AssembleLoopPrompt(nil, &reactloops.LoopPromptAssemblyInput{Nonce: "coding", FunctionCallMode: native, UserQuery: "implement a feature", Schema: `{"type":"object"}`})
			require.NoError(t, err)
			require.Contains(t, result.Prompt, "# Coding Agent (Experimental)")
			require.Contains(t, result.Prompt, "workspace-a")
			require.Contains(t, result.Prompt, "working dir: "+react.config.CodingWorkingDirectory)
			require.Contains(t, result.Prompt, "AI Artifacts dir: "+artifacts)
			require.Contains(t, result.Prompt, "AGENTS.md")
			require.Contains(t, result.Prompt, secondary)
			require.Contains(t, result.Prompt, "managed-tree")
			require.Contains(t, result.Prompt, "其余目录仍指向原目录")
			require.NoDirExists(t, artifacts)
			after, err := os.Getwd()
			require.NoError(t, err)
			require.Equal(t, cwd, after)
			child := aicommon.NewConfig(ctx, aicommon.ConvertConfigToOptions(react.config)...)
			require.True(t, child.CodingMode)
			require.Equal(t, react.config.CodingWorkingDirectory, child.CodingWorkingDirectory)
			require.Equal(t, artifacts, child.Workdir)
			require.Equal(t, react.config.CodingWorkspaceRoots, child.CodingWorkspaceRoots)
			require.Equal(t, "managed-tree", child.CodingWorktreeID)
			child.CodingWorkspaceRoots[1] = "changed"
			require.Equal(t, secondary, react.config.CodingWorkspaceRoots[1])
		})
	}
}

func TestCodingPromptAbsentForOrdinarySession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	react, err := NewTestReAct(aicommon.WithContext(ctx))
	require.NoError(t, err)
	result, err := react.AssembleLoopPrompt(nil, &reactloops.LoopPromptAssemblyInput{Nonce: "ordinary", UserQuery: "hello", Schema: `{"type":"object"}`})
	require.NoError(t, err)
	require.NotContains(t, result.Prompt, "# Coding Agent")
}
