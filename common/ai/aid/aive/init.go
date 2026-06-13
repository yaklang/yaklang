package aive

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// init 把 aive 的价值评估实现注册进 aicommon 注册缝.
// 默认开启: 被 blank-import 后即生效, 暂不提供关闭开关.
//
// 关键词: aive init, RegisterValueFeedbackSubmitter, 价值评估默认开启
func init() {
	aicommon.RegisterValueFeedbackSubmitter(submitValueFeedback)
}
