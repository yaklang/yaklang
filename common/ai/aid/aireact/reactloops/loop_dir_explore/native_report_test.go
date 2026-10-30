package loop_dir_explore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

type nativeExploreRuntime struct{ *exploreOutputRuntime }

func (*nativeExploreRuntime) AddToTimelineWithPromptProjection(string, string, string) {}

// Phase 1 also delegates to report_generating. The parent, filesystem action,
// report child and finish must all stay native.
func TestDirExploreNativeReportDelegation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	workdir := t.TempDir()
	notePath, reportPath := filepath.Join(workdir, "dir_structure.md"), filepath.Join(workdir, "report.md")
	report := "# 项目概况\n\n这是测试项目的完整探索报告。\n"
	steps := []struct {
		name string
		args map[string]any
	}{
		{"write_file", map[string]any{"file": notePath, "content": "# 目录\nentry.go\n", "force": true}},
		{"complete_explore", map[string]any{"project_name": "fixture", "project_overview": "测试项目", "tech_stack": "Go", "entry_points": "entry.go", "modules_summary": "one package", "reading_guide": "Read entry.go"}},
		{"read_reference_file", map[string]any{"file_path": notePath}},
		{"write_section", map[string]any{"content": report}},
		{"finish", map[string]any{}},
	}
	calls := 0
	writeTool := aitool.NewWithoutCallback("write_file", aitool.WithStringParam("file", aitool.WithParam_Required()), aitool.WithStringParam("content", aitool.WithParam_Required()), aitool.WithBoolParam("force"))
	cfg := aicommon.NewConfig(ctx, aicommon.WithTools(writeTool), aicommon.WithWorkdir(workdir),
		aicommon.WithEnableFunctionCallMode(true), aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisablePerception(true), aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if calls >= len(steps) {
				return nil, fmt.Errorf("unexpected model request")
			}
			wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
			require.NotNil(t, wire.ToolCallCallback, "report child must retain native protocol")
			require.NotEmpty(t, wire.Tools)
			step := steps[calls]
			raw, err := json.Marshal(step.args)
			require.NoError(t, err)
			resp := c.NewAIResponse()
			wire.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprintf("explore_%d", calls), Type: "function", Function: aispec.FuncReturn{Name: step.name, Arguments: string(raw)}}})
			wire.FinishReasonCallback("tool_calls", nil)
			calls++
			resp.Close()
			return resp, nil
		}))
	inv := &nativeExploreRuntime{&exploreOutputRuntime{mock.NewMockInvoker(ctx)}}
	inv.SetConfig(cfg)
	loop, err := BuildDirExploreLoop(inv, reactloops.WithVar("target_path", workdir), reactloops.WithVar("output_report_path", reportPath),
		reactloops.WithDisableLoopPerception(true), reactloops.WithDisablePeriodicVerification(true), reactloops.WithDisableIncreaseIteration(true), reactloops.WithMaxIterations(8))
	require.NoError(t, err)
	defer loop.Release()
	task := aicommon.NewStatefulTaskBase("explore-native", "Explore fixture", ctx, cfg.GetEmitter(), true)
	inv.SetCurrentTask(task)
	require.NoError(t, loop.ExecuteWithExistedTask(task))
	require.Equal(t, len(steps), calls)
	require.Equal(t, reportPath, loop.Get("result_report_path"))
	written, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	require.Equal(t, report, string(written))
}
