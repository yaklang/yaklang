// mirror.go - aibalance 流量镜像 (Mirror) 子系统
//
// 概念:
//   - 每条镜像规则对应一段用户编写的 yak 脚本 (必须定义 func handle(data) { ... })
//   - 每次客户端 /v1/chat/completions 请求完整结束后, aibalance 构造一个完整的
//     MirrorSnapshot 并把它异步投递给所有命中条件的规则; 规则各自的 worker pool
//     拉取并调用脚本.
//
// 设计原则:
//   - 绝不阻塞主请求链路: Trigger 是 fire-and-forget, 投递队列使用非阻塞 send.
//   - 每条规则隔离: 独立 channel + 独立 worker pool, 互不干扰.
//   - 队列满时丢弃 (drop newest 语义), 同步累加 dropped 计数.
//   - 单次脚本执行超时 = TimeoutMs (ms), 超时强制 cancel, 不影响其它脚本.
//   - 脚本 panic recover, 计入 failed.
//
// 关键词: aibalance MirrorManager, MirrorRuleRuntime, mirror snapshot,
//        yak callback, hook traffic, @action 解析, OpenAI tool_calls 镜像

package aibalance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

// APIKeyFingerprint 计算原始 API Key 的不可逆指纹 (SHA256[:16] hex).
//
// 用途: 镜像快照里需要"区分不同 key" (做统计 / 关联), 但绝不能把原始或
// shrink 形态的 key 暴露给用户脚本——shrink 后的 head/tail 几个字符
// 仍可能反推, 是泄漏面.
//
// 64 bit entropy 足够避免单实例下的碰撞 (生日攻击 ~2^32 个 key 才碰撞);
// 长度 16 同时保证日志可读. 空字符串返回空串, 让上层用 "free-user" 等
// 字面量替代.
//
// 关键词: APIKeyFingerprint, mirror snapshot 不可逆脱敏, SHA256 fp,
//
//	consume key but not leak
func APIKeyFingerprint(raw string) string {
	if raw == "" {
		return ""
	}
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])[:16]
}

// ==================== Condition Types ====================

const (
	// MirrorConditionActionEq 仅根据响应主输出中的 @action 字段过滤.
	MirrorConditionActionEq = "action_eq"

	// MirrorConditionAnyToolcall 任意 OpenAI 原生 tool_calls 出现即触发.
	MirrorConditionAnyToolcall = "any_toolcall"

	// MirrorConditionActionCallToolEq 同时要求 @action ∈ {call-tool,
	// directly_call_tool, require_tool} 且 payload 中工具名 = ToolName.
	MirrorConditionActionCallToolEq = "action_call_tool_eq"

	// MirrorConditionAlways 永真, 每次成功请求都触发.
	MirrorConditionAlways = "always"
)

// ValidMirrorConditionTypes 给 handler 做白名单校验用.
var ValidMirrorConditionTypes = map[string]bool{
	MirrorConditionActionEq:         true,
	MirrorConditionAnyToolcall:      true,
	MirrorConditionActionCallToolEq: true,
	MirrorConditionAlways:           true,
}

// ==================== MirrorSnapshot ====================

// MirrorSnapshot 是一次成功 chat 请求的完整快照, 透传给所有命中规则的回调脚本.
// 字段在 mirror.go 与 portal UI / 文档之间保持稳定; 新增字段允许, 删/改要小心.
//
// 关键词: MirrorSnapshot 数据结构, hook chat 请求快照
type MirrorSnapshot struct {
	ReqID       string `json:"req_id"`
	TimestampMs int64  `json:"timestamp_ms"`

	Model    string `json:"model"`     // 客户端请求的对外 model 名
	TypeName string `json:"type_name"` // 实际命中的 provider type (openai / deepseek / ...)
	Domain   string `json:"domain"`    // provider domain

	// APIKeyFP 是 API Key 的不可逆指纹 (SHA256[:16] hex). 用于"区分 key"
	// 做统计 / 调试关联, 但绝不可反推原 key. 历史字段曾叫 api_key 并用
	// shrink 形式, 那种 head/tail 可见仍是泄漏面, 已彻底废弃.
	//
	// 关键词: MirrorSnapshot api_key_fp, key fingerprint redaction
	APIKeyFP    string `json:"api_key_fp"`
	IsFreeModel bool   `json:"is_free_model"`
	Stream      bool   `json:"stream"`

	RequestMessages []aispec.ChatDetail `json:"request_messages"`

	ResponseText   string             `json:"response_text"`
	ResponseReason string             `json:"response_reason"`
	ToolCalls      []*aispec.ToolCall `json:"tool_calls"`

	// Action / ActionPayload 由 ParseAction 解析得到; 解析失败 Action="", Payload=nil.
	Action        string                 `json:"action"`
	ActionPayload map[string]interface{} `json:"action_payload"`

	DurationMs  int64             `json:"duration_ms"`
	InputBytes  int64             `json:"input_bytes"`
	OutputBytes int64             `json:"output_bytes"`
	Usage       *aispec.ChatUsage `json:"usage"`
}

