package subagent

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func TestNestedSubTask_InnerCompleteDoesNotCancelParent(t *testing.T) {
	parent := aicommon.NewStatefulTaskBase("parent-scan-sql", "audit", context.Background(), nil)
	nested := newNestedSubTask(parent, "fast-context")
	require.NotEqual(t, parent.GetId(), nested.AIStatefulTaskBase.GetId())
	require.Equal(t, parent.GetId(), nested.GetId())

	nested.SetStatus(aicommon.AITaskState_Completed)

	select {
	case <-parent.GetContext().Done():
		t.Fatal("parent context must stay alive when nested sub-task completes")
	default:
	}
}

func TestNestedSubTask_SharesParentEmitter(t *testing.T) {
	rootEmitter := aicommon.NewEmitter("root", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		return e, nil
	})
	const categorySubID = "parent-cat-sub-xxe_ssrf-abcd"
	categoryEmitter := BuildForwardingEmitter(rootEmitter, categorySubID)
	parent := aicommon.NewSubTaskBaseWithOptions(
		aicommon.NewStatefulTaskBase("orchestrator", "audit", context.Background(), rootEmitter, true),
		categorySubID,
		"scan",
		aicommon.WithStatefulTaskBaseSubAgent(),
		aicommon.WithStatefulTaskBaseSkipTaskStatusChangeEmit(),
	)
	parent.SetEmitter(categoryEmitter)

	nested := newNestedSubTask(parent, "fast-context")
	require.Same(t, categoryEmitter, nested.GetEmitter())
}

func TestWithEmitterProcessorOnTask_SerializesConcurrentScopes(t *testing.T) {
	rootEmitter := aicommon.NewEmitter("root", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		return e, nil
	})
	parent := aicommon.NewStatefulTaskBase("parent", "input", context.Background(), rootEmitter, true)
	nested := newNestedSubTask(parent, "fast-context")

	var running int
	var maxRunning int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			aicommon.WithEmitterProcessorOnTask(nested, func(event *schema.AiOutputEvent) *schema.AiOutputEvent {
				return event
			}, func() {
				mu.Lock()
				running++
				if running > maxRunning {
					maxRunning = running
				}
				mu.Unlock()
				mu.Lock()
				running--
				mu.Unlock()
			})
		}()
	}
	wg.Wait()
	require.Equal(t, 1, maxRunning)
}
