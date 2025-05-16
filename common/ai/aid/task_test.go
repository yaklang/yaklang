package aid

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAiTask_GenerateIndex(t *testing.T) {
	// Test case 1: Nil task
	t.Run("NilTask", func(t *testing.T) {
		var task *aiTask
		task.GenerateIndex() // Should not panic
		assert.Nil(t, task, "Task should still be nil")
	})

	// Test case 2: Single task (root)
	t.Run("SingleRootTask", func(t *testing.T) {
		task := &aiTask{Name: "Root"}
		task.GenerateIndex()
		assert.Equal(t, "1-0", task.Index)
	})

	// Test case 3: Task with subtasks
	t.Run("TaskWithSubtasks", func(t *testing.T) {
		root := &aiTask{
			Name: "Root",
			Subtasks: []*aiTask{
				{Name: "Sub1"},
				{Name: "Sub2"},
			},
		}
		// Set parent pointers for subtasks
		for _, sub := range root.Subtasks {
			sub.ParentTask = root
		}
		root.GenerateIndex()
		assert.Equal(t, "1-0", root.Index, "Root index should be 1-0")
		assert.Equal(t, "2-1", root.Subtasks[0].Index, "Sub1 index should be 2-1")
		assert.Equal(t, "3-1", root.Subtasks[1].Index, "Sub2 index should be 3-1")
	})

	// Test case 4: Calling GenerateIndex on a subtask (should rebuild from root)
	t.Run("GenerateIndexFromSubtask", func(t *testing.T) {
		root := &aiTask{Name: "Root"}
		sub1 := &aiTask{Name: "Sub1", ParentTask: root}
		sub2 := &aiTask{Name: "Sub2", ParentTask: root}
		root.Subtasks = []*aiTask{sub1, sub2}

		sub1.GenerateIndex() // Call on subtask

		assert.Equal(t, "1-0", root.Index, "Root index should be 1-0")
		assert.Equal(t, "2-1", sub1.Index, "Sub1 index should be 2-1")
		assert.Equal(t, "3-1", sub2.Index, "Sub2 index should be 3-1")
	})

	// Test case 5: Nested subtasks
	t.Run("NestedSubtasks", func(t *testing.T) {
		root := &aiTask{Name: "Root"}
		s1 := &aiTask{Name: "S1", ParentTask: root}
		s1_1 := &aiTask{Name: "S1.1", ParentTask: s1}
		s1_2 := &aiTask{Name: "S1.2", ParentTask: s1}
		s2 := &aiTask{Name: "S2", ParentTask: root}

		s1.Subtasks = []*aiTask{s1_1, s1_2}
		root.Subtasks = []*aiTask{s1, s2}

		root.GenerateIndex()

		assert.Equal(t, "1-0", root.Index)
		assert.Equal(t, "2-1", s1.Index)
		assert.Equal(t, "3-2", s1_1.Index)
		assert.Equal(t, "4-2", s1_2.Index)
		assert.Equal(t, "5-1", s2.Index)
	})

	// Test case 6: Calling GenerateIndex on a deeply nested subtask
	t.Run("GenerateIndexFromNestedSubtask", func(t *testing.T) {
		root := &aiTask{Name: "Root"}
		s1 := &aiTask{Name: "S1", ParentTask: root}
		s1_1 := &aiTask{Name: "S1.1", ParentTask: s1}
		s1_1_1 := &aiTask{Name: "S1.1.1", ParentTask: s1_1}
		s1_2 := &aiTask{Name: "S1.2", ParentTask: s1}
		s2 := &aiTask{Name: "S2", ParentTask: root}

		s1_1.Subtasks = []*aiTask{s1_1_1}
		s1.Subtasks = []*aiTask{s1_1, s1_2}
		root.Subtasks = []*aiTask{s1, s2}

		s1_1_1.GenerateIndex() // Call on the most nested subtask

		assert.Equal(t, "1-0", root.Index, "Root index")
		assert.Equal(t, "2-1", s1.Index, "S1 index")
		assert.Equal(t, "3-2", s1_1.Index, "S1.1 index")
		assert.Equal(t, "4-3", s1_1_1.Index, "S1.1.1 index")
		assert.Equal(t, "5-2", s1_2.Index, "S1.2 index")
		assert.Equal(t, "6-1", s2.Index, "S2 index")
	})

	// Test Case 7: Task with parent but no siblings, calling GenerateIndex on child
	t.Run("ChildWithParentNoSiblings", func(t *testing.T) {
		parent := &aiTask{Name: "Parent"}
		child := &aiTask{Name: "Child", ParentTask: parent}
		parent.Subtasks = []*aiTask{child}

		child.GenerateIndex()

		assert.Equal(t, "1-0", parent.Index, "Parent index")
		assert.Equal(t, "2-1", child.Index, "Child index")
	})

	// Test Case 8: Complex structure with GenerateIndex called on an intermediate node
	t.Run("ComplexStructureIntermediateCall", func(t *testing.T) {
		root := &aiTask{Name: "R"}
		sA := &aiTask{Name: "SA", ParentTask: root}
		sA1 := &aiTask{Name: "SA1", ParentTask: sA}
		sA2 := &aiTask{Name: "SA2", ParentTask: sA}
		sB := &aiTask{Name: "SB", ParentTask: root}
		sB1 := &aiTask{Name: "SB1", ParentTask: sB}

		sA.Subtasks = []*aiTask{sA1, sA2}
		sB.Subtasks = []*aiTask{sB1}
		root.Subtasks = []*aiTask{sA, sB}

		sA2.GenerateIndex() // Call GenerateIndex on sA2

		assert.Equal(t, "1-0", root.Index, "R")
		assert.Equal(t, "2-1", sA.Index, "SA")
		assert.Equal(t, "3-2", sA1.Index, "SA1")
		assert.Equal(t, "4-2", sA2.Index, "SA2")
		assert.Equal(t, "5-1", sB.Index, "SB")
		assert.Equal(t, "6-5", sB1.Index, "SB1")
	})
}
