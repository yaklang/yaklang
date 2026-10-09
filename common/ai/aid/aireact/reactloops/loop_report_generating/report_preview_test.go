package loop_report_generating

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

func TestReportDisplayBudget(t *testing.T) {
	// A user-edited file can contain malformed UTF-8; even its title must not panic.
	require.Equal(t, "…", reportDisplayTitle(strings.Repeat("\x80", maxReportTitleBytes+1)))
	for _, content := range []string{
		"\n# 格式报告\r\n\r\n```go\r\n\traw := `中文 😀\\n`  \r\n```\r\n\r\n| 字段 | 值 |\r\n| --- | --- |\r\n| 引号 | \\\" |\r\n",
		strings.Repeat("a", maxReportDisplayBytes),
		strings.Repeat("段落\n", maxReportDisplayLines),
	} {
		require.Equal(t, content, reportDisplayMarkdown(content), "within-budget reports, including exact limits, must remain byte-for-byte identical")
	}
	for _, tc := range []struct {
		name, content, keep, omit string
	}{
		{"bytes/UTF8", "# 开头\n\n" + strings.Repeat(strings.Repeat("😀 中文 ", 32)+"\n", 300) + "最后一行", "😀 中文", "最后一行"},
		{"lines", "# 开头\n\n" + strings.Repeat("- 项目\n", maxReportDisplayLines) + "最后一行", "- 项目", "最后一行"},
		{"single-line", strings.Repeat("😀", 1024*1024/4), "开头段落或代码块超过聊天展示上限", "😀"},
		{"unfinished-mermaid", "# 开头\n\n~~~~mermaid\n" + strings.Repeat("graph TD; A-->B;\n", 5000) + "~~~~\n最后一行", "# 开头", "graph TD"},
		{"nested-fence", "# 开头\n\n````text\n```\n" + strings.Repeat("字面量代码\n", 5000) + "````\n最后一行", "# 开头", "字面量代码"},
		{"complete-fence/CRLF", "# 开头\r\n\r\n  ```go\r\n\tfmt.Print(\"中文\")\r\n  ``` \r\n\r\n" + strings.Repeat("项目资料\r\n", 5000) + "最后一行", "  ```go\r\n\tfmt.Print(\"中文\")\r\n  ``` ", "最后一行"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			preview := reportDisplayMarkdown(tc.content)
			require.LessOrEqual(t, len(preview), maxReportDisplayBytes)
			require.LessOrEqual(t, reportMarkdownLines(preview), maxReportDisplayLines)
			require.True(t, utf8.ValidString(preview), "never split a multi-byte character")
			require.Contains(t, preview, "完整报告")
			require.Contains(t, preview, "查看文献")
			require.Contains(t, preview, tc.keep)
			require.NotContains(t, preview, tc.omit)
		})
	}
}

type reportTimelineRecorder struct {
	*mock.MockInvoker
	reportEntries []string
}

func (r *reportTimelineRecorder) AddToTimeline(entry, content string) {
	if entry == "report_finish" {
		r.reportEntries = append(r.reportEntries, content)
	}
}

// The parent focus mode uses the same delivery helper with its own overview.
// Verify it cannot bypass the display limit or copy the full body into timeline.
func TestParentReportDeliveryBudget(t *testing.T) {
	inv := &reportTimelineRecorder{MockInvoker: mock.NewMockInvoker(context.Background())}
	cfg := inv.GetConfig().(*mock.MockedAIConfig)
	var mu sync.Mutex
	var events []*schema.AiOutputEvent
	cfg.Emitter = aicommon.NewEmitter("report-preview-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
		return e, nil
	})
	loop := reactloops.NewMinimalReActLoop(cfg, inv)
	path := filepath.Join(t.TempDir(), "report.md")
	content := "# 报告\n\n" + strings.Repeat(strings.Repeat("中文 😀 ", 32)+"\n", 4000) + "全文尾部标记"
	require.GreaterOrEqual(t, len(content), 1024*1024)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	require.NoError(t, EmitReportFinish(loop, path, strings.Repeat("长标题😀", 10000), content))
	cfg.GetEmitter().WaitForStream()
	written, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, content, string(written))
	mu.Lock()
	defer mu.Unlock()
	var cardCount, referenceCount int
	for _, event := range events {
		switch event.Type {
		case schema.EVENT_TYPE_REPORT_FINISH:
			cardCount++
			markdown := event.GetContentJSONPath("$.summary_markdown")
			require.LessOrEqual(t, len(markdown), maxReportDisplayBytes)
			require.LessOrEqual(t, reportMarkdownLines(markdown), maxReportDisplayLines)
			require.Contains(t, markdown, "仅展示开头预览")
			require.NotContains(t, markdown, "全文尾部标记")
			title := event.GetContentJSONPath("$.title")
			require.LessOrEqual(t, len(title), maxReportTitleBytes)
			require.True(t, utf8.ValidString(title))
			require.True(t, strings.HasSuffix(title, "…"))
		case schema.EVENT_TYPE_REFERENCE_MATERIAL:
			referenceCount++
			require.Equal(t, content, event.GetContentJSONPath("$.payload"))
		}
	}
	require.Equal(t, 1, cardCount)
	require.Equal(t, 1, referenceCount)
	require.Len(t, inv.reportEntries, 1)
	require.Less(t, len(inv.reportEntries[0]), maxReportDisplayBytes+maxReportTitleBytes+len(path)+100)
	require.NotContains(t, inv.reportEntries[0], "全文尾部标记")
}
