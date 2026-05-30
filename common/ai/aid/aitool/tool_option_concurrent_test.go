package aitool

import (
	"sync"
	"testing"
)

// TestMUSTPASS_ToolOption_ConcurrentReuse 复现并锁死 "concurrent map writes" 致命崩溃.
//
// 背景: reactloops.buildSchema 会把 action.Options ([]ToolOption) 缓存下来, 每次生成
// prompt 都重新应用一遍 (NewObjectSchema -> newTool -> option(tool)). 当 agent 存在
// 并发 loop (主循环 + reflection/verification) 时, 同一批 ToolOption 闭包会被多个
// goroutine 并发应用. 历史实现里 WithRawParam / WithStructParam 直接修改闭包在构造期
// 捕获的同一个 schema map, 于是并发应用就会触发 fatal error: concurrent map writes
// (见 tool.go schema["description"]=desc 这类写入).
//
// 本用例构造一批"会被复用"的 ToolOption, 用大量 goroutine 并发应用, 配合 -race /
// 反复执行来稳定暴露该数据竞争. 修复后 (闭包不再修改共享 map) 必须稳定通过.
//
// 关键词: ToolOption 复用并发安全, concurrent map writes 回归, schema map 共享修复
func TestMUSTPASS_ToolOption_ConcurrentReuse(t *testing.T) {
	// 这批 options 模拟 action.Options: 构造一次, 之后被反复/并发应用.
	sharedOptions := []any{
		WithStringParam(
			"target",
			WithParam_Description("scan target host"),
			WithParam_Required(true),
			WithParam_EnumString("a", "b", "c"),
		),
		WithIntegerParam(
			"port",
			WithParam_Description("port number"),
			WithParam_Min(1),
			WithParam_Max(65535),
		),
		WithStringArrayParam(
			"tags",
			WithParam_Description("array of tags"),
		),
		WithStructParam(
			"options",
			[]PropertyOption{WithParam_Description("nested options object")},
			WithStringParam("mode", WithParam_Description("scan mode"), WithParam_Required(true)),
			WithBoolParam("verbose", WithParam_Description("verbose flag")),
		),
		WithStructArrayParam(
			"rules",
			[]PropertyOption{WithParam_Description("array of rule objects")},
			[]PropertyOption{WithParam_Description("single rule")},
			WithStringParam("name", WithParam_Description("rule name"), WithParam_Required(true)),
			WithNumberParam("weight", WithParam_Description("rule weight")),
		),
	}

	const goroutines = 64
	const iterations = 30

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// 复用同一批 options, 模拟 buildSchema 的反复应用.
				schema := NewObjectSchema(sharedOptions...)
				if schema == "" {
					panic("schema should not be empty")
				}
			}
		}()
	}
	wg.Wait()
}
