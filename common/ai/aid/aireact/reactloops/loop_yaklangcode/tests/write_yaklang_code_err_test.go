package yaklangcodetests

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func mockedYaklangWritingAndModifyCauseError(t *testing.T, i aicommon.AICallerConfigIf, req *aicommon.AIRequest, code string, stat *mockStats_forWriteAndModify) (*aicommon.AIResponse, error) {
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

	if utils.MatchAllOfSubString(prompt, `"grep_yaklang_samples"`, `"require_tool"`, `"write_code"`, `"@action"`) {
		nonceStr := aicommon.MustExtractDynamicSectionNonce(t, prompt)
		rsp := i.NewAIResponse()
		if !stat.writeDone {
			rsp.EmitOutputStream(bytes.NewBufferString(utils.MustRenderTemplate(`{"@action": "write_code"}

<|GEN_CODE_{{ .nonce }}|>
println("a")
for for for
println("b")
println("c")
<|GEN_CODE_END_{{ .nonce }}|>`, map[string]any{
				"nonce": nonceStr,
			})))
			stat.writeDone = true
		} else {
			rsp.EmitOutputStream(bytes.NewBufferString(utils.MustRenderTemplate(`{"@action": "modify_code", "modify_start_line": 2, "modify_end_line": 2}

<|GEN_CODE_{{ .nonce }}|>
println("modifiedcodecodecode")
<|GEN_CODE_END_{{ .nonce }}|>`, map[string]any{
				"nonce": nonceStr,
			})))
			stat.modifyDone = true
		}

		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, `"@action"`, `"create_new_file"`, `"check-filepath"`, `"existed_filepath"`) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "check-filepath", "create_new_file": true}`))
		rsp.Close()
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", prompt)
	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func TestFocusMode_WriteYaklangCodeCauseErrorAndThenModify(t *testing.T) {
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 100)

	var haveError bool

	stat := &mockStats_forWriteAndModify{
		writeDone: false,
	}
	ins, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			sample := `[Error]: 基础语法错误（Syntax Error）`
			if strings.Contains(r.GetPrompt(), sample) {
				if !haveError {
					haveError = true
				}
			}
			return mockedYaklangWritingAndModifyCauseError(t, i, r, "demo", stat)
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

	// Syntax-error scenario may never finish the loop; wait for deferred yaklang_code_change instead of disk.
	waitResult := waitForYaklangDeferredEditorSync(out, focusModeWriteYaklangTestTimeout())
	close(in)
	ins.Wait()

	if !stat.modifyDone {
		t.Fatal("mock modify_code was not invoked before timeout")
	}

	if len(waitResult.codeChangeEvents) > 0 {
		lastChange := waitResult.codeChangeEvents[len(waitResult.codeChangeEvents)-1]
		finalContent := utils.InterfaceToString(jsonpath.FindFirst(string(lastChange.GetContent()), "$.code.content"))
		if !strings.Contains(finalContent, "modifiedcodecodecode") {
			t.Fatalf("deferred yaklang_code_change content mismatch: %q", finalContent)
		}
	}

	filename := findGenCodeFilename(waitResult.filenames)
	if filename == "" {
		t.Fatal("gen_code_ filename not found")
	}
	if _, err := os.Stat(filename); err == nil {
		if data, readErr := os.ReadFile(filename); readErr == nil && len(data) > 0 {
			t.Fatalf("disk should not be written during loop; found %d bytes in %s", len(data), filename)
		}
	}

	fmt.Println("--------------------------------------")
	tl := ins.DumpTimeline()
	fmt.Println(tl)
	fmt.Println("--------------------------------------")

	if !haveError {
		t.Fatal("should have error, but not found, maybe the write_code then check syntax not work?")
	}
}