// ToScriptMap 把 snapshot 序列化为对脚本友好的 map[string]any 形态,
// 嵌套结构通过 JSON 往返一次以保证类型纯净 (避免暴露 Go 私有指针).
//
// 安全: 兜底删除任何形如 api_key / apikey 的字段, 不允许脚本层意外接触
// 到原始 key. 即使后来有人误改字段名, 也保住下游脚本不会拿到完整 key.
//
// 关键词: MirrorSnapshot ToScriptMap, JSON 往返脱壳, yak 脚本可见层,
//
//	api_key redact defensive
func (s *MirrorSnapshot) ToScriptMap() map[string]any {
	if s == nil {
		return map[string]any{}
	}
	raw, err := json.Marshal(s)
	if err != nil {
		log.Warnf("mirror: serialize snapshot failed: %v", err)
		return map[string]any{
			"req_id": s.ReqID,
			"model":  s.Model,
		}
	}
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	// 防御性兜底: 任何可能携带原 key 的字段都剔除.
	delete(out, "api_key")
	delete(out, "apikey")
	delete(out, "ApiKey")
	delete(out, "APIKey")
	return out
}

// ==================== Action 解析 ====================
//
// 设计参考: common/ai/aid/aicommon/action.go::ExtractAllAction
//   - 用 jsonextractor.ExtractObjectIndexes 做字节级状态机扫描, 不再手写 brace
//     counter. 这样能正确处理 markdown 代码块包裹 / 字符串内的花括号 / 多个
//     连续 JSON 对象 / 不完整结尾被 LLM 截断等场景.
//   - 用 jsonextractor.JsonValidObject 给"半破" JSON 做一次修复 (尾逗号 / 单引号
//     等) 再 Unmarshal, 跟 aicommon 同一套兼容策略.
//   - 唯一与 aicommon 不同的是: aicommon 是"生产者"语义 (强制要求 @action 字段
//     存在), mirror 是"被动观察者"语义 (尽量识别, 失败时返回空串和 nil 而不是
//     报错, 让上层规则决定要不要继续触发).
//
// 关键词: aibalance mirror 解析, 对齐 aicommon ExtractAllAction,
//        jsonextractor.ExtractObjectIndexes, JsonValidObject 半破修复

