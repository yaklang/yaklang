package test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

func getBashTool(t *testing.T) *aitool.Tool {
	t.Helper()
	content, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/system/bash.yak")
	assert.NilError(t, err)
	aiTool := yakscripttools.LoadYakScriptToAiTools("bash", string(content))
	assert.Assert(t, aiTool != nil, "failed to parse bash.yak metadata")
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	assert.Assert(t, len(tools) == 1, "ConvertTools returned %d tools", len(tools))
	return tools[0]
}

func TestBashToolNormalizesGeneratedScriptLineEndings(t *testing.T) {
	tool := getBashTool(t)
	stdout, stderr := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), aitool.InvokeParams{
		"command": "printf 'BASH_CRLF_OK\\n'",
		"shell":   "bash",
		"timeout": 10,
	}, nil, stdout, stderr)

	assert.NilError(t, err)
	combined := stdout.String() + stderr.String()
	assert.Assert(t, strings.Contains(combined, "BASH_CRLF_OK"), "command output missing: %s", combined)
	assert.Assert(t, !strings.Contains(combined, "command not found"), "generated script still contains CRLF: %s", combined)
	assert.Assert(t, !strings.Contains(combined, "exit code 127"), "bash tool unexpectedly failed: %s", combined)
}
