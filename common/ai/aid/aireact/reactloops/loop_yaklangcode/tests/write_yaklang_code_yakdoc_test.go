package yaklangcodetests

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type mockStats_forYakdoc struct {
	yakdocDone  bool
	codeWritten bool
}

func mockedYaklangYakdocFlow(t *testing.T, i aicommon.AICallerConfigIf, req *aicommon.AIRequest, stat *mockStats_forYakdoc) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()

	if utils.MatchAllOfSubString(prompt, "analyze-requirement-and-search", "create_new_file") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "analyze-requirement-and-search",
  "create_new_file": true
}`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, `"yakdoc_function_details"`, `"require_tool"`, `"write_code"`, `"@action"`) {
		nonceStr := aicommon.MustExtractDynamicSectionNonce(t, prompt)
		rsp := i.NewAIResponse()

		if !stat.yakdocDone {
			rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "yakdoc_function_details",
  "library": "str",
  "function": ["Split"],
  "human_readable_thought": "Query str.Split API before writing code"
}`))
			stat.yakdocDone = true
			rsp.Close()
			return rsp, nil
		}

		if !stat.codeWritten {
			rsp.EmitOutputStream(bytes.NewBufferString(utils.MustRenderTemplate(`{"@action": "write_code"}

<|GEN_CODE_{{ .nonce }}|>
parts = str.Split("a,b", ",")
println(parts)
<|GEN_CODE_END_{{ .nonce }}|>`, map[string]any{
				"nonce": nonceStr,
			})))
			stat.codeWritten = true
			rsp.Close()
			return rsp, nil
		}

		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
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

func TestFocusMode_YakdocThenWriteCode(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)
	stat := &mockStats_forYakdoc{}

	ins, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedYaklangYakdocFlow(t, i, r, stat)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	require.NoError(t, err)

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput:   true,
			FreeInput:     "use str.Split to split a string",
			FocusModeLoop: schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG,
		}
	}()

	du := 5 * time.Second
	if utils.InGithubActions() {
		du = 3 * time.Second
	}
	after := time.After(du)

	var editorSeen bool
	var yakdocTimelineSeen bool
LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR) {
				editorSeen = true
			}
			// EmitThoughtStream uses nodeId "re-act-loop-thought"; stream key is in TaskIndex.
			if e.GetTaskIndex() == "yakdoc_function_details_result" ||
				utils.MatchAllOfSubString(e.GetTaskIndex(), "yakdoc_function_details") {
				yakdocTimelineSeen = true
			}
			if stat.codeWritten && editorSeen {
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	require.True(t, stat.yakdocDone, "yakdoc_function_details should be invoked")
	require.True(t, stat.codeWritten, "write_code should be invoked after yakdoc")
	require.True(t, editorSeen, "code editor event should be emitted")
	require.True(t, yakdocTimelineSeen, "yakdoc timeline node should be present")

	tl := ins.DumpTimeline()
	require.Contains(t, tl, "yakdoc_function_details")
	require.Contains(t, tl, "str.Split")
}

func TestWriteYaklangLoopPromptContainsYakdocActions(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	ins, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()
			require.Contains(t, prompt, "yakdoc_get_all_library_names")
			require.Contains(t, prompt, "yakdoc_library_details")
			require.Contains(t, prompt, "yakdoc_function_details")
			require.Contains(t, prompt, "yakdoc_variable_details")
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	require.NoError(t, err)

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput:   true,
			FreeInput:     "quick prompt check",
			FocusModeLoop: schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG,
		}
	}()

	time.Sleep(2 * time.Second)
	close(in)
	_ = ins
}
