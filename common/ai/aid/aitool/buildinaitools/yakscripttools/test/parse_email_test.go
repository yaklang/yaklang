package test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

// getParseEmailTool 从 embed FS 加载 parse_email.yak 并转为可执行工具
func getParseEmailTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/email/parse_email.yak")
	assert.NilError(t, err)
	aiTool := yakscripttools.LoadYakScriptToAiTools("parse_email", string(content))
	assert.Assert(t, aiTool != nil, "failed to parse parse_email.yak metadata")
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	assert.Assert(t, len(tools) > 0, "ConvertTools returned empty")
	return tools[0]
}

// TestParseEmailTool_Metadata 验证 parse_email.yak 元数据可被正确解析
func TestParseEmailTool_Metadata(t *testing.T) {
	tool := getParseEmailTool(t)
	if tool.GetName() != "parse_email" {
		t.Errorf("tool name = %s, want parse_email", tool.GetName())
	}
}

// TestParseEmailTool_Execute 端到端：执行工具解析一封测试邮件，验证 mail 库在 yak 引擎可用
func TestParseEmailTool_Execute(t *testing.T) {
	tool := getParseEmailTool(t)

	eml := "From: phishing@evil.com\r\n" +
		"To: victim@example.com\r\n" +
		"Subject: Verify Account\r\n" +
		"Reply-To: attacker@evil.com\r\n" +
		"Authentication-Results: mx; spf=fail; dkim=fail; dmarc=fail\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"Visit http://evil.com/login to verify your account.\r\n"
	tmp := filepath.Join(t.TempDir(), "test.eml")
	assert.NilError(t, os.WriteFile(tmp, []byte(eml), 0o644))

	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, _ = tool.Callback(context.Background(), aitool.InvokeParams{"file": tmp}, nil, w1, w2)
	stdout := w1.String()

	// 验证摘要输出包含关键字段
	if !strings.Contains(stdout, "Verify Account") {
		t.Errorf("output missing subject, got:\n%s\nstderr: %s", stdout, w2.String())
	}
	if !strings.Contains(stdout, "phishing@evil.com") {
		t.Errorf("output missing sender, got:\n%s", stdout)
	}
	// 验证 URL 提取与可疑指标
	if !strings.Contains(stdout, "evil.com/login") {
		t.Errorf("output missing extracted URL, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "spf") || !strings.Contains(stdout, "fail") {
		t.Errorf("output missing auth results, got:\n%s", stdout)
	}
}