// ParseActionFromText 尝试从响应主输出中解析 yaklang JSON 协议中的 @action 字段.
//
// 行为:
//   - 扫出 text 中所有合法 / 可修复的 JSON 对象, 取第一个含非空 @action 的对象.
//   - @action 字段也支持嵌套对象 (例如 {"@action": {"name": "directly_answer"}}),
//     此时取该对象中第一个非空字符串值, 与 aicommon 行为对齐.
//   - 兜底 fallback: 当所有对象都没有 @action 但存在 next_action.type (aireact
//     早期协议形态), 退化取那个值. 此 fallback 是 mirror 专属松绑, aicommon 没有.
//
// 返回 (action, payload):
//   - 成功: action 是非空字符串, payload 是完整的 JSON 对象 map (包含 @action 字段本身).
//   - 失败: action="", payload=nil. 调用方在 always 规则下仍应当继续投递快照.
//
// 关键词: ParseActionFromText, @action JSON 协议解析, aireact next_action 兼容,
//
//	aicommon ExtractAllAction 对齐, mirror 被动观察者宽松解析
func ParseActionFromText(text string) (string, map[string]interface{}) {
	if text == "" {
		return "", nil
	}
	var fallbackPayload map[string]interface{}
	var fallbackAction string

	for _, pair := range jsonextractor.ExtractObjectIndexes(text) {
		start, end := pair[0], pair[1]
		if start < 0 || end > len(text) || start >= end {
			continue
		}
		raw := text[start:end]
		payload, ok := unmarshalJSONObjectLoose([]byte(raw))
		if !ok || payload == nil {
			continue
		}
		// 1. 主路径: 顶层 @action 字段.
		if name := extractActionNameFromPayload(payload); name != "" {
			return name, payload
		}
		// 2. fallback 路径: next_action.type. 仅作为"找不到任何 @action 时的兜底",
		//    所以先记下来, 不立刻返回, 让后续对象有机会胜出.
		if fallbackAction == "" {
			if next, ok := payload["next_action"].(map[string]interface{}); ok {
				if t, ok := next["type"].(string); ok && t != "" {
					fallbackAction = t
					fallbackPayload = payload
				}
			}
		}
	}
	if fallbackAction != "" {
		return fallbackAction, fallbackPayload
	}
	return "", nil
}

// unmarshalJSONObjectLoose 尝试把一段 JSON 子串解析成 map.
//   - 优先直接 json.Unmarshal.
//   - 失败时调 jsonextractor.JsonValidObject 做一次"半破修复" (尾逗号 / 单引号 /
//     缺引号等) 后再 Unmarshal. 这与 aicommon 走的修复路径一致.
//
// 关键词: unmarshalJSONObjectLoose, JsonValidObject 半破修复, 鲁棒 JSON 解析
func unmarshalJSONObjectLoose(raw []byte) (map[string]interface{}, bool) {
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err == nil {
		return payload, true
	}
	fixed, ok := jsonextractor.JsonValidObject(raw)
	if !ok {
		return nil, false
	}
	if err := json.Unmarshal(fixed, &payload); err != nil {
		return nil, false
	}
	return payload, true
}

