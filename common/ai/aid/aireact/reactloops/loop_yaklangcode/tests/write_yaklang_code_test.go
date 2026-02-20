package yaklangcodetests

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
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

func mockedYaklangWriting(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, code string) (*aicommon.AIResponse, error) {
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
		re := regexp.MustCompile(`<\|GEN_CODE_([^|]+)\|>`)
		matches := re.FindStringSubmatch(prompt)
		var nonceStr string
		if len(matches) > 1 {
			nonceStr = matches[1]
		}
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(utils.MustRenderTemplate(`{"@action": "write_code"}

<|GEN_CODE_{{ .nonce }}|>
println("a")
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
			return mockedYaklangWriting(i, r, "sleep")
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

	du := time.Duration(3)
	if utils.InGithubActions() {
		du = time.Duration(2)
	}
	after := time.After(du * time.Second)

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR) {
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	fmt.Println("--------------------------------------")
	tl := ins.DumpTimeline()
	fmt.Println(tl)
	fmt.Println("--------------------------------------")
}

type mockStats_forWriteAndModify struct {
	writeDone    bool
	verifyCalled bool
}

func mockedYaklangWritingAndModify(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, code string, stat *mockStats_forWriteAndModify) (*aicommon.AIResponse, error) {
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
		re := regexp.MustCompile(`<\|GEN_CODE_([^|]+)\|>`)
		matches := re.FindStringSubmatch(prompt)
		var nonceStr string
		if len(matches) > 1 {
			nonceStr = matches[1]
		}
		rsp := i.NewAIResponse()
		if !stat.writeDone {
			rsp.EmitOutputStream(bytes.NewBufferString(utils.MustRenderTemplate(`{"@action": "write_code"}

<|GEN_CODE_{{ .nonce }}|>
println("a")
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

func TestFocusMode_WriteYaklangCodeAndThenModify(t *testing.T) {
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	stat := &mockStats_forWriteAndModify{
		writeDone: false,
	}
	ins, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedYaklangWritingAndModify(i, r, "demo", stat)
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

	du := time.Duration(3)
	if utils.InGithubActions() {
		du = time.Duration(2)
	}
	after := time.After(du * time.Second)

	var filenames []string
	var writeCodeReceived bool
LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				content := string(e.GetContent())
				filename := utils.InterfaceToString(jsonpath.FindFirst(content, "$.path"))
				filenames = append(filenames, filename)
			}
			if e.Type == string(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR) {
				if e.GetNodeId() == "write_code" {
					writeCodeReceived = true
					break LOOP
				}
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !writeCodeReceived {
		t.Fatal("write_code event not received")
	}

	var filename string
	for _, name := range filenames {
		if strings.Contains(name, "gen_code_") {
			filename = name
			break
		}
	}
	if filename == "" {
		t.Fatal("gen_code_ filename not found")
	}
	fmt.Println("--------------------------------------")
	tl := ins.DumpTimeline()
	fmt.Println(tl)
	fmt.Println("--------------------------------------")

	result, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(result))
	expectedCode := `println("a")
println("b")
println("c")`
	if !strings.Contains(string(result), "println") {
		t.Fatal("code not written correctly")
	}
	if strings.Contains(string(result), "for for for") {
		t.Fatal("code should not contain syntax errors")
	}
	_ = expectedCode
}
