package yaklangcodetests

// YakRunner protocol E2E tests (FreeInput + FocusModeLoop + AttachedResourceInfo + yaklang_code_change).
//
// Run:
//   go test -v -run TestYakRunnerProtocol_ ./common/ai/aid/aireact/reactloops/loop_yaklangcode/tests/...

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const sampleScanYak = `// timeout was 30
// scan.yak placeholder
// user selection block
// end
`

type yaklangCodeChangeResponse struct {
	Op           string `json:"op"`
	SourceAction string `json:"source_action"`
	Reason       string `json:"reason,omitempty"`
	Code         struct {
		Content string `json:"content"`
		Path    string `json:"path,omitempty"`
		Summary string `json:"summary,omitempty"`
		Version int    `json:"version"`
	} `json:"code"`
}

func writeScanYakFile(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "scan.yak")
	require.NoError(t, os.WriteFile(path, []byte(sampleScanYak), 0o644))
	return path
}

func yakRunnerWorkspaceAttachments(dir, yakPath string) []*ypb.AttachedResourceInfo {
	return []*ypb.AttachedResourceInfo{
		{Type: "file", Key: "directory_path", Value: dir},
		{Type: "file", Key: "file_path", Value: yakPath},
	}
}

func yakRunnerFullAttachments(dir, yakPath, selectionJSON string) []*ypb.AttachedResourceInfo {
	return append(yakRunnerWorkspaceAttachments(dir, yakPath), &ypb.AttachedResourceInfo{
		Type:  "selected",
		Key:   "content",
		Value: selectionJSON,
	})
}

type yakRunnerModifyMock struct {
	modifyAttempts int
}

