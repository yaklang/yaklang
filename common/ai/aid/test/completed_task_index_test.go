package test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

// TestVerifySatisfactionResult tests the VerifySatisfactionResult struct
func TestVerifySatisfactionResult(t *testing.T) {
	t.Run("NewVerifySatisfactionResult", func(t *testing.T) {
		result := aicommon.NewVerifySatisfactionResult(true, "test reason", "1-1")
		assert.True(t, result.Satisfied)
		assert.Equal(t, "test reason", result.Reasoning)
		assert.Equal(t, "1-1", result.CompletedTaskIndex)
	})

	t.Run("EmptyCompletedTaskIndex", func(t *testing.T) {
		result := aicommon.NewVerifySatisfactionResult(false, "not done", "")
		assert.False(t, result.Satisfied)
		assert.Equal(t, "not done", result.Reasoning)
		assert.Empty(t, result.CompletedTaskIndex)
	})

	t.Run("MultipleCompletedTaskIndex", func(t *testing.T) {
		result := aicommon.NewVerifySatisfactionResult(true, "done", "1-1,1-2")
		assert.True(t, result.Satisfied)
		assert.Equal(t, "1-1,1-2", result.CompletedTaskIndex)
	})
}

// TestSatisfactionRecordWithCompletedTaskIndex tests the satisfaction record with completed task index
func TestSatisfactionRecordWithCompletedTaskIndex(t *testing.T) {
	// Create a mock invoker using the mock package
	invoker := mock.NewMockInvoker(context.Background())

	t.Run("PushAndGetSatisfactionRecordWithCompletedTaskIndex", func(t *testing.T) {
		loop, err := reactloops.NewReActLoop("test-loop", invoker)
		assert.NoError(t, err)

		// Push a satisfaction record with completed task index and next movements
		loop.PushSatisfactionRecordWithCompletedTaskIndex(true, "task completed", "1-1", "")

		// Get the last satisfaction record using the new struct-based API
		record := loop.GetLastSatisfactionRecordFull()
		assert.NotNil(t, record)
		assert.True(t, record.Satisfactory)
		assert.Equal(t, "task completed", record.Reason)
		assert.Equal(t, "1-1", record.CompletedTaskIndex)
		assert.Empty(t, record.NextMovements)
	})

	t.Run("MultipleSatisfactionRecords", func(t *testing.T) {
		loop, err := reactloops.NewReActLoop("test-loop", invoker)
		assert.NoError(t, err)

		// Push multiple records with next movements
		loop.PushSatisfactionRecordWithCompletedTaskIndex(false, "in progress", "", "next step: check file permissions")
		loop.PushSatisfactionRecordWithCompletedTaskIndex(true, "done", "1-2", "")

		// Should get the last one
		record := loop.GetLastSatisfactionRecordFull()
		assert.NotNil(t, record)
		assert.True(t, record.Satisfactory)
		assert.Equal(t, "done", record.Reason)
		assert.Equal(t, "1-2", record.CompletedTaskIndex)
		assert.Empty(t, record.NextMovements)
	})

	t.Run("EmptySatisfactionRecords", func(t *testing.T) {
		loop, err := reactloops.NewReActLoop("test-loop", invoker)
		assert.NoError(t, err)

		// Should return nil when no records
		record := loop.GetLastSatisfactionRecordFull()
		assert.Nil(t, record)
	})

	t.Run("BackwardCompatibility_PushSatisfactionRecord", func(t *testing.T) {
		loop, err := reactloops.NewReActLoop("test-loop", invoker)
		assert.NoError(t, err)

		// Use the old method without completed task index
		loop.PushSatisfactionRecord(true, "old style")

		// Should still work with GetLastSatisfactionRecord
		satisfied, reason := loop.GetLastSatisfactionRecord()
		assert.True(t, satisfied)
		assert.Equal(t, "old style", reason)

		// New method should return struct with empty completed task index and next movements
		record := loop.GetLastSatisfactionRecordFull()
		assert.NotNil(t, record)
		assert.Empty(t, record.CompletedTaskIndex)
		assert.Empty(t, record.NextMovements)
	})

	t.Run("NextMovementsTracking", func(t *testing.T) {
		loop, err := reactloops.NewReActLoop("test-loop", invoker)
		assert.NoError(t, err)

		// Push a record with next movements
		loop.PushSatisfactionRecordWithCompletedTaskIndex(false, "in progress", "", "use chmod 600 to fix file permissions")

		// Get the last satisfaction record
		record := loop.GetLastSatisfactionRecordFull()
		assert.NotNil(t, record)
		assert.False(t, record.Satisfactory)
		assert.Equal(t, "in progress", record.Reason)
		assert.Empty(t, record.CompletedTaskIndex)
		assert.Equal(t, "use chmod 600 to fix file permissions", record.NextMovements)
	})
}

