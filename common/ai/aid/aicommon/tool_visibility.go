package aicommon

import (
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// Tool Visibility (hidden tool pattern)
//
// 本模块给 Tool Inventory 渲染层提供一份"集中名单 + 最小行为差异"的可见性
// 模型. 它的存在原因是: 默认 Tool Inventory 段在 prompt 里展示所有 enable
// tools, 但其中部分工具:
//
//   1) 已有更好的替代品 (例: send_http_request_by_url / send_http_request_packet
//      / url_content_summary 都被 do_http_request 覆盖), 默认不再推荐 ->
//      标记 VisibilityHidden, 默认不展示, 也不允许 focus 模式拉回 inventory.
//
//   2) 仅在特定场景下值得推荐 (例: ssa / SyntaxFlow 系列工具只在代码审计
//      场景才有意义), 不该默认占据通用 inventory 名额 -> 标记 VisibilityScenario,
//      默认不展示, 但 focus 模式可以通过 ScenarioToolWhitelist 把指定 scenario
//      工具拉回 inventory 并置顶.
//
// 不论是 hidden 还是 scenario, 工具本身在 AiToolManager.GetEnableTools() /
// GetToolByName / search_capabilities (RAG / BM25 DB) / ResolveIdentifier 等
// 链路里仍然可用; 仅默认 inventory 渲染入口会按本模块的判定做一次过滤.
//
// 关键词: hidden tool pattern, tool visibility, default inventory filter,
//        scenario whitelist, amap hidden, ssa scenario, search_capabilities
//        discoverable

// Visibility 描述一个 AI 工具在默认 Tool Inventory 中的可见策略.
//
// 关键词: Visibility enum, VisibilityNormal, VisibilityHidden, VisibilityScenario
type Visibility int

const (
	// VisibilityNormal 是默认可见. 不在 hidden / scenario 名单 / 前缀中
	// 的工具都属于此类, 会正常出现在 Tool Inventory 中.
	VisibilityNormal Visibility = iota

	// VisibilityHidden 是隐藏工具 (不推荐使用 / 已有替代品). 默认 inventory
	// 一律不展示, focus 模式也无法拉回 inventory; 仅在 AI 显式通过名字
	// require_tool / load_capability / search_capabilities 命中时才会被用.
	VisibilityHidden

	// VisibilityScenario 是特定场景工具. 默认 inventory 不展示, 但 focus
	// 模式可以通过 __SCENARIO_TOOLS__ dunder 或代码侧 WithScenarioToolWhitelist
	// 把指定工具拉回 inventory 头部置顶.
	VisibilityScenario
)

// String 给 log / debug 用, 不参与业务逻辑.
func (v Visibility) String() string {
	switch v {
	case VisibilityNormal:
		return "normal"
	case VisibilityHidden:
		return "hidden"
	case VisibilityScenario:
		return "scenario"
	default:
		return "unknown"
	}
}

// 名单维护采用"集中 Go 代码"方案 (centralized): 不修改 .yak 脚本, 不动
// schema.AIYakTool, 不动 aitool.Tool 类型签名. 名单分两层:
//
//   - 精确名字匹配: 完全等于工具 Name 时命中.
//   - 名字前缀匹配: 用于给将来同系列新增工具兜底 (例: 新增 ssa-foo 自动归为
//     scenario), 避免每加一个工具都得回来手动登记.
//
// hidden 名单按 hidden 用途记录, scenario 名单按 scenario 用途记录. 两者
// 互斥 (一个名字不应该同时出现在两边); 若意外冲突, LookupToolVisibility 的
// 实现按 "hidden 优先" 处理, 因为 hidden 语义更强 (永远不进 inventory).
//
// 关键词: hiddenToolNames, scenarioToolNames, scenarioToolPrefixes,
//        centralized registry, prefix fallback

var (
	visibilityMu sync.RWMutex

	// hiddenToolNames 是默认隐藏工具的精确名字集合.
	//
	// 命中规则:
	//   - amap/ 目录下全部 .yak 工具 (8 个), 业务无关, 默认不推荐.
	//   - HTTP 系列里有更好替代品的: send_http_request_by_url /
	//     send_http_request_packet 被 do_http_request 完全覆盖;
	//     url_content_summary 是单 URL 摘要, 也被 do_http_request + 简易爬虫
	//     等覆盖.
	//
	// 关键词: hidden tool names, amap, http deprecated, do_http_request replacement
	hiddenToolNames = map[string]struct{}{
		// HTTP 已有替代品的旧工具
		"url_content_summary":      {},
		"send_http_request_by_url": {},
		"send_http_request_packet": {},

		// amap 系列 (高德地图相关, 与安全/通用 ReAct 场景无关)
		"walking_plan":   {},
		"transit_plan":   {},
		"get_weather":    {},
		"get_location":   {},
		"get_distance":   {},
		"get_arround":    {},
		"driving_plan":   {},
		"bicycling_plan": {},
	}

	// hiddenToolPrefixes 给 hidden 系列工具留前缀兜底口子. 当前没有可靠的
	// 命名约定能区分 amap 工具 (它们直接叫 walking_plan / get_weather 这种
	// 普通名字), 所以默认不放任何前缀, 留作未来扩展.
	//
	// 关键词: hidden tool prefixes, future extension
	hiddenToolPrefixes = []string{}

	// scenarioToolNames 是默认 scenario 工具的精确名字集合.
	//
	// 命中规则:
	//   - ssatools 包注册的 Go 内建工具 5 个 (ssa-project-info / ssa-list-files
	//     / ssa-read-file / ssa-grep / check-syntaxflow-syntax)
	//   - yakscriptforai/ssa 下的 3 个 .yak 工具 (check_syntaxflow_syntax /
	//     poc_template_searcher / check_yaklang_syntax)
	//
	// 这些工具只在代码审计 / SyntaxFlow 规则编写场景有意义, 默认 inventory
	// 不展示, 让相关 focus 模式通过 __SCENARIO_TOOLS__ 拉回.
	//
	// 关键词: scenario tool names, ssa, syntaxflow, code audit focus
	scenarioToolNames = map[string]struct{}{
		// ssatools 包 Go 内建 (ssa_tools.go / syntaxflow.go)
		"ssa-project-info":         {},
		"ssa-list-files":           {},
		"ssa-read-file":            {},
		"ssa-grep":                 {},
		"check-syntaxflow-syntax":  {},

		// yakscriptforai/ssa/*.yak 脚本
		"check_syntaxflow_syntax": {},
		"poc_template_searcher":   {},
		"check_yaklang_syntax":    {},
	}

	// scenarioToolPrefixes 是 scenario 系列名字前缀兜底. 主要给 ssa-xxx
	// 与 ssa_xxx 这两种命名风格留位置, 让未来新增的 ssa 工具自动归类,
	// 不需要每次都回来更新名单.
	//
	// 关键词: scenario tool prefixes, ssa- prefix, ssa_ prefix, future safety
	scenarioToolPrefixes = []string{
		"ssa-",
		"ssa_",
	}
)

// LookupToolVisibility 返回某个工具名当前的可见性判定.
//
// 优先级:
//  1. 运行时 / 编译期登记进 hiddenToolNames -> VisibilityHidden
//  2. hiddenToolPrefixes 命中 -> VisibilityHidden
//  3. 运行时 / 编译期登记进 scenarioToolNames -> VisibilityScenario
//  4. scenarioToolPrefixes 命中 -> VisibilityScenario
//  5. 其他 -> VisibilityNormal
//
// hidden 永远优先于 scenario, 避免冲突时把"已弃用"的工具误判成"可拉回".
//
// 关键词: LookupToolVisibility, hidden first, prefix fallback
func LookupToolVisibility(name string) Visibility {
	if name == "" {
		return VisibilityNormal
	}
	visibilityMu.RLock()
	defer visibilityMu.RUnlock()
	if _, ok := hiddenToolNames[name]; ok {
		return VisibilityHidden
	}
	for _, p := range hiddenToolPrefixes {
		if p == "" {
			continue
		}
		if strings.HasPrefix(name, p) {
			return VisibilityHidden
		}
	}
	if _, ok := scenarioToolNames[name]; ok {
		return VisibilityScenario
	}
	for _, p := range scenarioToolPrefixes {
		if p == "" {
			continue
		}
		if strings.HasPrefix(name, p) {
			return VisibilityScenario
		}
	}
	return VisibilityNormal
}

// IsHiddenTool 返回 true 当且仅当工具属于 VisibilityHidden.
// 关键词: IsHiddenTool
func IsHiddenTool(name string) bool {
	return LookupToolVisibility(name) == VisibilityHidden
}

// IsScenarioTool 返回 true 当且仅当工具属于 VisibilityScenario.
// 关键词: IsScenarioTool
func IsScenarioTool(name string) bool {
	return LookupToolVisibility(name) == VisibilityScenario
}

// IsDefaultVisibleTool 返回 true 当且仅当工具属于 VisibilityNormal,
// 即默认会出现在 Tool Inventory 中.
// 关键词: IsDefaultVisibleTool, default inventory filter
func IsDefaultVisibleTool(name string) bool {
	return LookupToolVisibility(name) == VisibilityNormal
}

// RegisterHiddenTool 把一个工具名注册进 hidden 名单. 主要给:
//   - Go 单测里临时染色 / 解染
//   - 运行时插件场景需要"动态废弃"某个工具
//
// 重复注册幂等. 已经在 scenario 名单里的名字会被覆盖为 hidden (因为
// hidden 优先级更高).
//
// 关键词: RegisterHiddenTool, runtime registration
func RegisterHiddenTool(name string) {
	if name == "" {
		return
	}
	visibilityMu.Lock()
	defer visibilityMu.Unlock()
	hiddenToolNames[name] = struct{}{}
	delete(scenarioToolNames, name)
}

// RegisterScenarioTool 把一个工具名注册进 scenario 名单. 主要给:
//   - Go 单测里临时染色 / 解染
//   - 运行时插件场景需要"动态归为特定场景"
//
// 如果该名字已经在 hidden 名单里, 不会被降级 (hidden 永远更高).
//
// 关键词: RegisterScenarioTool, runtime registration, hidden wins
func RegisterScenarioTool(name string) {
	if name == "" {
		return
	}
	visibilityMu.Lock()
	defer visibilityMu.Unlock()
	if _, ok := hiddenToolNames[name]; ok {
		return
	}
	scenarioToolNames[name] = struct{}{}
}

// UnregisterToolVisibility 把工具从 hidden / scenario 名单都移除, 让它
// 回到 VisibilityNormal. 主要给单测用 (避免不同测试之间互相污染).
//
// 关键词: UnregisterToolVisibility, test reset
func UnregisterToolVisibility(name string) {
	if name == "" {
		return
	}
	visibilityMu.Lock()
	defer visibilityMu.Unlock()
	delete(hiddenToolNames, name)
	delete(scenarioToolNames, name)
}

// FilterToolsByVisibility 按当前可见性策略过滤工具列表, 给默认 Tool
// Inventory 渲染入口使用 (GetLoopPromptBaseMaterials / GetBasicPromptInfo).
//
// 规则:
//   - VisibilityHidden 永远过滤掉.
//   - VisibilityScenario 默认过滤掉; 仅当 scenarioWhitelist 命中时保留.
//   - VisibilityNormal 保留.
//
// 顺序保持与入参一致, 不做排序 (排序在调用方的 getPrioritizedTools 里做,
// 比如把 whitelist 命中的 scenario 工具置顶).
//
// nil-safe: tools 为 nil 时返回 nil; scenarioWhitelist 为 nil 时退化为
// "只保留 normal".
//
// 关键词: FilterToolsByVisibility, default inventory entry filter,
//        scenario whitelist, hidden drop
func FilterToolsByVisibility(tools []*aitool.Tool, scenarioWhitelist []string) []*aitool.Tool {
	if len(tools) == 0 {
		return tools
	}
	whitelist := make(map[string]struct{}, len(scenarioWhitelist))
	for _, n := range scenarioWhitelist {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		whitelist[n] = struct{}{}
	}
	result := make([]*aitool.Tool, 0, len(tools))
	for _, t := range tools {
		if t == nil {
			continue
		}
		name := t.GetName()
		switch LookupToolVisibility(name) {
		case VisibilityHidden:
			// hidden 一律丢弃, 即便被 whitelist 提到也不允许 (语义: 已废弃)
			continue
		case VisibilityScenario:
			if _, ok := whitelist[name]; !ok {
				continue
			}
			result = append(result, t)
		default:
			result = append(result, t)
		}
	}
	return result
}