func (m *yakRunnerModifyMock) callback(t *testing.T, i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()

	if utils.MatchAnyOfSubString(prompt, "modify_code", "GEN_CODE", "yak_code") {
		m.modifyAttempts++
		nonceStr := aicommon.MustExtractDynamicSectionNonce(t, prompt)
		partial := `// timeout was 60`
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(utils.MustRenderTemplate(`{"@action": "modify_code", "modify_start_line": 1, "modify_end_line": 1, "modify_code_reason": "修正 synscan 超时参数"}

<|GEN_CODE_{{ .nonce }}|>
`+partial+`
<|GEN_CODE_END_{{ .nonce }}|>`, map[string]any{
			"nonce": nonceStr,
		})))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "grep_yaklang_samples", "@action") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "grep_yaklang_samples", "patterns": ["synscan.timeout"]}`))
		rsp.Close()
		return rsp, nil
	}

	return nil, utils.Errorf("unexpected prompt: %s", utils.ShrinkTextBlock(prompt, 512))
}

type yakRunnerScenarioResult struct {
	timeline         string
	taskFailed       bool
	taskCompleted    bool
	codeChangeEvents []*ypb.AIOutputEvent
	pinFilenameEvents []*ypb.AIOutputEvent
	modifyAttempts   int
}

type yakRunnerAICallback func(t *testing.T, i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error)

func runYakRunnerProtocolScenario(
	t *testing.T,
	callback yakRunnerAICallback,
	userQuery string,
	attached []*ypb.AttachedResourceInfo,
) yakRunnerScenarioResult {
	t.Helper()

	in := make(chan *ypb.AIInputEvent, 4)
	out := make(chan *ypb.AIOutputEvent, 128)

	ins, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return callback(t, i, r)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	require.NoError(t, err)

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput:          true,
			FreeInput:            userQuery,
			FocusModeLoop:        schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG,
			AttachedResourceInfo: attached,
		}
	}()

	timeout := 10 * time.Second
	if utils.InGithubActions() {
		timeout = 15 * time.Second
	}
	deadline := time.After(timeout)

	var result yakRunnerScenarioResult

taskLoop:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_YAKLANG_CODE_CHANGE) {
				result.codeChangeEvents = append(result.codeChangeEvents, e)
			}
			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				result.pinFilenameEvents = append(result.pinFilenameEvents, e)
			}
			if e.GetNodeId() == "react_task_status_changed" {
				content := string(e.GetContent())
				if strings.Contains(content, "Aborted") || strings.Contains(content, "Failed") {
					result.taskFailed = true
					break taskLoop
				}
				if strings.Contains(content, `"react_task_now_status":"completed"`) ||
					strings.Contains(content, `"react_task_now_status": "completed"`) {
					result.taskCompleted = true
					break taskLoop
				}
			}
			if len(result.codeChangeEvents) > 0 {
				break taskLoop
			}
		case <-deadline:
			break taskLoop
		}
	}
	close(in)
	ins.Wait()

	result.timeline = ins.DumpTimeline()
	return result
}

func yakRunnerDirectoryOnlyAttachments(dir string) []*ypb.AttachedResourceInfo {
	return []*ypb.AttachedResourceInfo{
		{Type: "file", Key: "directory_path", Value: dir},
	}
}

func yakRunnerPreviewWriteCallback(t *testing.T, i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	return mockedYaklangWriting(t, i, req, "hello")
}

// isYakRunnerIntermediatePinPath reports aispace staging/work files that may be pinned
// while write_code/modify_code runs; they are not the user-facing delivery target.
func isYakRunnerIntermediatePinPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	base := strings.ToLower(filepath.Base(path))
	if strings.HasPrefix(base, "yaklang_code_staging_") && strings.HasSuffix(base, ".yak") {
		return true
	}
	aispace := string(filepath.Separator) + "aispace" + string(filepath.Separator)
	if strings.Contains(path, aispace) && strings.Contains(base, "gen_code_") && strings.HasSuffix(base, ".yak") {
		return true
	}
	return false
}

func assertYakRunnerNoYakPinFilenameEvents(t *testing.T, events []*ypb.AIOutputEvent) {
	t.Helper()
	for _, e := range events {
		path := utils.InterfaceToString(jsonpath.FindFirst(string(e.GetContent()), "$.path"))
		if isYakRunnerIntermediatePinPath(path) {
			continue
		}
		if strings.HasSuffix(strings.ToLower(path), ".yak") || strings.Contains(path, "gen_code_") {
			t.Fatalf("unexpected yak pin_filename before loop finished: %s", path)
		}
	}
}

func parseYaklangCodeChangeResponse(t *testing.T, e *ypb.AIOutputEvent) yaklangCodeChangeResponse {
	t.Helper()
	require.Equal(t, string(schema.EVENT_TYPE_YAKLANG_CODE_CHANGE), e.Type)
	require.Equal(t, "yaklang_code_change", e.NodeId)
	require.True(t, e.IsJson)

	var payload yaklangCodeChangeResponse
	require.NoError(t, json.Unmarshal(e.Content, &payload))
	return payload
}

// TestYakRunnerProtocol_3_FullFreeInputModifyWithAllAttachments verifies the「完整案例（后续输入）」：
//
//	FreeInput: ":codeBlockTag[scan.yak (10-18)] ..."
//	FocusModeLoop: write_yaklang_code
//	AttachedResourceInfo: directory_path + file_path + selected/content
func TestYakRunnerProtocol_3_FullFreeInputModifyWithAllAttachments(t *testing.T) {
	dir := t.TempDir()
	yakPath := writeScanYakFile(t, dir)

	selection, err := json.Marshal(map[string]any{
		"path":      yakPath,
		"startLine": 1,
		"endLine":   4,
		"language":  "yak",
		"content":   sampleScanYak,
	})
	require.NoError(t, err)

	mock := &yakRunnerModifyMock{}
	userQuery := fmt.Sprintf(":codeBlockTag[%s (1-4)] 修正 synscan 超时参数", filepath.Base(yakPath))

	result := runYakRunnerProtocolScenario(
		t, mock.callback, userQuery,
		yakRunnerFullAttachments(dir, yakPath, string(selection)),
	)
	t.Log("timeline:\n", result.timeline)

	require.Greater(t, mock.modifyAttempts, 0, "AI should attempt modify_code")
	require.False(t, result.taskFailed, "task should complete when attachment seeds full_code")
	require.NotContains(t, result.timeline, "line number out of range")
	require.Contains(t, result.timeline, "modify_success")

	disk, readErr := os.ReadFile(yakPath)
	require.NoError(t, readErr)
	require.Contains(t, string(disk), `// timeout was 60`, "delivery file should be updated after loop finishes")
	require.NotContains(t, string(disk), `// timeout was 30`)

	require.GreaterOrEqual(t, len(result.codeChangeEvents), 1, "should emit yaklang_code_change when loop finishes")
	assert.Len(t, result.codeChangeEvents, 1, "exactly one yaklang_code_change per conversation")
	assertYakRunnerNoYakPinFilenameEvents(t, result.pinFilenameEvents)
}

