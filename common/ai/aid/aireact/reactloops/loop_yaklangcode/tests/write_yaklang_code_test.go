package yaklangcodetests

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func focusModeWriteYaklangTestTimeout() time.Duration {
	if utils.InGithubActions() {
		return 15 * time.Second
	}
	return 10 * time.Second
}

type yaklangDeferredEditorSyncWaitResult struct {
	codeChangeEvents []*ypb.AIOutputEvent
	filenames        []string
	taskFailed       bool
	taskCompleted    bool
}

func isYaklangReactTaskTerminalStatus(content string) (failed, completed bool) {
	status := strings.ToLower(strings.TrimSpace(utils.InterfaceToString(jsonpath.FindFirst(content, "$..react_task_now_status"))))
	switch status {
	case "aborted", "failed":
		return true, false
	case "completed", "skipped":
		return false, true
	}
	if strings.Contains(content, "Aborted") || strings.Contains(content, "Failed") {
		return true, false
	}
	if strings.Contains(content, `"react_task_now_status":"completed"`) ||
		strings.Contains(content, `"react_task_now_status": "completed"`) {
		return false, true
	}
	return false, false
}

// waitForYaklangDeferredEditorSync collects output until the focus-mode task finishes.
// Terminal signal: react task status (completed/failed). code_change events are collected
// for assertions but must not end the wait — replace-mode live edits would race modify.
func waitForYaklangDeferredEditorSync(out <-chan *ypb.AIOutputEvent, timeout time.Duration) yaklangDeferredEditorSyncWaitResult {
	deadline := time.After(timeout)
	var result yaklangDeferredEditorSyncWaitResult
	for {
		select {
		case e := <-out:
			if e == nil {
				continue
			}
			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				content := string(e.GetContent())
				filename := utils.InterfaceToString(jsonpath.FindFirst(content, "$.path"))
				result.filenames = append(result.filenames, filename)
			}
			if e.Type == string(schema.EVENT_TYPE_YAKLANG_CODE_CHANGE) {
				result.codeChangeEvents = append(result.codeChangeEvents, e)
			}
			if e.GetNodeId() == "react_task_status_changed" {
				content := string(e.GetContent())
				if failed, completed := isYaklangReactTaskTerminalStatus(content); failed {
					result.taskFailed = true
					return result
				} else if completed {
					result.taskCompleted = true
					return result
				}
			}
		case <-deadline:
			return result
		}
	}
}

func mockYaklangFinalizeDirectlyAnswer(t *testing.T, i aicommon.AICallerConfigIf, prompt string) (*aicommon.AIResponse, bool) {
	if !strings.Contains(prompt, "AI_CACHE_SYSTEM") {
		return nil, false
	}
	if !(strings.Contains(prompt, "Yaklang") || strings.Contains(prompt, "代码生成") || strings.Contains(prompt, "write_yaklang_code")) {
		return nil, false
	}
	nonceStr := aicommon.MustExtractDynamicSectionNonce(t, prompt)
	rsp := i.NewAIResponse()
	rsp.EmitOutputStream(bytes.NewBufferString(utils.MustRenderTemplate(`{"@action": "directly_answer"}

<|FINAL_ANSWER_{{ .nonce }}|>
代码生成完成。
<|FINAL_ANSWER_END_{{ .nonce }}|>`, map[string]any{
		"nonce": nonceStr,
	})))
	rsp.Close()
	return rsp, true
}

func mockYaklangModifyCodeResponse(t *testing.T, i aicommon.AICallerConfigIf, prompt string, stat *mockStats_forWriteAndModify, modifyBody string) (*aicommon.AIResponse, bool) {
	if stat == nil || !stat.writeDone || stat.modifyDone {
		return nil, false
	}
	if !utils.MatchAllOfSubString(prompt, `"modify_code"`, `"require_tool"`, `"@action"`) {
		return nil, false
	}
	nonceStr := aicommon.MustExtractDynamicSectionNonce(t, prompt)
	rsp := i.NewAIResponse()
	rsp.EmitOutputStream(bytes.NewBufferString(utils.MustRenderTemplate(`{"@action": "modify_code", "modify_start_line": 2, "modify_end_line": 2, "modify_code_reason": "replace line b"}

<|GEN_CODE_{{ .nonce }}|>
{{ .body }}
<|GEN_CODE_END_{{ .nonce }}|>`, map[string]any{
		"nonce": nonceStr,
		"body":  modifyBody,
	})))
	stat.modifyDone = true
	rsp.Close()
	return rsp, true
}

