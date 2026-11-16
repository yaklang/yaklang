package aicommon

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestCodingWorkspaceJSONRoundTrip(t *testing.T) {
	// Include shell metacharacters, template syntax, control characters and projection tags.
	primary := "C:\\项目\\quote\"\n\t\v\x01 ${HOME} {{ .Wrong }} <|TRAITS_END|>"
	secondary := "/source/second folder"
	m := &PromptMaterials{AgentMode: ypb.AgentModeCoding, Workspace: true, OSArch: "windows/amd64", WorkingDir: primary,
		AIArtifactsDir: "/artifacts", CodingWorkspaceID: "workspace", CodingWorktreeID: "tree",
		CodingSourceRoots: []string{"/original", secondary}, CodingWorkspaceRoots: []string{primary, secondary},
		CodingBaseRef: "origin/main", CodingBaseCommit: "abc"}
	content := m.WorkspaceContext()
	require.Equal(t, 1, strings.Count(content, "# Workspace Context"))
	start := strings.Index(content, "\n{")
	require.GreaterOrEqual(t, start, 0)
	var data codingWorkspacePromptData
	require.NoError(t, json.NewDecoder(strings.NewReader(content[start:])).Decode(&data))
	require.Equal(t, primary, data.WorkingDirectory)
	require.Equal(t, []codingPromptDirectory{{true, "/original", primary}, {false, secondary, secondary}}, data.Directories)
	require.Equal(t, "origin/main", data.Worktree.CreationBaseRef)
	require.NotContains(t, content, "<|TRAITS_END|>", "path data must not become projection boundaries")
	require.NotContains(t, content, "<no value>")
	m.CodingWorktreeID = ""
	require.NotContains(t, m.WorkspaceContext(), `"worktree":`)
	require.NotContains(t, m.WorkspaceContext(), `"creation_base_commit":`)
}

func TestAgentPromptCacheBoundaries(t *testing.T) {
	for _, mode := range []string{ypb.AgentModeSecurity, ypb.AgentModeCoding} {
		for _, native := range []bool{false, true} {
			m := &PromptMaterials{AgentMode: mode, FunctionCallMode: native, Workspace: true,
				WorkingDir: "/project", OSArch: "linux/amd64", TaskInstruction: "task", Schema: "schema"}
			builder := NewDefaultPromptPrefixBuilder()
			build := func(query string) map[string][]*aiprojection.Chunk {
				prompt, err := builder.AssemblePromptWithDynamicSection(m, "dynamic", "{{ . }}", query, query)
				require.NoError(t, err)
				require.Equal(t, 1, strings.Count(prompt, "# Workspace Context"))
				require.NotContains(t, prompt, "<no value>")
				return promptBuilderChunksBySection(t, prompt)
			}
			first, second := build("first"), build("second")
			for _, section := range []string{aiprojection.SectionHighStatic, aiprojection.SectionSemiDynamic, aiprojection.SectionSemiDynamic1, aiprojection.SectionSemiDynamic2} {
				require.Equal(t, first[section], second[section], "%s native=%v %s", mode, native, section)
			}
			require.NotEqual(t, first[aiprojection.SectionDynamic], second[aiprojection.SectionDynamic])
			m.WorkingDir, m.CodingWorktreeID = "/other-project", "tree"
			third := build("second")
			require.Equal(t, second[aiprojection.SectionHighStatic], third[aiprojection.SectionHighStatic])
			require.Equal(t, second[aiprojection.SectionSemiDynamic2], third[aiprojection.SectionSemiDynamic2])
			require.NotEqual(t, second[aiprojection.SectionSemiDynamic1], third[aiprojection.SectionSemiDynamic1])
			builder.HighStaticTemplate = "caller-owned identity"
			custom, err := builder.AssemblePromptPrefix(m)
			require.NoError(t, err)
			require.Equal(t, "caller-owned identity", custom.HighStatic)
		}
	}
}

func TestAgentModePlanningReviewContext(t *testing.T) {
	for _, mode := range []string{ypb.AgentModeSecurity, ypb.AgentModeCoding} {
		for name, build := range map[string]func(*Config, aitool.InvokeParams) (string, error){"plan": generatePlanReviewPrompt, "task": generateTaskReviewPrompt} {
			t.Run(mode+"/"+name, func(t *testing.T) {
				// Prompt rendering must use the bound snapshot without requiring these paths to exist.
				cfg := NewConfig(context.Background(), WithAgentMode(mode), WithWorkdir("/review-artifacts"))
				cfg.CodingWorkingDirectory, cfg.CodingWorkspaceID = "/review-worktree", "review-workspace"
				cfg.CodingWorktreeID, cfg.CodingBaseCommit = "review-tree", "base-commit"
				cfg.CodingWorkspaceRoots = []string{cfg.CodingWorkingDirectory, "/secondary"}
				cfg.CodingSourceRoots = []string{"/source", "/secondary"}
				prompt, err := build(cfg, aitool.InvokeParams{"progress": "task observation"})
				require.NoError(t, err)
				require.Contains(t, prompt, "task observation")
				chunks := promptBuilderChunksBySection(t, prompt)
				require.NotEmpty(t, chunks[aiprojection.SectionHighStatic])
				require.Contains(t, chunks[aiprojection.SectionHighStatic][0].Content, map[string]string{ypb.AgentModeCoding: "Coding Agent 工作方法", ypb.AgentModeSecurity: "# 第一部分: 工作方法论"}[mode])
				if mode == ypb.AgentModeCoding {
					require.Contains(t, prompt, `"working_directory": "/review-worktree"`)
					require.Contains(t, prompt, `"artifacts_directory": "/review-artifacts"`)
					require.Contains(t, prompt, "review-tree")
					require.Contains(t, prompt, "base-commit")
					require.NotContains(t, prompt, "未落档的风险禁止终结")
				} else {
					require.Contains(t, prompt, "working dir: /review-artifacts")
					require.NotContains(t, prompt, "review-tree")
					require.NotContains(t, prompt, "Coding Agent 工作方法")
				}
			})
		}
	}
}
