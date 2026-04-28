package yakcmds

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// PrepareYakAIFocusFromFile 路径不存在时报错。
// 关键词: yak ai-focus prepare not exist
func TestPrepareYakAIFocusFromFile_NotExist(t *testing.T) {
	_, err := PrepareYakAIFocusFromFile("/path/does/not/exist/__test__.ai-focus.yak")
	require.Error(t, err)
}

// PrepareYakAIFocusFromFile 文件后缀不是 .ai-focus.yak 时报错。
// 关键词: yak ai-focus prepare bad suffix
func TestPrepareYakAIFocusFromFile_BadSuffix(t *testing.T) {
	tmp := t.TempDir()
	bad := filepath.Join(tmp, "demo.yak")
	require.NoError(t, os.WriteFile(bad, []byte(`__VERBOSE_NAME__ = "x"`), 0o644))

	_, err := PrepareYakAIFocusFromFile(bad)
	require.Error(t, err)
	require.Contains(t, err.Error(), reactloops.FocusModeFileSuffix)
}

// PrepareYakAIFocusFromFile 空字符串报错。
// 关键词: yak ai-focus prepare empty path
func TestPrepareYakAIFocusFromFile_Empty(t *testing.T) {
	_, err := PrepareYakAIFocusFromFile("")
	require.Error(t, err)
}

// PrepareYakAIFocusFromFile：写入一个临时 focus mode + sidekick，校验 register 成功。
// 关键词: yak ai-focus prepare success
func TestPrepareYakAIFocusFromFile_Success(t *testing.T) {
	tmp := t.TempDir()
	uniq := utils.RandStringBytes(6)
	mainName := "cli_" + uniq + ".ai-focus.yak"
	sidekickName := "cli_" + uniq + "_helper.yak"

	mainPath := filepath.Join(tmp, mainName)
	sidekickPath := filepath.Join(tmp, sidekickName)

	require.NoError(t, os.WriteFile(mainPath, []byte(`
__VERBOSE_NAME__ = greeting()
__MAX_ITERATIONS__ = 3
`), 0o644))
	require.NoError(t, os.WriteFile(sidekickPath, []byte(`
greeting = func() { return "cli demo via sidekick" }
`), 0o644))

	name, err := PrepareYakAIFocusFromFile(mainPath)
	require.NoError(t, err)
	require.Equal(t, "cli_"+uniq, name)

	_, ok := reactloops.GetLoopFactory(name)
	require.True(t, ok, "factory should be registered after prepare")

	meta, ok := reactloops.GetLoopMetadata(name)
	require.True(t, ok)
	require.Equal(t, "cli demo via sidekick", meta.VerboseName)
}

// printAIFocusEvent：人类可读模式下，对纯心跳/状态噪声跳过；其他事件输出到 stdout。
// 关键词: yak ai-focus print event human readable
func TestPrintAIFocusEvent_HumanReadable(t *testing.T) {
	stdout := captureStdout(t, func() {
		// 心跳事件应被过滤
		printAIFocusEvent(&schema.AiOutputEvent{Type: schema.EVENT_TYPE_PONG, NodeId: "ping"}, false)

		// 流式事件
		printAIFocusEvent(&schema.AiOutputEvent{
			Type:        schema.EVENT_TYPE_STREAM,
			NodeId:      "answer",
			IsStream:    true,
			StreamDelta: []byte("hello world"),
		}, false)

		// 普通 content 事件
		printAIFocusEvent(&schema.AiOutputEvent{
			Type:    schema.EVENT_TYPE_STRUCTURED,
			NodeId:  "react_task_status_changed",
			Content: []byte(`{"react_task_now_status":"completed"}`),
		}, false)

		// 仅 type 没内容
		printAIFocusEvent(&schema.AiOutputEvent{Type: schema.EVENT_TYPE_STRUCTURED, NodeId: "noop"}, false)
	})

	require.NotContains(t, stdout, "[pong]")
	require.NotContains(t, stdout, "[ping]")
	require.Contains(t, stdout, "[answer]")
	require.Contains(t, stdout, "hello world")
	require.Contains(t, stdout, "react_task_status_changed")
	require.Contains(t, stdout, "completed")
	require.Contains(t, stdout, "[noop]")
}

// printAIFocusEvent：JSON 模式下每行一个 JSON 事件。
// 关键词: yak ai-focus print event json mode
func TestPrintAIFocusEvent_JSONMode(t *testing.T) {
	stdout := captureStdout(t, func() {
		printAIFocusEvent(&schema.AiOutputEvent{
			Type:    schema.EVENT_TYPE_STRUCTURED,
			NodeId:  "demo",
			Content: []byte("payload"),
		}, true)
	})

	stdout = strings.TrimSpace(stdout)
	require.NotEmpty(t, stdout)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &parsed))
	require.Equal(t, "demo", parsed["NodeId"])
}

// captureStdout 在 fn 执行期间替换 os.Stdout，并返回捕获到的内容。
// 关键词: stdout capture helper for cli test
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)

	old := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	fn()
	_ = w.Close()
	<-done
	return buf.String()
}
