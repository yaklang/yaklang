package aistats

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// init 把 aistats 的统计实现注册进 aicommon 注册缝.
// 默认开启: 被 blank-import 后即生效.
//
// 关键词: aistats init, RegisterStatsRecorder, 命中统计默认开启
func init() {
	aicommon.RegisterStatsRecorder(recorder{})
}
