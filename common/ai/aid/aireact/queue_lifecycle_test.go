package aireact

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func TestTaskQueue_RemovalEmitsTerminalLifecycle(t *testing.T) {
	for _, operation := range []string{"clear", "remove"} {
		t.Run(operation, func(t *testing.T) {
			queue := NewTaskQueue(operation)
			var events []*schema.AiOutputEvent
			emitter := aicommon.NewEmitter(operation, func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
				events = append(events, event)
				return event, nil
			})
			newTask := func(id string) *aicommon.AIStatefulTaskBase {
				task := aicommon.NewStatefulTaskBase(id, "input", nil, emitter)
				t.Cleanup(func() { task.Cancel() })
				task.SetStatus(aicommon.AITaskState_Queueing)
				require.NoError(t, queue.Append(task))
				return task
			}
			running := newTask("running")
			require.Same(t, running, queue.GetFirst())
			running.SetStatus(aicommon.AITaskState_Processing)
			first := newTask("first")
			second := newTask("second")

			removed := []*aicommon.AIStatefulTaskBase{first}
			if operation == "clear" {
				queue.Clear()
				queue.Clear()
				removed = append(removed, second)
			} else {
				require.True(t, queue.RemoveTask(first.GetId()))
				require.False(t, queue.RemoveTask(first.GetId()))
				require.Same(t, second, queue.GetFirst())
				require.Equal(t, aicommon.AITaskState_Queueing, second.GetStatus())
				require.NoError(t, second.GetContext().Err())
			}

			require.True(t, queue.IsEmpty())
			require.False(t, queue.RemoveTask(running.GetId()))
			require.Equal(t, aicommon.AITaskState_Processing, running.GetStatus())
			require.NoError(t, running.GetContext().Err())
			for _, task := range removed {
				require.Equal(t, aicommon.AITaskState_Skipped, task.GetStatus())
				require.True(t, task.IsUserCancelled())
				require.ErrorIs(t, task.GetContext().Err(), context.Canceled)
				var terminalEvents []*schema.AiOutputEvent
				for _, event := range events {
					if event.TaskId == task.GetId() && event.NodeId == "react_task_status_changed" &&
						parsePayload(t, event.Content)["react_task_now_status"] == string(aicommon.AITaskState_Skipped) {
						terminalEvents = append(terminalEvents, event)
					}
				}
				require.Len(t, terminalEvents, 1, "removed task must emit exactly one terminal lifecycle event")
				require.Equal(t, task.GetUUID(), terminalEvents[0].TaskUUID)
				require.Equal(t, string(aicommon.AITaskState_Queueing), parsePayload(t, terminalEvents[0].Content)["react_task_old_status"])
			}
		})
	}
}

func TestTaskQueue_RemovalDetachesBeforeCallbacks(t *testing.T) {
	for _, operation := range []string{"clear", "remove"} {
		t.Run(operation, func(t *testing.T) {
			queue := NewTaskQueue(operation)
			callbackEntered := make(chan int, 1)
			releaseCallback := make(chan struct{})
			defer close(releaseCallback)
			emitter := aicommon.NewEmitter(operation, func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
				if event.TaskId == "first" && event.NodeId == "react_task_status_changed" {
					// A lifecycle observer can re-enter the queue and may block while
					// another goroutine enqueues or starts work.
					callbackEntered <- queue.Len()
					<-releaseCallback
				}
				return event, nil
			})
			first := aicommon.NewStatefulTaskBase("first", "input", nil, emitter)
			second := aicommon.NewStatefulTaskBase("second", "input", nil, nil)
			incoming := aicommon.NewStatefulTaskBase("incoming", "input", nil, nil)
			later := aicommon.NewStatefulTaskBase("later", "input", nil, nil)
			for _, task := range []*aicommon.AIStatefulTaskBase{first, second, incoming, later} {
				t.Cleanup(func() { task.Cancel() })
			}
			require.NoError(t, queue.Append(first))
			require.NoError(t, queue.Append(second))
			removedByHook := make(chan aicommon.AIStatefulTask, 1)
			queue.AddDequeueHook(func(task aicommon.AIStatefulTask, reason string) {
				if reason == "manual_remove" {
					// The existing removal hook must also run without the queue lock.
					removedByHook <- queue.GetFirst()
				}
			})
			operationDone := make(chan struct{})
			go func() {
				defer close(operationDone)
				if operation == "clear" {
					queue.Clear()
				} else {
					queue.RemoveTask(first.GetId())
				}
			}()

			select {
			case count := <-callbackEntered:
				if operation == "clear" {
					require.Zero(t, count, "clear must detach every queued task before emitting the first terminal event")
				} else {
					require.Equal(t, 1, count)
				}
			case <-time.After(time.Second):
				t.Fatal("terminal lifecycle callback was missing or deadlocked on queue re-entry")
			}
			// While the terminal callback is blocked, concurrent consumers must
			// see only retained/new work and cannot remove the detached task again.
			require.False(t, queue.RemoveTask(first.GetId()))
			require.NoError(t, queue.PrependToFirst(incoming))
			require.Same(t, incoming, queue.GetFirst())
			require.NoError(t, incoming.GetContext().Err())
			require.NoError(t, queue.Append(later))
			releaseCallback <- struct{}{}
			select {
			case <-operationDone:
			case <-time.After(time.Second):
				t.Fatal("removal hook deadlocked on queue re-entry")
			}
			require.Equal(t, aicommon.AITaskState_Skipped, first.GetStatus())
			if operation == "clear" {
				require.Equal(t, aicommon.AITaskState_Skipped, second.GetStatus())
			} else {
				select {
				case task := <-removedByHook:
					require.Same(t, second, task)
				default:
					t.Fatal("removal did not invoke the existing dequeue hook")
				}
				require.NoError(t, second.GetContext().Err())
			}
			require.Same(t, later, queue.GetFirst())
			require.NoError(t, later.GetContext().Err())
			require.True(t, queue.IsEmpty())
		})
	}
}
