package test

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/schema"
)

func TestAiTask_GenerateIndex(t *testing.T) {
	// Test case 1: Nil task
	t.Run("NilTask", func(t *testing.T) {
		var task *aid.AiTask
		task.GenerateIndex() // Should not panic
		assert.Nil(t, task, "Task should still be nil")
	})

	// Test case 2: Single task (root)
	t.Run("SingleRootTask", func(t *testing.T) {
		task := &aid.AiTask{Name: "Root"}
		task.GenerateIndex()
		assert.Equal(t, "1", task.Index)
	})

	// Test case 3: Task with subtasks
	t.Run("TaskWithSubtasks", func(t *testing.T) {
		root := &aid.AiTask{
			Name: "Root",
			Subtasks: []*aid.AiTask{
				{Name: "Sub1"},
				{Name: "Sub2"},
			},
		}
		// Set parent pointers for subtasks
		for _, sub := range root.Subtasks {
			sub.ParentTask = root
		}
		root.GenerateIndex()
		assert.Equal(t, "1", root.Index, "Root index should be 1")
		assert.Equal(t, "1-1", root.Subtasks[0].Index, "Sub1 index should be 1-1")
		assert.Equal(t, "1-2", root.Subtasks[1].Index, "Sub2 index should be 1-2")
	})

	// Test case 4: Calling GenerateIndex on a subtask (should rebuild from root)
	t.Run("GenerateIndexFromSubtask", func(t *testing.T) {
		root := &aid.AiTask{Name: "Root"}
		sub1 := &aid.AiTask{Name: "Sub1", ParentTask: root}
		sub2 := &aid.AiTask{Name: "Sub2", ParentTask: root}
		root.Subtasks = []*aid.AiTask{sub1, sub2}

		sub1.GenerateIndex() // Call on subtask

		assert.Equal(t, "1", root.Index, "Root index should be 1")
		assert.Equal(t, "1-1", sub1.Index, "Sub1 index should be 1-1")
		assert.Equal(t, "1-2", sub2.Index, "Sub2 index should be 1-2")
	})

	// Test case 5: Nested subtasks
	t.Run("NestedSubtasks", func(t *testing.T) {
		root := &aid.AiTask{Name: "Root"}
		s1 := &aid.AiTask{Name: "S1", ParentTask: root}
		s1_1 := &aid.AiTask{Name: "S1.1", ParentTask: s1}
		s1_2 := &aid.AiTask{Name: "S1.2", ParentTask: s1}
		s2 := &aid.AiTask{Name: "S2", ParentTask: root}

		s1.Subtasks = []*aid.AiTask{s1_1, s1_2}
		root.Subtasks = []*aid.AiTask{s1, s2}

		root.GenerateIndex()

		assert.Equal(t, "1", root.Index)
		assert.Equal(t, "1-1", s1.Index)
		assert.Equal(t, "1-1-1", s1_1.Index)
		assert.Equal(t, "1-1-2", s1_2.Index)
		assert.Equal(t, "1-2", s2.Index)
	})

	// Test case 6: Calling GenerateIndex on a deeply nested subtask
	t.Run("GenerateIndexFromNestedSubtask", func(t *testing.T) {
		root := &aid.AiTask{Name: "Root"}
		s1 := &aid.AiTask{Name: "S1", ParentTask: root}
		s1_1 := &aid.AiTask{Name: "S1.1", ParentTask: s1}
		s1_1_1 := &aid.AiTask{Name: "S1.1.1", ParentTask: s1_1}
		s1_2 := &aid.AiTask{Name: "S1.2", ParentTask: s1}
		s2 := &aid.AiTask{Name: "S2", ParentTask: root}

		s1_1.Subtasks = []*aid.AiTask{s1_1_1}
		s1.Subtasks = []*aid.AiTask{s1_1, s1_2}
		root.Subtasks = []*aid.AiTask{s1, s2}

		s1_1_1.GenerateIndex() // Call on the most nested subtask

		assert.Equal(t, "1", root.Index, "Root index")
		assert.Equal(t, "1-1", s1.Index, "S1 index")
		assert.Equal(t, "1-1-1", s1_1.Index, "S1.1 index")
		assert.Equal(t, "1-1-1-1", s1_1_1.Index, "S1.1.1 index")
		assert.Equal(t, "1-1-2", s1_2.Index, "S1.2 index")
		assert.Equal(t, "1-2", s2.Index, "S2 index")
	})

	// Test Case 7: Task with parent but no siblings, calling GenerateIndex on child
	t.Run("ChildWithParentNoSiblings", func(t *testing.T) {
		parent := &aid.AiTask{Name: "Parent"}
		child := &aid.AiTask{Name: "Child", ParentTask: parent}
		parent.Subtasks = []*aid.AiTask{child}

		child.GenerateIndex()

		assert.Equal(t, "1", parent.Index, "Parent index")
		assert.Equal(t, "1-1", child.Index, "Child index")
	})

	// Test Case 8: Complex structure with GenerateIndex called on an intermediate node
	t.Run("ComplexStructureIntermediateCall", func(t *testing.T) {
		root := &aid.AiTask{Name: "R"}
		sA := &aid.AiTask{Name: "SA", ParentTask: root}
		sA1 := &aid.AiTask{Name: "SA1", ParentTask: sA}
		sA2 := &aid.AiTask{Name: "SA2", ParentTask: sA}
		sB := &aid.AiTask{Name: "SB", ParentTask: root}
		sB1 := &aid.AiTask{Name: "SB1", ParentTask: sB}

		sA.Subtasks = []*aid.AiTask{sA1, sA2}
		sB.Subtasks = []*aid.AiTask{sB1}
		root.Subtasks = []*aid.AiTask{sA, sB}

		sA2.GenerateIndex() // Call GenerateIndex on sA2

		assert.Equal(t, "1", root.Index, "R")
		assert.Equal(t, "1-1", sA.Index, "SA")
		assert.Equal(t, "1-1-1", sA1.Index, "SA1")
		assert.Equal(t, "1-1-2", sA2.Index, "SA2")
		assert.Equal(t, "1-2", sB.Index, "SB")
		assert.Equal(t, "1-2-1", sB1.Index, "SB1")
	})
}

