package aidmock

import "github.com/yaklang/yaklang/common/ai/aid/aicommon"

// AIScenario 是用于测试的AI场景接口
// 实现此接口的类型可以生成模拟的AI回调函数，用于测试AI相关功能
type AIScenario interface {
	// GetAICallbackType 返回一个AI回调函数，该函数会根据请求内容返回模拟的AI响应
	GetAICallbackType() aicommon.AICallbackType
}
