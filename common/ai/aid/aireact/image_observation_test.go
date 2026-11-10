package aireact

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// This exercises the real loop, tool invocation/artifact ingress and next model
// request, using deterministic model callbacks and a synthetic screenshot. It is
// not a real-model autonomous browser acceptance test.
func TestImageToolObservationLoop(t *testing.T) {
	for _, route := range []string{"native", "describe"} {
		for _, functionCall := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/functioncall=%v", route, functionCall), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				var imageBytes bytes.Buffer
				require.NoError(t, png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 16, 12))))
				var mainCalls, visionCalls, toolCalls atomic.Int32
				tool, err := aitool.New("fixture_screenshot", aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
					toolCalls.Add(1)
					return aitool.ImageResult(ctx, imageBytes.Bytes(), "image/png", "screenshot captured; success must be determined from the pixels")
				}))
				require.NoError(t, err)
				main := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					if strings.Contains(req.GetPrompt(), `"tool-call-reason"`) {
						resp := aicommon.NewAIResponse(c)
						resp.EmitOutputStream(strings.NewReader(`{"@action":"tool-call-reason","reason":"capture screen"}`))
						resp.Close()
						return resp, nil
					}
					call := mainCalls.Add(1)
					action := "capture_fixture"
					if call > 1 {
						action = "fixture_done"
						if route == "native" {
							require.Len(t, req.GetImageList(), 1)
							require.Equal(t, imageBytes.Bytes(), req.GetImageList()[0].Data)
							require.Contains(t, req.GetPrompt(), "<image_attachments>")
							require.Contains(t, req.GetPrompt(), "fixture_screenshot")
						} else {
							require.Empty(t, req.GetImageList())
							require.Contains(t, req.GetPrompt(), "Saved is visible in the screenshot")
						}
					}
					resp := aicommon.NewAIResponse(c)
					if functionCall {
						wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
						require.NotNil(t, wire.ToolCallCallback)
						wire.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprintf("fixture-%d", call), Type: "function", Function: aispec.FuncReturn{Name: action, Arguments: `{}`}}})
						wire.FinishReasonCallback("tool_calls", nil)
					} else {
						resp.EmitOutputStream(strings.NewReader(fmt.Sprintf(`{"@action":%q}`, action)))
					}
					resp.Close()
					return resp, nil
				}
				vision := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					visionCalls.Add(1)
					require.Len(t, req.GetImageList(), 1)
					resp := aicommon.NewAIResponse(c)
					resp.EmitOutputStream(strings.NewReader("Saved is visible in the screenshot; no other state is verified."))
					resp.Close()
					return resp, nil
				}
				react, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(t.TempDir()),
					aicommon.WithAICallback(main), aicommon.WithVisionPriorityAICallback(vision),
					aicommon.WithImageInputMode(route), aicommon.WithTools(tool), aicommon.WithAgreeYOLO(),
					aicommon.WithDisableToolCallerIntervalReview(true), aicommon.WithDisableAutoSkills(true))
				require.NoError(t, err)
				loop, err := reactloops.NewReActLoop("image-observation-fixture", react,
					reactloops.WithFunctionCallMode(functionCall), reactloops.WithMaxIterations(3),
					reactloops.WithRegisterLoopAction("capture_fixture", "Capture a synthetic screenshot", nil, nil,
						func(_ *reactloops.ReActLoop, _ *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
							_, _, invokeErr := react.ExecuteToolRequiredAndCallWithoutRequired(ctx, tool.Name, aitool.InvokeParams{})
							if invokeErr != nil {
								op.Fail(invokeErr)
								return
							}
							op.Continue()
						}),
					reactloops.WithRegisterLoopAction("fixture_done", "Finish after observing the screenshot", nil, nil,
						func(_ *reactloops.ReActLoop, _ *aicommon.Action, op *reactloops.LoopActionHandlerOperator) { op.Exit() }),
				)
				require.NoError(t, err)
				require.NoError(t, loop.Execute("image-flow", ctx, "Capture the screen and verify whether Saved is visible."))
				require.Equal(t, int32(1), toolCalls.Load())
				require.Equal(t, int32(2), mainCalls.Load())
				if route == "native" {
					require.Zero(t, visionCalls.Load(), "native consumption does not force a second visual interpretation")
				} else {
					require.Equal(t, int32(1), visionCalls.Load())
					require.Contains(t, react.config.Timeline.DumpForPrompt(), "[image_observation]")
				}
			})
		}
	}
}