func mockYaklangWriteCodeResponse(t *testing.T, i aicommon.AICallerConfigIf, prompt string, stat *mockStats_forWriteAndModify, writeBody string) (*aicommon.AIResponse, bool) {
	if stat == nil || stat.writeDone {
		return nil, false
	}
	if !utils.MatchAllOfSubString(prompt, `"grep_yaklang_samples"`, `"require_tool"`, `"write_code"`, `"@action"`) {
		return nil, false
	}
	nonceStr := aicommon.MustExtractDynamicSectionNonce(t, prompt)
	rsp := i.NewAIResponse()
	rsp.EmitOutputStream(bytes.NewBufferString(utils.MustRenderTemplate(`{"@action": "write_code"}

<|GEN_CODE_{{ .nonce }}|>
{{ .body }}
<|GEN_CODE_END_{{ .nonce }}|>`, map[string]any{
		"nonce": nonceStr,
		"body":  writeBody,
	})))
	stat.writeDone = true
	rsp.Close()
	return rsp, true
}

func findGenCodeFilename(filenames []string) string {
	for _, name := range filenames {
		if strings.Contains(name, "gen_code_") {
			return name
		}
	}
	return ""
}

func previewGenCodeArtifactPath(pinFilenames []string, codeChangeEvents []*ypb.AIOutputEvent) string {
	for i := len(codeChangeEvents) - 1; i >= 0; i-- {
		path := utils.InterfaceToString(jsonpath.FindFirst(string(codeChangeEvents[i].GetContent()), "$.code.path"))
		if strings.Contains(filepath.Base(path), "gen_code_") {
			return path
		}
	}
	return findGenCodeFilename(pinFilenames)
}

func assertPreviewGenCodeArtifactPath(t *testing.T, pinFilenames []string, codeChangeEvents []*ypb.AIOutputEvent) string {
	t.Helper()
	path := previewGenCodeArtifactPath(pinFilenames, codeChangeEvents)
	if path == "" {
		t.Fatal("gen_code_ artifact path not found")
	}
	base := strings.ToLower(filepath.Base(path))
	if !strings.Contains(base, "gen_code_") || !strings.HasSuffix(base, ".yak") {
		t.Fatalf("preview artifact should be gen_code_*.yak, got %s", path)
	}
	return path
}

func mockedYaklangWriting(t *testing.T, i aicommon.AICallerConfigIf, req *aicommon.AIRequest, code string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()

	if utils.MatchAllOfSubString(prompt, "analyze-requirement-and-search", "create_new_file") {
		rsp := i.NewAIResponse()
		if utils.MatchAllOfSubString(prompt, "search_patterns", "Grep模式") {
			rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "analyze-requirement-and-search",
  "create_new_file": true,
  "search_patterns": ["println"],
  "reason": "Simple test code"
}`))
		} else {
			rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "analyze-requirement-and-search",
  "create_new_file": true
}`))
		}
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "extract-ranked-lines", "ranges", "rank", "reason") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "extract-ranked-lines",
  "ranges": [
    {"range": "1-3", "rank": 1, "reason": "Test code"}
  ]
}`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, `"grep_yaklang_samples"`, `"require_tool"`, `"write_code"`, `"@action"`) {
		nonceStr := aicommon.MustExtractDynamicSectionNonce(t, prompt)
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(utils.MustRenderTemplate(`{"@action": "write_code"}

<|GEN_CODE_{{ .nonce }}|>
// hello yak
<|GEN_CODE_END_{{ .nonce }}|>`, map[string]any{
			"nonce": nonceStr,
		})))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, `"@action"`, `"create_new_file"`, `"check-filepath"`, `"existed_filepath"`) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "check-filepath", "create_new_file": true}`))
		rsp.Close()
		return rsp, nil
	}

	if rsp, ok := mockYaklangFinalizeDirectlyAnswer(t, i, prompt); ok {
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", prompt)
	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func TestFocusMode_WriteYaklangCode(t *testing.T) {
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	ins, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedYaklangWriting(t, i, r, "sleep")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins
	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput:   true,
			FreeInput:     "abc",
			FocusModeLoop: schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG,
		}
	}()

	waitResult := waitForYaklangDeferredEditorSync(out, focusModeWriteYaklangTestTimeout())
	close(in)
	ins.Wait()

	if waitResult.taskFailed {
		t.Fatal("write_yaklang_code task failed")
	}
	if len(waitResult.codeChangeEvents) == 0 {
		t.Fatal("deferred yaklang_code_change event not received after loop finished")
	}
	if len(waitResult.codeChangeEvents) != 1 {
		t.Fatalf("expected exactly 1 deferred yaklang_code_change, got %d", len(waitResult.codeChangeEvents))
	}

	lastChange := waitResult.codeChangeEvents[0]
	op := utils.InterfaceToString(jsonpath.FindFirst(string(lastChange.GetContent()), "$.op"))
	if op != "create" {
		t.Fatalf("preview mode deferred yaklang_code_change should use op create, got %q", op)
	}

	fmt.Println("--------------------------------------")
	tl := ins.DumpTimeline()
	fmt.Println(tl)
	fmt.Println("--------------------------------------")
}

