package aiengine

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func newAttachmentCaptureEngine(t *testing.T, options ...AIEngineConfigOption) (*AIEngine, *[]*ypb.AIInputEvent) {
	t.Helper()
	engine := newEngineForTerminalTest(t)
	engine.config = NewAIEngineConfig(options...)
	var events []*ypb.AIInputEvent
	engine.operator = &aicommon.AIEngineOperatorBase{SendInputEventFunc: func(event *ypb.AIInputEvent) error {
		events = append(events, event)
		taskID := fmt.Sprintf("attachment-task-%d", len(events))
		engine.processOutputEvent(&schema.AiOutputEvent{
			Type: schema.EVENT_TYPE_STRUCTURED, NodeId: "react_task_created",
			Content: []byte(fmt.Sprintf(`{"react_task_id":%q,"react_task_status":"processing"}`, taskID)),
		})
		engine.processOutputEvent(&schema.AiOutputEvent{
			Type: schema.EVENT_TYPE_STRUCTURED, NodeId: "react_task_status_changed",
			Content: []byte(fmt.Sprintf(`{"react_task_id":%q,"react_task_now_status":"completed","react_task_old_status":"processing"}`, taskID)),
		})
		return nil
	}}
	return engine, &events
}

func TestAIEngineSendMsgAttachmentScope(t *testing.T) {
	for _, async := range []bool{false, true} {
		t.Run(fmt.Sprintf("async_%t", async), func(t *testing.T) {
			engine, events := newAttachmentCaptureEngine(t,
				WithSessionID("attachment-scope"), WithAttachedFileContent("engine attachment"),
			)
			engine.config.AttachedResources = append(engine.config.AttachedResources, nil)
			send := engine.SendMsg
			if async {
				send = engine.SendMsgAsync
			}
			if err := send("read the attached files", WithAttachedFileContent("message attachment"), WithAttachedFilePath("reference.txt")); err != nil {
				t.Fatal(err)
			}
			first := (*events)[0]
			if !first.GetIsFreeInput() || first.GetFreeInput() != "read the attached files" {
				t.Fatalf("input event changed: %#v", first)
			}
			resources := first.GetAttachedResourceInfo()
			if len(resources) != 3 {
				t.Errorf("engine and message resources: got %d, want 3", len(resources))
			} else {
				for index, expected := range []struct{ key, value string }{
					{aicommon.CONTEXT_PROVIDER_KEY_FILE_CONTENT, "engine attachment"},
					{aicommon.CONTEXT_PROVIDER_KEY_FILE_CONTENT, "message attachment"},
					{aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH, "reference.txt"},
				} {
					resource := resources[index]
					if resource.GetType() != aicommon.CONTEXT_PROVIDER_TYPE_FILE || resource.GetKey() != expected.key || resource.GetValue() != expected.value {
						t.Errorf("resource %d changed: %#v", index, resource)
					}
				}
			}
			if err := send("next message without per-message attachments"); err != nil {
				t.Fatal(err)
			}
			next := (*events)[1].GetAttachedResourceInfo()
			if len(next) != 1 || next[0].GetValue() != "engine attachment" {
				t.Errorf("next message must retain only the engine attachment: %#v", next)
			}
			if len(engine.config.AttachedResources) != 2 || engine.config.AttachedResources[1] != nil {
				t.Fatal("sending a message mutated the engine's attachment configuration")
			}
			other, otherEvents := newAttachmentCaptureEngine(t, WithSessionID("other-attachment-scope"))
			if err := other.SendMsg("separate session"); err != nil {
				t.Fatal(err)
			}
			if len((*otherEvents)[0].GetAttachedResourceInfo()) != 0 {
				t.Fatal("attachments leaked into a separate engine")
			}
		})
	}
}

func TestInvokeReActAttachmentOptionsAppliedOnce(t *testing.T) {
	const childFlag = "AIENGINE_ATTACHMENT_INVOKE_TEST_PROCESS"
	if os.Getenv(childFlag) != "1" {
		// The factory registration is process-global. Keep the mock in a child
		// test process so other engine tests retain their registered factory.
		binary, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		command := exec.Command(binary, "-test.run=^TestInvokeReActAttachmentOptionsAppliedOnce$", "-test.count=1", "-test.timeout=30s")
		command.Env = append(os.Environ(), childFlag+"=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("isolated InvokeReAct regression: %v\n%s", err, output)
		}
		return
	}
	var captured *ypb.AIInputEvent
	aicommon.RegisterReActAIEngineOperator(func(options ...aicommon.ConfigOption) (aicommon.AIEngineOperator, error) {
		config := &aicommon.Config{}
		for _, option := range options {
			if err := option(config); err != nil {
				return nil, err
			}
		}
		return &aicommon.AIEngineOperatorBase{SendInputEventFunc: func(event *ypb.AIInputEvent) error {
			if !event.GetIsFreeInput() {
				return nil
			}
			captured = event
			config.EventHandler(&schema.AiOutputEvent{
				Type: schema.EVENT_TYPE_STRUCTURED, NodeId: "react_task_created",
				Content: []byte(`{"react_task_id":"invoke-attachment","react_task_status":"processing"}`),
			})
			config.EventHandler(&schema.AiOutputEvent{
				Type: schema.EVENT_TYPE_STRUCTURED, NodeId: "react_task_status_changed",
				Content: []byte(`{"react_task_id":"invoke-attachment","react_task_now_status":"completed","react_task_old_status":"processing"}`),
			})
			return nil
		}}, nil
	})
	optionApplications := 0
	err := InvokeReAct("analyze the configured attachment",
		WithStateless(true), WithSessionID("invoke-attachment-fixture"),
		WithDisableToolUse(true), WithDisableAIForge(true), WithDisableMCPServers(true),
		WithEnableAISearchTool(false), WithEnableForgeSearchTool(false),
		func(config *AIEngineConfig) {
			optionApplications++
			WithAttachedFileContent("INVOKE_ATTACHMENT_ONCE")(config)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if optionApplications != 1 {
		t.Errorf("InvokeReAct reapplied engine options: got %d applications, want 1", optionApplications)
	}
	resources := captured.GetAttachedResourceInfo()
	if len(resources) != 1 || resources[0].GetValue() != "INVOKE_ATTACHMENT_ONCE" {
		t.Errorf("InvokeReAct must send its configured attachment exactly once: %#v", resources)
	}
}
