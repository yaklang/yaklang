package reactloops

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

func newActionHistoryTestLoop() *ReActLoop {
	invoker := mock.NewMockInvoker(context.Background())
	return NewMinimalReActLoop(invoker.GetConfig(), invoker)
}

// TestGetLastValidAction 覆盖 GetLastValidAction 的边界行为：
// 它必须从历史尾部向前跳过 finish 记录（和 nil 记录），返回最近一次有效 action。
// loop_default 的 post-iteration 总结钩子依赖该语义判断收尾 action。
func TestGetLastValidAction(t *testing.T) {
	finishType := loopAction_Finish.ActionType

	cases := []struct {
		name          string
		history       []*ActionRecord
		wantNil       bool
		wantType      string
		wantIteration int
	}{
		{
			name:    "empty history returns nil",
			history: nil,
			wantNil: true,
		},
		{
			name: "only finish records returns nil",
			history: []*ActionRecord{
				{ActionType: finishType, IterationIndex: 1},
				{ActionType: finishType, IterationIndex: 2},
			},
			wantNil: true,
		},
		{
			name: "trailing finish is skipped",
			history: []*ActionRecord{
				{ActionType: "tool_call", IterationIndex: 1},
				{ActionType: loopAction_DirectlyAnswer.ActionType, IterationIndex: 2},
				{ActionType: finishType, IterationIndex: 3},
			},
			wantType:      loopAction_DirectlyAnswer.ActionType,
			wantIteration: 2,
		},
		{
			name: "multiple trailing finishes are skipped",
			history: []*ActionRecord{
				{ActionType: "tool_call", IterationIndex: 1},
				{ActionType: finishType, IterationIndex: 2},
				{ActionType: finishType, IterationIndex: 3},
			},
			wantType:      "tool_call",
			wantIteration: 1,
		},
		{
			name: "last non-finish record is returned directly",
			history: []*ActionRecord{
				{ActionType: finishType, IterationIndex: 1},
				{ActionType: "tool_call", IterationIndex: 2},
			},
			wantType:      "tool_call",
			wantIteration: 2,
		},
		{
			name: "nil records are skipped",
			history: []*ActionRecord{
				{ActionType: "tool_call", IterationIndex: 1},
				nil,
				{ActionType: finishType, IterationIndex: 2},
				nil,
			},
			wantType:      "tool_call",
			wantIteration: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loop := newActionHistoryTestLoop()
			loop.actionHistory = append(loop.actionHistory, tc.history...)

			got := loop.GetLastValidAction()
			if tc.wantNil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, tc.wantType, got.ActionType)
			require.Equal(t, tc.wantIteration, got.IterationIndex)
		})
	}
}

// TestGetLastValidAction_VersusGetLastAction 锁定两个 API 的语义差异：
// 当循环以 finish 收尾时，GetLastAction 返回 finish 记录本身，
// 而 GetLastValidAction 必须返回 finish 之前的有效 action（如 directly_answer）。
// 这是 loop_default 最终总结钩子改用 GetLastValidAction 的直接原因。
func TestGetLastValidAction_VersusGetLastAction(t *testing.T) {
	loop := newActionHistoryTestLoop()
	loop.actionHistory = append(loop.actionHistory,
		&ActionRecord{ActionType: loopAction_DirectlyAnswer.ActionType, IterationIndex: 1},
		&ActionRecord{ActionType: loopAction_Finish.ActionType, IterationIndex: 2},
	)

	last := loop.GetLastAction()
	require.NotNil(t, last)
	require.Equal(t, loopAction_Finish.ActionType, last.ActionType)

	valid := loop.GetLastValidAction()
	require.NotNil(t, valid)
	require.Equal(t, loopAction_DirectlyAnswer.ActionType, valid.ActionType)
}
