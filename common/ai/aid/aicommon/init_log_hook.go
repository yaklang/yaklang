package aicommon

import (
	"github.com/yaklang/yaklang/common/log"
)

// 关键词: init log hook, DebugStreamPrinter log 包装注入
//
// 把 common/log 的 SetOutput 注册给 debug_stream_printer.go 里的
// setLogOutput 变量。这样 EnsureLogFlushWrapperInstalled 可以在不引入
// 循环依赖的前提下接管 log 默认输出。
func init() {
	setLogOutput = log.SetOutput
}