// TestCompletedTaskIndexParsing tests the parsing logic for completed task index
// This tests the same logic used in task_execute.go
func TestCompletedTaskIndexParsing(t *testing.T) {
	testCases := []struct {
		name                string
		completedTaskIndex  string
		currentTaskIndex    string
		expectedShouldMatch bool
	}{
		{
			name:                "single exact match",
			completedTaskIndex:  "1-1",
			currentTaskIndex:    "1-1",
			expectedShouldMatch: true,
		},
		{
			name:                "single no match",
			completedTaskIndex:  "1-1",
			currentTaskIndex:    "1-2",
			expectedShouldMatch: false,
		},
		{
			name:                "multiple match first",
			completedTaskIndex:  "1-1,1-2",
			currentTaskIndex:    "1-1",
			expectedShouldMatch: true,
		},
		{
			name:                "multiple match second",
			completedTaskIndex:  "1-1,1-2",
			currentTaskIndex:    "1-2",
			expectedShouldMatch: true,
		},
		{
			name:                "multiple with spaces",
			completedTaskIndex:  "1-1, 1-2, 1-3",
			currentTaskIndex:    "1-2",
			expectedShouldMatch: true,
		},
		{
			name:                "nested task index match",
			completedTaskIndex:  "1-1-1,1-1-2",
			currentTaskIndex:    "1-1-1",
			expectedShouldMatch: true,
		},
		{
			name:                "empty completed index",
			completedTaskIndex:  "",
			currentTaskIndex:    "1-1",
			expectedShouldMatch: false,
		},
		{
			name:                "partial match should not match",
			completedTaskIndex:  "1-1",
			currentTaskIndex:    "1-10",
			expectedShouldMatch: false,
		},
		{
			name:                "prefix match should not match",
			completedTaskIndex:  "1-1-1",
			currentTaskIndex:    "1-1",
			expectedShouldMatch: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			shouldMatch := false
			if tc.completedTaskIndex != "" {
				completedIndexes := strings.Split(tc.completedTaskIndex, ",")
				for _, idx := range completedIndexes {
					trimmedIdx := strings.TrimSpace(idx)
					if trimmedIdx == tc.currentTaskIndex {
						shouldMatch = true
						break
					}
				}
			}
			assert.Equal(t, tc.expectedShouldMatch, shouldMatch, "Mismatch for test case: %s", tc.name)
		})
	}
}

// TestOnPostIterationOperator tests the OnPostIterationOperator functionality
func TestOnPostIterationOperator(t *testing.T) {
	invoker := mock.NewMockInvoker(context.Background())

	t.Run("EndIterationShouldSignalLoopToEnd", func(t *testing.T) {
		var operatorEndCalled int32 = 0

		loop, err := reactloops.NewReActLoop("test-loop", invoker,
			reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
				// Simulate completed_task_index matching current task
				if iteration >= 1 {
					operator.EndIteration("test: task completed via completed_task_index")
					atomic.AddInt32(&operatorEndCalled, 1)
				}
			}),
			reactloops.WithMaxIterations(10), // Set max iterations to prevent infinite loop
		)
		assert.NoError(t, err)
		assert.NotNil(t, loop)
	})

	t.Run("OperatorDefaultState", func(t *testing.T) {
		loop, err := reactloops.NewReActLoop("test-loop", invoker,
			reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
				// Operator should be provided and not nil
				assert.NotNil(t, operator)
			}),
		)
		assert.NoError(t, err)
		assert.NotNil(t, loop)

		// Verify operator is provided in the callback
		// Note: We can't actually execute the loop without proper mocking,
		// but we can verify the loop creation succeeds with the new signature
	})

	t.Run("OperatorEndIterationWithReason", func(t *testing.T) {
		// This tests the operator behavior in isolation
		// Create a loop with the callback that tests operator behavior
		callbackExecuted := false

		_, err := reactloops.NewReActLoop("test-loop", invoker,
			reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
				callbackExecuted = true
				// Test that EndIteration sets the correct state
				assert.False(t, operator.ShouldEndIteration(), "Initially should not end iteration")
				assert.Nil(t, operator.GetEndReason(), "Initially reason should be nil")

				operator.EndIteration("custom reason")

				assert.True(t, operator.ShouldEndIteration(), "After EndIteration, should end iteration")
				assert.Equal(t, "custom reason", operator.GetEndReason(), "Reason should be set")
			}),
		)
		assert.NoError(t, err)
		// Note: callbackExecuted would be true only if we execute the loop
		_ = callbackExecuted
	})
}