// extractActionNameFromPayload 从已解析的 map 中按 aicommon 语义抽 @action.
//
// 兼容形态:
//   - {"@action": "directly_answer"}  (常规)
//   - {"@action": {"name": "directly_answer"}} 等嵌套对象 (取第一个非空字符串)
//
// 关键词: extractActionNameFromPayload, @action 字段抽取, aicommon 嵌套对象兼容
func extractActionNameFromPayload(payload map[string]interface{}) string {
	v, ok := payload["@action"]
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case map[string]interface{}:
		for _, vv := range t {
			if s, ok := vv.(string); ok {
				if s = strings.TrimSpace(s); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// ExtractToolFromActionPayload 根据 action 取出工具名, 用于条件 3 匹配:
//   - call-tool / directly_call_tool: 顶层 "tool" 字段
//   - require_tool:                  "tool_require_payload" 字段
//
// 关键词: ExtractToolFromActionPayload, call-tool tool 字段, require_tool payload
func ExtractToolFromActionPayload(action string, payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	switch action {
	case "call-tool", "directly_call_tool":
		if v, ok := payload["tool"].(string); ok {
			return v
		}
		// 嵌套 next_action.tool
		if next, ok := payload["next_action"].(map[string]interface{}); ok {
			if v, ok := next["tool"].(string); ok {
				return v
			}
		}
	case "require_tool":
		if v, ok := payload["tool_require_payload"].(string); ok {
			return v
		}
		if next, ok := payload["next_action"].(map[string]interface{}); ok {
			if v, ok := next["tool_require_payload"].(string); ok {
				return v
			}
		}
	}
	return ""
}

// ==================== Evaluator ====================

// MirrorRuleMatch 判定一条规则是否命中给定快照.
//
// 关键词: MirrorRuleMatch, mirror condition evaluator, 4 \u4e2d\u4e00 \u5206\u652f
func MirrorRuleMatch(rule *schema.AiMirrorRule, snap *MirrorSnapshot) bool {
	if rule == nil || snap == nil {
		return false
	}
	switch rule.ConditionType {
	case MirrorConditionAlways:
		return true
	case MirrorConditionAnyToolcall:
		return len(snap.ToolCalls) > 0
	case MirrorConditionActionEq:
		return rule.ActionName != "" && snap.Action == rule.ActionName
	case MirrorConditionActionCallToolEq:
		// action 必须是 call-tool / directly_call_tool / require_tool 之一.
		// 这是该 condition 的"硬性收敛", 没有这一层就退化成纯 tool 名过滤,
		// 容易跟客户端真正的 OpenAI tool_calls (那个走 any_toolcall) 串味.
		switch snap.Action {
		case "call-tool", "directly_call_tool", "require_tool":
		default:
			return false
		}
		// ActionName 可选过滤器:
		//   - 留空: 上述 3 种 action 任意通配 (大多数场景用这个)
		//   - 非空: 进一步收敛到指定 action (例如只关心 require_tool, 排除 call-tool)
		// 关键词: action_call_tool_eq ActionName 可选过滤, optional action narrow
		if rule.ActionName != "" && rule.ActionName != snap.Action {
			return false
		}
		if rule.ToolName == "" {
			return false
		}
		tool := ExtractToolFromActionPayload(snap.Action, snap.ActionPayload)
		return tool == rule.ToolName
	default:
		return false
	}
}

// ==================== Recent Logs (in-memory ring) ====================

// MirrorRunLog 单条回调脚本运行记录.
//
// 关键词: MirrorRunLog, 内存环形日志条目
type MirrorRunLog struct {
	Timestamp    time.Time `json:"timestamp"`
	ReqID        string    `json:"req_id"`
	DurationMs   int64     `json:"duration_ms"`
	Success      bool      `json:"success"`
	ErrorMessage string    `json:"error_message"`
	Stdout       string    `json:"stdout"`

	// save() 调用反馈: 让运行日志也能看出这次有没有落盘、落了多少。
	// 关键词: MirrorRunLog save_calls/save_persisted/save_bytes, save 可观测
	SaveCalls     int   `json:"save_calls"`
	SavePersisted int   `json:"save_persisted"`
	SaveBytes     int64 `json:"save_bytes"`
}

const mirrorLogRingCap = 100

type mirrorLogRing struct {
	mu     sync.Mutex
	buf    []MirrorRunLog
	pos    int
	filled bool
}

func newMirrorLogRing() *mirrorLogRing {
	return &mirrorLogRing{buf: make([]MirrorRunLog, mirrorLogRingCap)}
}

func (r *mirrorLogRing) push(entry MirrorRunLog) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[r.pos] = entry
	r.pos = (r.pos + 1) % mirrorLogRingCap
	if r.pos == 0 {
		r.filled = true
	}
}

func (r *mirrorLogRing) snapshot() []MirrorRunLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []MirrorRunLog
	if r.filled {
		out = make([]MirrorRunLog, 0, mirrorLogRingCap)
		out = append(out, r.buf[r.pos:]...)
		out = append(out, r.buf[:r.pos]...)
	} else {
		out = make([]MirrorRunLog, 0, r.pos)
		out = append(out, r.buf[:r.pos]...)
	}
	// reverse, newest first
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// ==================== Runtime per rule ====================

// mirrorRuleRuntime 单条规则的运行时态.
//
// 关键词: mirrorRuleRuntime, worker pool, buffered channel queue
type mirrorRuleRuntime struct {
	rule    *schema.AiMirrorRule
	ch      chan *MirrorSnapshot
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	logs    *mirrorLogRing
	scriptN int32 // 实时在跑的 worker 数 (atomic)
}

// ==================== MirrorManager ====================

// MirrorManager 全局镜像调度器, 单例挂在 ServerConfig 上.
//
// 关键词: MirrorManager, mirror rules runtime registry
type MirrorManager struct {
	mu      sync.RWMutex
	runtime map[uint]*mirrorRuleRuntime

	// engineConcurrency 控制 yak.NewScriptEngine 初始化时的 maxConcurrent;
	// 这里只是 pool 的上限, 实际并发由每条规则各自的 worker 数决定.
	engineConcurrency int
}

// NewMirrorManager 仅构造空 manager, 实际加载 DB 规则要显式调用 LoadRules.
func NewMirrorManager() *MirrorManager {
	return &MirrorManager{
		runtime:           make(map[uint]*mirrorRuleRuntime),
		engineConcurrency: 32,
	}
}

// LoadRules 从 DB 加载全部启用规则, 启动各自 worker pool.
// 等价于 Reload, 但语义上仅用于 boot 阶段.
func (m *MirrorManager) LoadRules() error {
	rules, err := ListEnabledMirrorRules()
	if err != nil {
		return fmt.Errorf("list enabled mirror rules: %w", err)
	}
	for _, r := range rules {
		m.startRule(r)
	}
	log.Infof("mirror: loaded %d enabled rules", len(rules))
	return nil
}

// ReloadRule 把单条规则从 DB 重新拉一次并热替换运行时.
// 调用方场景:
//   - 创建规则后 (rule 是新的)
//   - 更新规则后 (字段可能影响 worker 数 / 队列大小)
//   - 切换 enabled
func (m *MirrorManager) ReloadRule(id uint) error {
	rule, err := GetMirrorRuleByID(id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// 先停掉旧的
	if old, ok := m.runtime[id]; ok {
		old.cancel()
		// 等 worker 退出 (drain ch, 不阻塞太久)
		go func(rt *mirrorRuleRuntime) {
			rt.wg.Wait()
		}(old)
		delete(m.runtime, id)
	}
	// 如果是删除或被禁用, 不再启动新的
	if rule == nil || !rule.Enabled {
		return nil
	}
	m.startRuleLocked(rule)
	return nil
}

// RemoveRule 与 ReloadRule 类似但仅做停止, 调用方在 DELETE handler 中使用.
func (m *MirrorManager) RemoveRule(id uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.runtime[id]; ok {
		old.cancel()
		go func(rt *mirrorRuleRuntime) { rt.wg.Wait() }(old)
		delete(m.runtime, id)
	}
}

// startRule 是 startRuleLocked 的加锁包装, 仅供 LoadRules 使用.
func (m *MirrorManager) startRule(rule *schema.AiMirrorRule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startRuleLocked(rule)
}

func (m *MirrorManager) startRuleLocked(rule *schema.AiMirrorRule) {
	if rule == nil || !rule.Enabled {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	queueSize := rule.QueueSize
	if queueSize <= 0 {
		queueSize = 1024
	}
	concurrency := rule.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	rt := &mirrorRuleRuntime{
		rule:   rule,
		ch:     make(chan *MirrorSnapshot, queueSize),
		cancel: cancel,
		logs:   newMirrorLogRing(),
	}
	m.runtime[rule.ID] = rt
	for i := 0; i < concurrency; i++ {
		rt.wg.Add(1)
		go m.workerLoop(ctx, rt)
	}
	log.Infof("mirror: started rule id=%d name=%q concurrency=%d queue=%d",
		rule.ID, rule.Name, concurrency, queueSize)
}

// workerLoop 是单个 worker 的主循环, 从 ch 读快照并调用 runOnce.
//
// 关键词: workerLoop, mirror worker 主循环, ctx 退出
func (m *MirrorManager) workerLoop(ctx context.Context, rt *mirrorRuleRuntime) {
	defer rt.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case snap, ok := <-rt.ch:
			if !ok {
				return
			}
			atomic.AddInt32(&rt.scriptN, 1)
			m.runOnce(ctx, rt, snap)
			atomic.AddInt32(&rt.scriptN, -1)
		}
	}
}

// runOnce 执行一次回调脚本; 单次超时由 rt.rule.TimeoutMs 控制.
//
// 关键词: runOnce, yak script callback, 超时 + panic recover
func (m *MirrorManager) runOnce(parentCtx context.Context, rt *mirrorRuleRuntime, snap *MirrorSnapshot) {
	rule := rt.rule
	timeout := time.Duration(rule.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	start := time.Now()
	var execErr error
	var stdout string
	var saveStat saveOutcome

	defer func() {
		if r := recover(); r != nil {
			execErr = fmt.Errorf("script panic: %v", r)
		}
		dur := time.Since(start)
		runLog := MirrorRunLog{
			Timestamp:     start,
			ReqID:         snap.ReqID,
			DurationMs:    dur.Milliseconds(),
			Success:       execErr == nil,
			Stdout:        stdout,
			SaveCalls:     saveStat.Calls,
			SavePersisted: saveStat.Persisted,
			SaveBytes:     saveStat.Bytes,
		}
		if execErr != nil {
			runLog.ErrorMessage = execErr.Error()
			log.Warnf("mirror: rule id=%d name=%q req_id=%s failed: %v",
				rule.ID, rule.Name, snap.ReqID, execErr)
		} else if saveStat.Calls > 0 {
			log.Infof("mirror: rule id=%d name=%q req_id=%s save calls=%d persisted=%d bytes=%d",
				rule.ID, rule.Name, snap.ReqID, saveStat.Calls, saveStat.Persisted, saveStat.Bytes)
		}
		rt.logs.push(runLog)
		// 写库: 触发 +1, 成功 / 失败 +1 (二者只有一个会真正写)
		delta := MirrorCounterDelta{Triggered: 1, TouchTime: true}
		if execErr == nil {
			delta.Success = 1
		} else {
			delta.Failed = 1
		}
		if dbErr := IncrementMirrorCounters(rule.ID, delta); dbErr != nil {
			log.Warnf("mirror: increment counters failed for rule id=%d: %v", rule.ID, dbErr)
		}
	}()

	execErr, stdout, saveStat = executeMirrorScript(ctx, rule.CallbackScript, snap, true)
}

// executeMirrorScript 真正调用 yak ScriptEngine.
// 用户脚本必须包含 func handle(data) { ... }; 这里在脚本末尾追加一段
// 自动调用代码 handle(getParam("MIRROR_DATA")), 其中 MIRROR_DATA 是
// 通过 yak ScriptEngine 的 params 传入 (用户脚本里也可以通过
// getParam("MIRROR_DATA") 自行获取).
//
// saveOutcome 汇总单次脚本执行里 save() 的调用情况, 供试运行面板与运行日志展示,
// 让用户清楚 save 调没调、写了多少、生产是否会真落盘。
// 关键词: saveOutcome, save 可观测性, 试运行/日志反馈
type saveOutcome struct {
	Calls     int    `json:"calls"`     // save() 被调用次数
	Persisted int    `json:"persisted"` // 实际写盘成功次数 (试运行恒为 0)
	Bytes     int64  `json:"bytes"`     // 序列化后总字节 (即将/已写入)
	Enabled   bool   `json:"enabled"`   // 落盘子系统当前是否启用 (生产是否会真落)
	Preview   string `json:"preview"`   // 首次 save 的内容预览 (截断)
}

// savePreviewLimit 是 save 预览的截断长度。
const savePreviewLimit = 800

// allowPersist 控制注入的 save() 是否真正落盘: 生产触发为 true; 面板「试运行」为 false,
// 避免试运行污染落盘数据。无论是否落盘, 都会记录 save() 的调用情况 (saveOutcome) 以反馈给用户。
// save() 注入为可直接调用的全局函数 (SetVars), 语义:
//   - save()      落盘当前镜像数据 (即 handle 的入参 data)
//   - save(x)     落盘任意可序列化对象 x
//
// 返回值: (执行错误, stdout, save 调用汇总)。
//
// 关键词: executeMirrorScript, yak engine ExecuteExWithContext, handle 调用, save 注入落盘, saveOutcome 反馈
func executeMirrorScript(ctx context.Context, script string, snap *MirrorSnapshot, allowPersist bool) (error, string, saveOutcome) {
	outcome := saveOutcome{Enabled: dataSinkEnabled()}
	if strings.TrimSpace(script) == "" {
		return fmt.Errorf("empty script"), "", outcome
	}
	dataMap := snap.ToScriptMap()
	const tail = "\n\n// auto-injected by aibalance mirror runner\n" +
		"__mirror_data = getParam(\"MIRROR_DATA\")\n" +
		"handle(__mirror_data)\n"
	wrapped := script + tail

	// save() 闭包: 默认落盘当前快照, 也可传入自定义内容。无论是否真正落盘, 都记录调用情况。
	// 试运行 (allowPersist=false) 时不写盘, 返回值按「生产是否启用」回放, 便于脚本逻辑分支贴近真实。
	var saveMu sync.Mutex
	saveFn := func(args ...any) bool {
		var payload any = dataMap
		if len(args) > 0 && args[0] != nil {
			payload = args[0]
		}
		raw, mErr := json.Marshal(payload)

		saveMu.Lock()
		outcome.Calls++
		if mErr == nil {
			outcome.Bytes += int64(len(raw))
			if outcome.Preview == "" {
				preview := string(raw)
				if len(preview) > savePreviewLimit {
					preview = preview[:savePreviewLimit] + "...(truncated)"
				}
				outcome.Preview = preview
			}
		}
		saveMu.Unlock()

		if !allowPersist {
			// 试运行: 不落盘, 返回「生产环境是否会落盘」, 让脚本里的 if save() 分支贴近真实。
			return outcome.Enabled
		}
		ok, err := dataSinkAppend(payload)
		if err != nil {
			log.Warnf("mirror: save record failed (req_id=%s): %v", snap.ReqID, err)
			return false
		}
		if ok {
			saveMu.Lock()
			outcome.Persisted++
			saveMu.Unlock()
		}
		return ok
	}

	engine := yak.NewScriptEngine(1)
	// 把 save 注入为脚本可直接调用的全局函数。
	engine.RegisterEngineHooks(func(ng *antlr4yak.Engine) error {
		ng.SetVars(map[string]any{"save": saveFn})
		return nil
	})
	params := map[string]any{
		"MIRROR_DATA": dataMap,
		"runtime_id":  "mirror-" + snap.ReqID,
	}
	_, err := engine.ExecuteExWithContext(ctx, wrapped, params)
	if err != nil {
		return err, "", outcome
	}
	return nil, "", outcome
}

// ==================== Trigger / Dispatch ====================

// Trigger 把一个快照分发给所有命中规则; 调用方应当 go c.MirrorManager.Trigger(snap).
//
// 投递语义: 非阻塞 send; channel 满时丢弃并累加 dropped 计数 + 落日志环形缓冲.
//
// 关键词: MirrorManager.Trigger, 非阻塞投递, drop newest 语义
func (m *MirrorManager) Trigger(snap *MirrorSnapshot) {
	if m == nil || snap == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("mirror: trigger panic recovered: %v", r)
		}
	}()
	m.mu.RLock()
	runtimes := make([]*mirrorRuleRuntime, 0, len(m.runtime))
	for _, rt := range m.runtime {
		runtimes = append(runtimes, rt)
	}
	m.mu.RUnlock()
	for _, rt := range runtimes {
		if !MirrorRuleMatch(rt.rule, snap) {
			continue
		}
		select {
		case rt.ch <- snap:
		default:
			// 队列满, 丢弃并记账
			rt.logs.push(MirrorRunLog{
				Timestamp:    time.Now(),
				ReqID:        snap.ReqID,
				Success:      false,
				ErrorMessage: "queue full, dropped",
			})
			if err := IncrementMirrorCounters(rt.rule.ID, MirrorCounterDelta{
				Triggered: 1,
				Dropped:   1,
				TouchTime: true,
			}); err != nil {
				log.Warnf("mirror: rule id=%d drop counter failed: %v", rt.rule.ID, err)
			}
			log.Warnf("mirror: rule id=%d name=%q dropped snapshot req_id=%s (queue full)",
				rt.rule.ID, rt.rule.Name, snap.ReqID)
		}
	}
}

// ==================== Capability Hints (for server.go short-circuit) ====================
//
// server.go 在每次 chat completion 完成后会调用 Trigger 来分发快照. 但构造一个
// 完整 MirrorSnapshot 不便宜 (要 deep copy ToolCalls / Usage, 解析 @action,
// 拼字符串等). 如果当前一条镜像规则都没启用, 这些工作就是白做.
//
// 这里给 server 层提供两个**廉价**的提示:
//   - HasActiveRules:      整个 manager 是否有任何在跑的规则. 没有 -> 整段跳过.
//   - NeedsActionParsing:  在跑的规则里是否至少有一条需要 @action (action_eq /
//                          action_call_tool_eq). 没有 -> 不调 ParseActionFromText.
//
// 两个方法都仅持读锁扫一遍 runtime map, O(N), 没有 IO. 关键词:
//   MirrorManager.HasActiveRules, MirrorManager.NeedsActionParsing,
//   mirror trigger 节能短路, 无规则跳过 snapshot 构造

// HasActiveRules 返回当前是否有任何在跑的规则.
//
// 关键词: HasActiveRules, mirror 节能短路
func (m *MirrorManager) HasActiveRules() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.runtime) > 0
}

