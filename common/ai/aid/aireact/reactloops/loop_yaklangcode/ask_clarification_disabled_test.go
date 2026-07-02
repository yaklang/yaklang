package loop_yaklangcode

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

// allowInteractConfig 强制 GetAllowUserInteraction()=true, 模拟真实 YakRunner AI 场景
// (全局允许用户交互)。其余方法透传底层 mock config。
type allowInteractConfig struct {
	aicommon.AICallerConfigIf
}

func (allowInteractConfig) GetAllowUserInteraction() bool { return true }

// allowInteractInvoker 用 allowInteractConfig 覆盖 GetConfig, 其余透传底层 MockInvoker。
type allowInteractInvoker struct {
	*mock.MockInvoker
	cfg aicommon.AICallerConfigIf
}

func (i *allowInteractInvoker) GetConfig() aicommon.AICallerConfigIf { return i.cfg }

// TestWriteYaklangLoop_DisablesAskForClarification 回归保护:
// write_yaklang_code 是"直接写代码"的 focus 模式, 即使全局允许用户交互, 也绝不能暴露
// ask_for_clarification 动作 —— 否则 AI 会在遇到"可多种实现"的开放业务需求(如"标记敏感数据")
// 时反复反问用户, 而不是基于合理默认直接产出代码。
func TestWriteYaklangLoop_DisablesAskForClarification(t *testing.T) {
	base := mock.NewMockInvoker(context.Background())
	inv := &allowInteractInvoker{
		MockInvoker: base,
		cfg:         allowInteractConfig{AICallerConfigIf: base.GetConfig()},
	}
	require.True(t, inv.GetConfig().GetAllowUserInteraction(), "precondition: 全局允许用户交互")

	factory, ok := reactloops.GetLoopFactory(schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG)
	require.True(t, ok, "write_yaklang_code loop factory should be registered")

	loop, err := factory(inv)
	require.NoError(t, err)

	_, err = loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_ASK_FOR_CLARIFICATION)
	require.Error(t, err, "write_yaklang_code 即便全局允许交互也不得提供 ask_for_clarification")

	// sanity: 写代码动作必须存在, 证明 loop 本身构建正常。
	_, err = loop.GetActionHandler("write_code")
	require.NoError(t, err, "write_code action should be available")
}
