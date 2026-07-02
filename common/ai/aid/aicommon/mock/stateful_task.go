package mock

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// MockStatefulTask provides a reusable AIStatefulTask test double that can be
// embedded or composed by tests which only need to override a few behaviors.
type MockStatefulTask struct {
	*aicommon.AIStatefulTaskBase

	index           string
	originUserInput string
}

func NewMockStatefulTask(ctx context.Context, id string, userInput string) *MockStatefulTask {
	base := aicommon.NewStatefulTaskBase(id, userInput, ctx, nil, true)
	return &MockStatefulTask{
		AIStatefulTaskBase: base,
		index:              id,
		originUserInput:    userInput,
	}
}

func (m *MockStatefulTask) GetIndex() string {
	if m == nil {
		return ""
	}
	if m.index != "" {
		return m.index
	}
	if m.AIStatefulTaskBase == nil {
		return ""
	}
	return m.AIStatefulTaskBase.GetIndex()
}

func (m *MockStatefulTask) SetIndex(index string) {
	if m == nil {
		return
	}
	m.index = index
}

func (m *MockStatefulTask) GetOriginUserInput() string {
	if m == nil {
		return ""
	}
	if m.originUserInput != "" {
		return m.originUserInput
	}
	if m.AIStatefulTaskBase == nil {
		return ""
	}
	return m.AIStatefulTaskBase.GetOriginUserInput()
}

func (m *MockStatefulTask) SetOriginUserInput(input string) {
	if m == nil {
		return
	}
	m.originUserInput = input
}

var _ aicommon.AIStatefulTask = (*MockStatefulTask)(nil)
