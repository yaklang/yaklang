package aireact

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_default"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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
			react, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(artifacts), aicommon.WithAgentMode(ypb.AgentModeCoding), aicommon.WithCodingConfig(aicommon.CodingConfig{
				WorkspaceID: "workspace-a", PrimaryProjectDir: source, AdditionalProjectDirs: []string{secondary},
				Worktree: &aicommon.CodingWorktreeConfig{ID: "managed-tree", SourceProjectDir: "/original/project", BaseRef: "origin/main", BaseCommit: "initial-commit"},
			}))
			require.NoError(t, err)
			loop, err := loop_default.NewLoop("coding-prompt-test", react)
			require.NoError(t, err)
			require.Contains(t, loop.GetPersistentInstruction(), "# Coding 主循环")
			require.NotContains(t, loop.GetPersistentInstruction(), "用union注入探测字段数")
			require.Contains(t, loop.GetOutputExample(), "编程任务决策示例")
			require.NotContains(t, loop.GetOutputExample(), "重放漏洞")
			result, err := react.AssembleLoopPrompt(nil, &reactloops.LoopPromptAssemblyInput{Nonce: "coding", FunctionCallMode: native, UserQuery: "implement a feature", Schema: `{"type":"object"}`})
			require.NoError(t, err)
			require.Contains(t, result.Prompt, "# Workspace Context")
			require.Contains(t, result.Prompt, "# 第一部分: Coding Agent 工作方法")
			require.Contains(t, result.Prompt, "detached HEAD")
			require.Contains(t, result.Prompt, "不复制源目录的暂存、未暂存、未跟踪或忽略文件")
			require.NotContains(t, result.Prompt, "不可逆动作 (删除 / 覆写 / 提交) 降级强确认")
			require.NotContains(t, result.Prompt, "未落档的风险禁止终结")
			require.NotContains(t, result.Prompt, "任务 / 目标已外层鉴权")
			require.Contains(t, result.Prompt, "workspace-a")
			require.Contains(t, result.Prompt, `"working_directory": "`+react.config.Coding.PrimaryProjectDir+`"`)
			require.Contains(t, result.Prompt, `"artifacts_directory": "`+artifacts+`"`)
			require.Contains(t, result.Prompt, "AGENTS.md")
			require.Contains(t, result.Prompt, secondary)
			require.Contains(t, result.Prompt, "managed-tree")
			require.Contains(t, result.Prompt, `"creation_base_ref": "origin/main"`)
			require.Contains(t, result.Prompt, `"creation_base_commit": "initial-commit"`)
			require.Contains(t, result.Prompt, "其余目录仍指向原目录")
			require.NoDirExists(t, artifacts)
			after, err := os.Getwd()
			require.NoError(t, err)
			require.Equal(t, cwd, after)
			child := aicommon.NewConfig(ctx, aicommon.ConvertConfigToOptions(react.config)...)
			require.Equal(t, ypb.AgentModeCoding, child.AgentMode)
			require.Equal(t, react.config.Coding.PrimaryProjectDir, child.Coding.PrimaryProjectDir)
			require.Equal(t, artifacts, child.Workdir)
			require.Equal(t, react.config.Coding.ProjectDirs(), child.Coding.ProjectDirs())
			require.Equal(t, "managed-tree", child.Coding.Worktree.ID)
			require.Equal(t, "origin/main", child.Coding.Worktree.BaseRef)
			require.Equal(t, "initial-commit", child.Coding.Worktree.BaseCommit)
			child.Coding.AdditionalProjectDirs[0] = "changed"
			child.Coding.Worktree.BaseCommit = "changed"
			require.Equal(t, "initial-commit", react.config.Coding.Worktree.BaseCommit)
			require.Equal(t, secondary, react.config.Coding.ProjectDirs()[1])
		})
	}
}

func TestCodingPromptAbsentForOrdinarySession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	react, err := NewTestReAct(aicommon.WithContext(ctx))
	require.NoError(t, err)
	require.Equal(t, ypb.AgentModeSecurity, react.config.AgentMode)
	result, err := react.AssembleLoopPrompt(nil, &reactloops.LoopPromptAssemblyInput{Nonce: "ordinary", UserQuery: "hello", Schema: `{"type":"object"}`})
	require.NoError(t, err)
	require.NotContains(t, result.Prompt, "# Coding Agent")
	require.NotContains(t, result.Prompt, "# 第一部分: Coding Agent 工作方法")
	require.Contains(t, result.Prompt, "# 第一部分: 工作方法论")
}

