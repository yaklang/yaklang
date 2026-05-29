// tool_call_aggregator.go - aibalance 工具调用聚合日志器
//
// 用途: OpenAI Chat Completions 流式 tool_calls 协议下, 上游每帧 SSE 只携带
//
//	delta.tool_calls[*].function.arguments 的几字节增量, 客户端按 index 累积
//	后才能拼出完整 tool_call. 这种 incremental 形态对客户端是合规的, 但对
//	aibalance 运维而言, 在生产日志里只能看到"index=0 args='curl'"/"index=0
//	args=' -'"/... 几十帧零碎片段, 几乎无法肉眼判断 "这一轮 model 究竟调了
//	哪个工具、参数是什么".
//
//	ToolCallAggregator 在不破坏下行 SSE 透传的前提下镜像一份增量流到一个
//	状态机, 按 OpenAI tool_calls 协议规则聚合, 在以下三个时机往系统日志输出
//	一行 "完整 tool_call" 记录:
//
//	  1. 某个 index 首次出现 (且 name 已知) → "tool_call_agg: started ..."
//	  2. 看到不同 index 出现 → flush 之前那个 index → "completed ..."
//	  3. Flush() 调用 (通常 writer.Close 触发) → 把所有未 flush 的 entry 一次性 flush
//
// 设计原则:
//   - 不影响下行 SSE 字节: Observe 仅做聚合, 不写 writerClose
//   - 默认启用 (高价值低开销), env AIBALANCE_TOOL_CALL_AGG=off/0/false 可关闭
//   - nil-safe receiver: Observe/Flush 对 nil aggregator 是 no-op
//   - 并发安全: mu 串行化 Observe/Flush
//   - 日志全英文, 严肃, 一行一记录, 便于 grep
//
// 关键词: ToolCallAggregator, incremental tool_calls 聚合日志, 镜像状态机,
//
//	OpenAI 流式协议 index 累积, aibalance 运维可观测性
package aibalance

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
)

// ToolCallAggregatorEnabled 全局开关. 默认开启 (返回 true).
// env AIBALANCE_TOOL_CALL_AGG=off / 0 / false / no / disable 任一关闭.
//
// 关键词: ToolCallAggregator env 开关, 默认 on, 可显式关闭
func ToolCallAggregatorEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("AIBALANCE_TOOL_CALL_AGG")))
	switch v {
	case "off", "0", "false", "no", "disable", "disabled":
		return false
	default:
		return true
	}
}

// ToolCallAggregator 把 OpenAI Chat Completions 流式 delta.tool_calls
// incremental 增量聚合成"完整 tool_call"日志.
//
// 生命周期: 通常每个 chat completion 请求 (chatJSONChunkWriter) 持有一个 aggregator.
// 上游每帧 incremental delta 来时 Observe; stream 关闭时 Flush.
//
// 并发: mu 串行化 Observe / Flush. 调用方无需额外加锁.
//
// 关键词: ToolCallAggregator 主结构, 一个请求一个聚合器
type ToolCallAggregator struct {
	mu      sync.Mutex
	tag     string                    // 来源标识, 推荐用 req_id / writer uid
	entries map[int]*toolCallAggEntry // 按 index 维护
	order   []int                     // 首次出现顺序, 便于 Flush 按顺序输出
	lastIdx int                       // 上一次 Observe 看到的 index
	seenAny bool                      // 是否至少 Observe 过一次
	enabled bool                      // env 决定的启用状态, 构造时 snapshot
}

// toolCallAggEntry 单个 index 的聚合状态.
type toolCallAggEntry struct {
	Index     int
	ID        string
	TypeName  string
	Name      string
	Args      strings.Builder
	StartedAt time.Time
	Started   bool // 是否已打过 "started" 日志
	Flushed   bool // 是否已打过 "completed" 日志
}

// NewToolCallAggregator 构造聚合器. env 关闭时返回 nil, 调用方所有方法对 nil 安全.
// tag 用于在日志里关联具体请求 (推荐 writer uid / DebugTraceSession ReqID).
//
// 关键词: NewToolCallAggregator, env-gated 默认 on, nil-safe
func NewToolCallAggregator(tag string) *ToolCallAggregator {
	if !ToolCallAggregatorEnabled() {
		return nil
	}
	return &ToolCallAggregator{
		tag:     tag,
		entries: make(map[int]*toolCallAggEntry),
		enabled: true,
	}
}