// TestYakRunnerProtocol_4_YaklangCodeChangeResponseShape verifies the「返回数据」协议字段。
func TestYakRunnerProtocol_4_YaklangCodeChangeResponseShape(t *testing.T) {
	dir := t.TempDir()
	yakPath := writeScanYakFile(t, dir)

	selection, err := json.Marshal(map[string]any{
		"path":      yakPath,
		"startLine": 1,
		"endLine":   4,
		"language":  "yak",
		"content":   sampleScanYak,
	})
	require.NoError(t, err)

	mock := &yakRunnerModifyMock{}
	userQuery := fmt.Sprintf(":codeBlockTag[%s (1-4)] 修正 synscan 超时参数", filepath.Base(yakPath))

	result := runYakRunnerProtocolScenario(
		t, mock.callback, userQuery,
		yakRunnerFullAttachments(dir, yakPath, string(selection)),
	)
	require.NotEmpty(t, result.codeChangeEvents)

	last := result.codeChangeEvents[len(result.codeChangeEvents)-1]
	payload := parseYaklangCodeChangeResponse(t, last)

	assert.Equal(t, "replace", payload.Op)
	assert.Equal(t, "modify_code", payload.SourceAction)
	assert.Equal(t, "修正 synscan 超时参数", payload.Reason)
	assert.Equal(t, filepath.Clean(yakPath), filepath.Clean(payload.Code.Path))
	assert.Contains(t, payload.Code.Content, `// timeout was 60`)
	assert.NotEmpty(t, payload.Code.Summary)
	assert.Greater(t, payload.Code.Version, 0)

	// Deferred editor sync: only one final yaklang_code_change after the loop completes.
	assert.Len(t, result.codeChangeEvents, 1)
}

// TestYakRunnerProtocol_5_NoAttachmentsEmitsCreateOp verifies preview mode when
// AttachedResourceInfo is completely absent: final yaklang_code_change uses op create.
func TestYakRunnerProtocol_5_NoAttachmentsEmitsCreateOp(t *testing.T) {
	base := t.TempDir()
	t.Setenv("YAKIT_HOME", base)

	result := runYakRunnerProtocolScenario(
		t, yakRunnerPreviewWriteCallback, "写一个 hello yak 脚本", nil,
	)
	require.False(t, result.taskFailed, "preview task should complete")
	require.Len(t, result.codeChangeEvents, 1, "exactly one yaklang_code_change per conversation")
	assertYakRunnerNoYakPinFilenameEvents(t, result.pinFilenameEvents)

	payload := parseYaklangCodeChangeResponse(t, result.codeChangeEvents[0])
	assert.Equal(t, "create", payload.Op)
	assert.Contains(t, payload.Code.Content, "hello yak")
	assert.Contains(t, filepath.Base(payload.Code.Path), "gen_code_")
	assert.True(t, strings.HasSuffix(strings.ToLower(payload.Code.Path), ".yak"))
}

// TestYakRunnerProtocol_6_DirectoryPathOnlyEmitsCreateOp verifies preview mode when
// only directory_path is attached (no open file): final yaklang_code_change uses op create.
func TestYakRunnerProtocol_6_DirectoryPathOnlyEmitsCreateOp(t *testing.T) {
	base := t.TempDir()
	t.Setenv("YAKIT_HOME", base)
	dir := filepath.Join(base, "workspace")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	result := runYakRunnerProtocolScenario(
		t, yakRunnerPreviewWriteCallback, "写一个 hello yak 脚本",
		yakRunnerDirectoryOnlyAttachments(dir),
	)
	require.False(t, result.taskFailed, "directory-only preview task should complete")
	require.Len(t, result.codeChangeEvents, 1, "exactly one yaklang_code_change per conversation")
	assertYakRunnerNoYakPinFilenameEvents(t, result.pinFilenameEvents)

	payload := parseYaklangCodeChangeResponse(t, result.codeChangeEvents[0])
	assert.Equal(t, "create", payload.Op)
	assert.Contains(t, payload.Code.Content, "hello yak")
	assert.Contains(t, filepath.Base(payload.Code.Path), "gen_code_")
	assert.True(t, strings.HasSuffix(strings.ToLower(payload.Code.Path), ".yak"))
}

// TestYakRunnerProtocol_7_DirectoryAndNamedFileInQuery writes directly to the file named in FreeInput.
func TestYakRunnerProtocol_7_DirectoryAndNamedFileInQuery(t *testing.T) {
	dir := t.TempDir()
	yakPath := filepath.Join(dir, "123.yak")
	require.NoError(t, os.WriteFile(yakPath, []byte(""), 0o644))

	result := runYakRunnerProtocolScenario(
		t, yakRunnerPreviewWriteCallback, "在当前打开的123.yak文件里生成一份端口扫描的yak代码",
		yakRunnerDirectoryOnlyAttachments(dir),
	)
	require.False(t, result.taskFailed, "named-file task should complete")
	require.Len(t, result.codeChangeEvents, 1, "exactly one yaklang_code_change per conversation")

	payload := parseYaklangCodeChangeResponse(t, result.codeChangeEvents[0])
	assert.Equal(t, "replace", payload.Op)
	assert.Equal(t, filepath.Clean(yakPath), filepath.Clean(payload.Code.Path))
	assert.Contains(t, payload.Code.Content, "hello")

	disk, readErr := os.ReadFile(yakPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(disk), "hello")
}