func TestCodingPromptStaticPrefixAndDirectSourceContext(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{true: "native", false: "text"}[native], func(t *testing.T) {
			builder := aicommon.NewDefaultPromptPrefixBuilder()
			materials := &aicommon.PromptMaterials{AgentMode: ypb.AgentModeCoding, FunctionCallMode: native,
				Workspace: true,
				Coding:    aicommon.CodingConfig{WorkspaceID: "workspace-one", PrimaryProjectDir: "/project-one"}}
			first, err := builder.AssemblePromptPrefix(materials)
			require.NoError(t, err)
			require.Contains(t, first.HighStatic, "Coding Agent 工作方法")
			require.Contains(t, first.HighStatic, "## 输出协议")
			require.Contains(t, first.HighStatic, "directly_call_tool_params_group")
			require.NotContains(t, first.HighStatic, materials.Coding.PrimaryProjectDir)
			require.Contains(t, materials.WorkspaceContext(), "未创建托管工作树")
			require.NotContains(t, materials.WorkspaceContext(), "detached HEAD")
			materials.Coding.PrimaryProjectDir = "/another-project"
			materials.Coding.WorkspaceID = "another-workspace"
			materials.Coding.Worktree = &aicommon.CodingWorktreeConfig{ID: "another-tree", SourceProjectDir: "/project-one"}
			second, err := builder.AssemblePromptPrefix(materials)
			require.NoError(t, err)
			require.Equal(t, first.HighStatic, second.HighStatic, "paths and worktree IDs must not fragment the mode's system prefix")
			materials.AgentMode = ypb.AgentModeSecurity
			ordinary, err := builder.AssemblePromptPrefix(materials)
			require.NoError(t, err)
			require.NotEqual(t, first.HighStatic, ordinary.HighStatic)
			require.Contains(t, ordinary.HighStatic, "未落档的风险禁止终结")
			require.NotContains(t, ordinary.HighStatic, "Coding Agent 工作方法")
			builder.HighStaticTemplate = "custom caller instructions"
			materials.AgentMode = ypb.AgentModeCoding
			custom, err := builder.AssemblePromptPrefix(materials)
			require.NoError(t, err)
			require.Equal(t, "custom caller instructions", custom.HighStatic)
		})
	}
}

func TestAgentModeHelperPrompts(t *testing.T) {
	for _, mode := range []string{ypb.AgentModeSecurity, ypb.AgentModeCoding} {
		for _, native := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/native=%v", mode, native), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				opts := []aicommon.ConfigOption{aicommon.WithContext(ctx), aicommon.WithAgentMode(mode), aicommon.WithEnableFunctionCallMode(native)}
				if mode == ypb.AgentModeCoding {
					primary, err := filepath.EvalSymlinks(t.TempDir())
					require.NoError(t, err)
					opts = append(opts, aicommon.WithCodingConfig(aicommon.CodingConfig{WorkspaceID: "helper-workspace", PrimaryProjectDir: primary}))
				}
				r, err := NewTestReAct(opts...)
				require.NoError(t, err)
				prompts := aicommon.AgentPrompts(mode)
				verification, _, err := r.promptManager.GenerateVerificationPrompt("fix input", true, "regression passed")
				require.NoError(t, err)
				require.Contains(t, verification, strings.TrimSpace(prompts.VerificationInstruction))
				require.Contains(t, verification, strings.TrimSpace(prompts.VerificationOutputExample))
				answer, _, err := r.promptManager.GenerateDirectlyAnswerPrompt("explain changes", nil)
				require.NoError(t, err)
				for _, prompt := range []string{verification, answer} {
					if mode == ypb.AgentModeCoding {
						require.Contains(t, prompt, "Coding Agent 工作方法")
						require.Contains(t, prompt, "helper-workspace")
						require.NotContains(t, prompt, "未落档的风险禁止终结")
						require.NotContains(t, prompt, "安全测试阴性结论必须有区分力")
					} else {
						require.NotContains(t, prompt, "Coding Agent 工作方法")
					}
					require.True(t, aiprojection.Project(aiprojection.ProjectionInput{Prompt: prompt}).Metadata.CacheProjected)
				}
				// Specialized loops retain their explicit caller instruction.
				custom, err := loop_default.NewLoop("custom", r, reactloops.WithPersistentInstruction("caller-specific task"))
				require.NoError(t, err)
				require.Equal(t, "caller-specific task", custom.GetPersistentInstruction())
			})
		}
	}
}