type mockStats_forWriteAndModify struct {
	writeDone    bool
	modifyDone   bool
	verifyCalled bool
}

func mockedYaklangWritingAndModify(t *testing.T, i aicommon.AICallerConfigIf, req *aicommon.AIRequest, code string, stat *mockStats_forWriteAndModify) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()

	if utils.MatchAllOfSubString(prompt, "analyze-requirement-and-search", "create_new_file") {
		rsp := i.NewAIResponse()
		if utils.MatchAllOfSubString(prompt, "search_patterns", "Grep模式") {
			rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "analyze-requirement-and-search",
  "create_new_file": true,
  "search_patterns": ["println"],
  "reason": "Simple test code"
}`))
		} else {
			rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "analyze-requirement-and-search",
  "create_new_file": true
}`))
		}
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "extract-ranked-lines", "ranges", "rank", "reason") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "extract-ranked-lines",
  "ranges": [
    {"range": "1-3", "rank": 1, "reason": "Test code"}
  ]
}`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "KNOWLEDGE_CHUNK", "ranges", "score") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "knowledge-compress",
  "ranges": [
    {"range": "1-3", "score": 0.9}
  ]
}`))
		rsp.Close()
		return rsp, nil
	}

	if rsp, ok := mockYaklangModifyCodeResponse(t, i, prompt, stat, "// modifiedcodecodecode"); ok {
		return rsp, nil
	}
	if rsp, ok := mockYaklangWriteCodeResponse(t, i, prompt, stat, "// line a\nfor for for\n// line c"); ok {
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, `"@action"`, `"create_new_file"`, `"check-filepath"`, `"existed_filepath"`) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "check-filepath", "create_new_file": true}`))
		rsp.Close()
		return rsp, nil
	}

	if rsp, ok := mockYaklangFinalizeDirectlyAnswer(t, i, prompt); ok {
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", prompt)
	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func TestFocusMode_WriteYaklangCodeAndThenModify(t *testing.T) {
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 100)

	stat := &mockStats_forWriteAndModify{
		writeDone: false,
	}
	ins, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedYaklangWritingAndModify(t, i, r, "demo", stat)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins
	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput:   true,
			FreeInput:     "abc",
			FocusModeLoop: schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG,
		}
	}()

	waitResult := waitForYaklangDeferredEditorSync(out, focusModeWriteYaklangTestTimeout())
	close(in)
	ins.Wait()

	if !stat.writeDone {
		t.Fatal("mock write_code was not invoked")
	}
	if !stat.modifyDone {
		t.Fatal("mock modify_code was not invoked")
	}
	if waitResult.taskFailed {
		t.Fatal("write_yaklang_code task failed")
	}
	if len(waitResult.codeChangeEvents) == 0 {
		t.Fatal("deferred yaklang_code_change event not received after loop finished")
	}
	if len(waitResult.codeChangeEvents) != 1 {
		t.Fatalf("expected exactly 1 deferred yaklang_code_change, got %d", len(waitResult.codeChangeEvents))
	}

	lastChange := waitResult.codeChangeEvents[0]
	op := utils.InterfaceToString(jsonpath.FindFirst(string(lastChange.GetContent()), "$.op"))
	if op != "create" {
		t.Fatalf("preview mode deferred yaklang_code_change should use op create, got %q", op)
	}
	sourceAction := utils.InterfaceToString(jsonpath.FindFirst(string(lastChange.GetContent()), "$.source_action"))
	if sourceAction != "modify_code" {
		t.Fatalf("expected final source_action modify_code, got %q", sourceAction)
	}
	finalContent := utils.InterfaceToString(jsonpath.FindFirst(string(lastChange.GetContent()), "$.code.content"))
	if !strings.Contains(finalContent, "modifiedcodecodecode") {
		t.Fatalf("deferred yaklang_code_change content mismatch: %q", finalContent)
	}

	filename := assertPreviewGenCodeArtifactPath(t, waitResult.filenames, waitResult.codeChangeEvents)
	fmt.Println("--------------------------------------")
	tl := ins.DumpTimeline()
	fmt.Println(tl)
	fmt.Println("--------------------------------------")

	data, readErr := os.ReadFile(filename)
	if readErr != nil {
		t.Fatalf("read preview code file %s: %v", filename, readErr)
	}
	if !strings.Contains(string(data), "modifiedcodecodecode") {
		t.Fatalf("preview mode should persist final code to %s, got %q", filename, string(data))
	}
}