// NeedsActionParsing 仅当至少一条在跑规则的 ConditionType 需要 @action 字段
// (action_eq / action_call_tool_eq) 时才返回 true. 其它 condition (always /
// any_toolcall) 不依赖 @action, 调用方可以跳过 ParseActionFromText.
//
// 关键词: NeedsActionParsing, mirror 是否需要解析 @action, ParseAction 节能
func (m *MirrorManager) NeedsActionParsing() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, rt := range m.runtime {
		if rt == nil || rt.rule == nil {
			continue
		}
		switch rt.rule.ConditionType {
		case MirrorConditionActionEq, MirrorConditionActionCallToolEq:
			return true
		}
	}
	return false
}

// ==================== Public Read APIs (for handle_mirror.go) ====================

// MirrorRuleStatus 给 portal 用的运行时态聚合视图.
type MirrorRuleStatus struct {
	Rule          *schema.AiMirrorRule `json:"rule"`
	QueueLength   int                  `json:"queue_length"`
	QueueCapacity int                  `json:"queue_capacity"`
	ActiveWorkers int32                `json:"active_workers"`
}

// GetStatus 返回单条规则的运行时态, 不命中返回 nil.
func (m *MirrorManager) GetStatus(id uint) *MirrorRuleStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rt, ok := m.runtime[id]
	if !ok {
		return nil
	}
	return &MirrorRuleStatus{
		Rule:          rt.rule,
		QueueLength:   len(rt.ch),
		QueueCapacity: cap(rt.ch),
		ActiveWorkers: atomic.LoadInt32(&rt.scriptN),
	}
}

