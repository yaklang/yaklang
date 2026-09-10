package tests

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Exercise the production input/event path with a deterministic model callback:
// queue a follow-up, remove it, stop the running root, then send another message.
func TestResilienceStopAfterRemovingQueuedTasksFinishesEngine(t *testing.T) {
	for _, operation := range []string{"clear", "remove"} {
		t.Run(operation, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			started := make(chan struct{})
			var once sync.Once
			var continueConversation atomic.Bool
			answer := mockedAnswerThenFinish("continued")
			events := make(chan *schema.AiOutputEvent, 256)
			engine := newTestAIEngine(t, func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				if continueConversation.Load() {
					return answer(config, request)
				}
				once.Do(func() { close(started) })
				requestContext := request.GetContext()
				if requestContext == nil {
					requestContext = config.GetContext()
				}
				<-requestContext.Done()
				return nil, requestContext.Err()
			}, aiengine.WithContext(ctx), aiengine.WithStateless(true),
				aiengine.WithDisableToolUse(true), aiengine.WithDisableAIForge(true),
				aiengine.WithYOLOMode(), aiengine.WithWorkdir(t.TempDir()),
				aiengine.WithOnEvent(func(_ aicommon.AIEngineOperator, event *schema.AiOutputEvent) {
					if event.Type != schema.EVENT_TYPE_STRUCTURED {
						return
					}
					select {
					case events <- event:
					case <-ctx.Done():
					}
				}))
			defer engine.Close()
			waitEvent := func(match func(*schema.AiOutputEvent) bool) *schema.AiOutputEvent {
				t.Helper()
				for {
					select {
					case event := <-events:
						if match(event) {
							return event
						}
					case <-ctx.Done():
						t.Fatal("timed out waiting for runtime event")
						return nil
					}
				}
			}
			rootDone := make(chan error, 1)
			go func() { rootDone <- engine.SendMsg("running root") }()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("root did not start")
			}
			if err := engine.SendInputEvent(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: "queued follow-up"}); err != nil {
				t.Fatal(err)
			}
			var queuedTaskID string
			waitEvent(func(event *schema.AiOutputEvent) bool {
				var payload map[string]string
				_ = json.Unmarshal(event.Content, &payload)
				if event.NodeId == "react_task_created" && payload["react_user_input"] == "queued follow-up" {
					queuedTaskID = payload["react_task_id"]
				}
				return queuedTaskID != "" && event.NodeId == "react_task_enqueue" && payload["react_task_id"] == queuedTaskID
			})
			control := &ypb.AIInputEvent{IsSyncMessage: true, SyncID: "remove-queued", SyncType: "react_clear_task"}
			if operation == "remove" {
				control.SyncType = "react_remove_task"
				payload, _ := json.Marshal(map[string]string{"task_id": queuedTaskID})
				control.SyncJsonInput = string(payload)
			}
			if err := engine.SendInputEvent(control); err != nil {
				t.Fatal(err)
			}
			waitEvent(func(event *schema.AiOutputEvent) bool { return event.IsSync && event.SyncID == control.SyncID })
			if err := engine.SendInputEvent(&ypb.AIInputEvent{IsSyncMessage: true, SyncID: "stop-root", SyncType: "react_cancel_current_task"}); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-rootDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("stop did not release the root message")
			}
			allDone := make(chan error, 1)
			go func() { allDone <- engine.WaitTaskFinish() }()
			select {
			case err := <-allDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("stop left removed queue tasks active: %d", engine.GetActiveTaskCount())
			}
			if engine.Context().Err() != nil {
				t.Fatal("stop closed the conversation")
			}
			continueConversation.Store(true)
			if err := engine.SendMsg("continue conversation"); err != nil {
				t.Fatalf("follow-up after stop: %v", err)
			}
			if err := engine.WaitTaskFinish(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
