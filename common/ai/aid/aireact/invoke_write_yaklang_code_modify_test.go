package aireact

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
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type mockStats_forWriteAndModify struct {
	writeDone bool
}

func mockedYaklangWritingAndModify(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, code string, stat *mockStats_forWriteAndModify) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()
	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool", `"write_yaklang_code"`) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "write_yaklang_code", "write_yaklang_code_approach": "` + code + `" },
"human_readable_thought": "mocked thought for write-yaklang", "cumulative_summary": "..cumulative-mocked for write-yaklang calling.."}
`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "abc-mocked-reason"}`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, `"query_document"`, `"require_tool"`, `"write_code"`, `"@action"`) {
		// extract nonce from <|GEN_CODE_{{.Nonce}}|>
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

	fmt.Println("Unexpected prompt:", prompt)

	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func TestReAct_WriteYaklangCodeAndThenModify(t *testing.T) {
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	stat := &mockStats_forWriteAndModify{
		writeDone: false,
	}
	ins, err := NewTestReAct(
		WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedYaklangWritingAndModify(i, r, "demo", stat)
		}),
		WithEventInputChan(in),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins
	go func() {
		for i := 0; i < 1; i++ {
			in <- &ypb.AIInputEvent{
				IsFreeInput: true,
				FreeInput:   "abc",
			}
		}
	}()

	du := time.Duration(50)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	var filename string
LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				content := string(e.GetContent())
				filename = utils.InterfaceToString(jsonpath.FindFirst(content, "$.path"))
			}
			if e.Type == string(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR) {
				if e.GetNodeId() == "modify_code" {
					break LOOP
				}
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	fmt.Println("--------------------------------------")
	tl := ins.DumpTimeline()
	fmt.Println(tl)
	if !utils.MatchAllOfSubString(tl, "mocked thought for write-yaklang") {
		t.Fatal("timeline not match")
	}
	fmt.Println("--------------------------------------")

	result, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(result))
	if !strings.Contains(string(result), "modifiedcodecodecode") {
		t.Fatal("modified code not match")
	}
}