// GetRecentLogs 返回单条规则的最近 N 条调用日志 (newest first).
func (m *MirrorManager) GetRecentLogs(id uint) []MirrorRunLog {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rt, ok := m.runtime[id]
	if !ok {
		return nil
	}
	return rt.logs.snapshot()
}

// MirrorTestResult 是试运行的结构化结果, 除成功/耗时外, 额外带 save() 调用反馈,
// 让用户在面板上直观看到 save 调没调、会写多少、生产是否会真落盘。
// 关键词: MirrorTestResult, 试运行 save 反馈
type MirrorTestResult struct {
	Executed     bool
	ErrorMessage string
	DurationMs   int64
	Save         saveOutcome
}

// RunOnceForTest 直接同步执行一次回调脚本 (不入 ch, 不计 DB 计数, 不真正落盘).
// 供 /portal/api/mirror-rules/{id}/test 试运行用; 返回结果含 save() 调用汇总。
func (m *MirrorManager) RunOnceForTest(script string, snap *MirrorSnapshot, timeoutMs int64) (result MirrorTestResult) {
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()
	start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			result.Executed = false
			result.ErrorMessage = fmt.Sprintf("script panic: %v", r)
			result.DurationMs = time.Since(start).Milliseconds()
		}
	}()
	if snap == nil {
		snap = &MirrorSnapshot{ReqID: utils.RandStringBytes(8), TimestampMs: time.Now().UnixMilli()}
	}
	err, _, saveStat := executeMirrorScript(ctx, script, snap, false)
	result.DurationMs = time.Since(start).Milliseconds()
	result.Save = saveStat
	if err != nil {
		result.Executed = false
		result.ErrorMessage = err.Error()
		return result
	}
	result.Executed = true
	return result
}

// Close 关停所有规则的 worker, 用于服务退出.
func (m *MirrorManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, rt := range m.runtime {
		rt.cancel()
		go func(rt *mirrorRuleRuntime) { rt.wg.Wait() }(rt)
		delete(m.runtime, id)
	}
}
