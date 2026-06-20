package test

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

func r2Available() bool {
	_, err := exec.LookPath("r2")
	return err == nil
}

func getR2AnalyzeTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/binary/r2_analyze.yak")
	assert.NilError(t, err)
	aiTool := yakscripttools.LoadYakScriptToAiTools("r2_analyze", string(content))
	assert.Assert(t, aiTool != nil, "failed to parse r2_analyze.yak metadata")
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	assert.Assert(t, len(tools) > 0, "ConvertTools returned empty")
	return tools[0]
}

func execR2Tool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (stdout, stderr string) {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, _ = tool.Callback(context.Background(), params, nil, w1, w2)
	return w1.String(), w2.String()
}

// TestR2AnalyzeTool_Metadata 验证 r2_analyze.yak 元数据可被正确解析
func TestR2AnalyzeTool_Metadata(t *testing.T) {
	tool := getR2AnalyzeTool(t)
	if tool.GetName() != "r2_analyze" {
		t.Errorf("tool name = %s, want r2_analyze", tool.GetName())
	}
}

// TestR2AnalyzeTool_Info 对 /bin/ls 跑 info action
func TestR2AnalyzeTool_Info(t *testing.T) {
	if !r2Available() {
		t.Skip("r2 not installed, skipping")
	}
	tool := getR2AnalyzeTool(t)
	stdout, _ := execR2Tool(t, tool, aitool.InvokeParams{
		"file":    "/bin/ls",
		"action":  "info",
		"timeout": 60,
	})
	// info 应输出文件信息（arch/bintype/binary 等）
	if !strings.Contains(stdout, "arch") && !strings.Contains(stdout, "bintype") && !strings.Contains(stdout, "binary") && !strings.Contains(stdout, "bits") {
		t.Errorf("info output missing file info, got:\n%s", stdout)
	}
}

// TestR2AnalyzeTool_StringsFilter 验证 strings action + filter 机制
func TestR2AnalyzeTool_StringsFilter(t *testing.T) {
	if !r2Available() {
		t.Skip("r2 not installed, skipping")
	}
	tool := getR2AnalyzeTool(t)
	stdout, _ := execR2Tool(t, tool, aitool.InvokeParams{
		"file":    "/bin/ls",
		"action":  "strings",
		"filter":  "ls",
		"timeout": 60,
	})
	// 工具回显的 r2 命令应含 ~ls 过滤后缀
	if !strings.Contains(stdout, "~ls") {
		t.Errorf("filter ~ls not reflected in command echo, got:\n%s", stdout)
	}
}

// TestR2AnalyzeTool_Sections 对 /bin/ls 跑 sections action
func TestR2AnalyzeTool_Sections(t *testing.T) {
	if !r2Available() {
		t.Skip("r2 not installed, skipping")
	}
	tool := getR2AnalyzeTool(t)
	stdout, _ := execR2Tool(t, tool, aitool.InvokeParams{
		"file":    "/bin/ls",
		"action":  "sections",
		"timeout": 60,
	})
	if !strings.Contains(stdout, "section") && !strings.Contains(stdout, "__TEXT") && !strings.Contains(stdout, ".text") {
		t.Errorf("sections output missing section info, got:\n%s", stdout)
	}
}