// Observe 接收上游一帧 tool_calls incremental delta.
// 不破坏下行 SSE 字节, 仅聚合状态.
//
// 触发逻辑:
//   - 新 index 首次出现: 创建 entry, 若 hasFirst 已为 true 且新旧 index 不同,
//     flush 之前的 lastIdx entry
//   - name 首次到位 (从无到有): 打 "started" 日志
//   - arguments 增量: 拼接到 entry.Args
//
// 关键词: ToolCallAggregator.Observe, 喂帧聚合, 新 index flush 旧
func (a *ToolCallAggregator) Observe(toolCalls []*aispec.ToolCall) {
	if a == nil || len(toolCalls) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, tc := range toolCalls {
		if tc == nil {
			continue
		}
		entry, ok := a.entries[tc.Index]
		if !ok {
			// 新 index 出现: 若之前已有一个 index, 把它 flush 掉.
			// 这给客户端 / 运维一个"上一个 tool_call 已结束"的及时信号,
			// 不必等整个 stream 关闭.
			// 关键词: 新 index 触发上一个 flush, 实时聚合
			if a.seenAny && a.lastIdx != tc.Index {
				a.flushEntryLocked(a.entries[a.lastIdx])
			}
			entry = &toolCallAggEntry{
				Index:     tc.Index,
				StartedAt: time.Now(),
			}
			a.entries[tc.Index] = entry
			a.order = append(a.order, tc.Index)
		}
		if tc.ID != "" && entry.ID == "" {
			entry.ID = tc.ID
		}
		if tc.Type != "" && entry.TypeName == "" {
			entry.TypeName = tc.Type
		}
		if tc.Function.Name != "" && entry.Name == "" {
			entry.Name = tc.Function.Name
			// name 首次到位才认为"started"信息完整 (没 name 客户端也分发不了),
			// 此时才打 started 日志, 避免不完整记录.
			// 关键词: started 日志触发条件, name 必须到位
			if !entry.Started {
				entry.Started = true
				log.Infof("tool_call_agg: started tag=%s index=%d name=%s id=%s type=%s",
					a.tag, entry.Index, entry.Name, entry.ID, entry.TypeName)
			}
		}
		if tc.Function.Arguments != "" {
			entry.Args.WriteString(tc.Function.Arguments)
		}
		a.lastIdx = tc.Index
		a.seenAny = true
	}
}

// Flush 把所有还未 flush 的 entries 输出到日志.
// 通常在 writer.Close / stream 结束 / finish_reason="tool_calls" 帧到达时调用.
// 幂等: 已 flush 的 entry 不会被重复输出.
//
// 关键词: ToolCallAggregator.Flush, 兜底输出所有累积, 幂等
func (a *ToolCallAggregator) Flush() {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, idx := range a.order {
		a.flushEntryLocked(a.entries[idx])
	}
}

// Snapshot 返回当前已聚合的 tool_call 完整快照, 便于测试断言 / 外部审计.
// 返回的切片按 index 首次出现顺序排列.
//
// 关键词: ToolCallAggregator.Snapshot, 测试断言用快照
func (a *ToolCallAggregator) Snapshot() []AggregatedToolCall {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]AggregatedToolCall, 0, len(a.order))
	for _, idx := range a.order {
		e := a.entries[idx]
		if e == nil {
			continue
		}
		out = append(out, AggregatedToolCall{
			Index:    e.Index,
			ID:       e.ID,
			TypeName: e.TypeName,
			Name:     e.Name,
			Args:     e.Args.String(),
			Flushed:  e.Flushed,
		})
	}
	return out
}

// AggregatedToolCall 一个 index 的完整聚合视图, 供 Snapshot / 测试使用.
type AggregatedToolCall struct {
	Index    int
	ID       string
	TypeName string
	Name     string
	Args     string
	Flushed  bool
}

// flushEntryLocked 必须持锁调用. nil 或已 flush 直接 skip.
// 打"completed"日志, 把完整聚合内容输出.
//
// 关键词: flushEntryLocked, completed 日志输出, 完整 tool_call 还原
func (a *ToolCallAggregator) flushEntryLocked(entry *toolCallAggEntry) {
	if entry == nil || entry.Flushed {
		return
	}
	entry.Flushed = true
	args := entry.Args.String()
	name := entry.Name
	if name == "" {
		name = "(unknown)"
	}
	typeName := entry.TypeName
	if typeName == "" {
		typeName = "function"
	}
	log.Infof("tool_call_agg: completed tag=%s index=%d name=%s id=%s type=%s args_len=%d duration_ms=%d args=%s",
		a.tag, entry.Index, name, entry.ID, typeName, len(args),
		time.Since(entry.StartedAt).Milliseconds(), args)
}