func TestTaskCancel(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)
	ctx, cancel := context.WithCancel(context.Background())
	coordinator, err := aid.NewCoordinatorContext(
		ctx,
		"test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithSystemFileOperator(),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()
			defer func() {
				cancel()
				time.Sleep(100 * time.Millisecond)
				rsp.Close()
			}()
			fmt.Println("===========" + "request:" + "===========\n" + request.GetPrompt())
			rsp.EmitOutputStream(strings.NewReader(`
{
    "@action": "plan",
    "query": "找出 /Users/v1ll4n/Projects/yaklang 目录中最大的文件",
    "main_task": "在给定路径下寻找体积最大的文件",
    "main_task_goal": "识别 /Users/v1ll4n/Projects/yaklang 目录中占用存储空间最多的文件，并展示其完整路径与大小信息",
    "tasks": [
        {
            "subtask_name": "扫描目录结构",
            "subtask_goal": "递归遍历 /Users/v1ll4n/Projects/yaklang 目录下所有文件，记录每个文件的位置和占用空间"
        },
        {
            "subtask_name": "计算文件大小",
            "subtask_goal": "遍历所有文件，计算每个文件的大小"
        }
    ]
}
			`))

			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}
	go func() {
		count := 0
	LOOP:
		for {
			select {
			case <-time.After(30 * time.Second):
				break LOOP
			case result := <-outputChan:
				count++
				if count > 100 {
					break LOOP
				}

				if result.Type == schema.EVENT_TYPE_CONSUMPTION {
					continue
				}

				fmt.Println("result:" + result.String())
				if result.IsInteractive() {
					inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
					continue
				}
			}
		}
	}()
	_ = coordinator.Run()
}
