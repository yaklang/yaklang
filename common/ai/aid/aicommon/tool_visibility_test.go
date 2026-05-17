package aicommon

import (
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// 单测组织原则:
//   - 每个 Test 围绕一个 hidden tool pattern 关注点写, 命名为
//     TestToolVisibility_<焦点>, 方便 go test -run TestToolVisibility 一把跑.
//   - 任何用到 RegisterXxx / UnregisterXxx 的用例都必须用 defer
//     UnregisterToolVisibility 清理, 避免污染同包内其他测试 (尤其是 amap /
//     ssa / http 那一批硬编码名单).
//
// 关键词: tool visibility tests, hidden tool pattern, filter inventory,
//        scenario whitelist, runtime register reset

// TestToolVisibility_InitialHiddenNames 验证 plan 中列出的全部初始 hidden 名字
// 都被命中, 防止 hiddenToolNames 被未来重构时无意义地裁掉.
//
// 关键词: initial hidden names, amap, http deprecated
func TestToolVisibility_InitialHiddenNames(t *testing.T) {
	hiddenCases := []string{
		// HTTP 已有替代品
		"url_content_summary",
		"send_http_request_by_url",
		"send_http_request_packet",
		// amap 全 8 个
		"walking_plan",
		"transit_plan",
		"get_weather",
		"get_location",
		"get_distance",
		"get_arround",
		"driving_plan",
		"bicycling_plan",
	}
	for _, name := range hiddenCases {
		if !IsHiddenTool(name) {
			t.Fatalf("expect %q to be hidden, got %s", name, LookupToolVisibility(name))
		}
		if IsScenarioTool(name) {
			t.Fatalf("expect %q NOT to be scenario", name)
		}
		if IsDefaultVisibleTool(name) {
			t.Fatalf("expect %q NOT to be default-visible", name)
		}
	}
}

// TestToolVisibility_InitialScenarioNames 验证 plan 中列出的全部初始 scenario
// 名字都被命中, 防止 scenarioToolNames 被裁掉.
//
// 关键词: initial scenario names, ssa go tools, ssa yak scripts
func TestToolVisibility_InitialScenarioNames(t *testing.T) {
	scenarioCases := []string{
		// ssatools 包 Go 内建
		"ssa-project-info",
		"ssa-list-files",
		"ssa-read-file",
		"ssa-grep",
		"check-syntaxflow-syntax",
		// yakscriptforai/ssa/*.yak
		"check_syntaxflow_syntax",
		"poc_template_searcher",
		"check_yaklang_syntax",
	}
	for _, name := range scenarioCases {
		if !IsScenarioTool(name) {
			t.Fatalf("expect %q to be scenario, got %s", name, LookupToolVisibility(name))
		}
		if IsHiddenTool(name) {
			t.Fatalf("expect %q NOT to be hidden", name)
		}
		if IsDefaultVisibleTool(name) {
			t.Fatalf("expect %q NOT to be default-visible", name)
		}
	}
}

// TestToolVisibility_PrefixFallback 验证未来如果新增 ssa-xxx / ssa_xxx 工具,
// 即便没有手动登记进 scenarioToolNames, 也会被前缀兜底归类为 scenario.
//
// 关键词: prefix fallback, ssa- prefix, ssa_ prefix
func TestToolVisibility_PrefixFallback(t *testing.T) {
	prefixCases := []string{
		"ssa-newcomer",
		"ssa_newcomer",
		"ssa-very-long-name",
	}
	for _, name := range prefixCases {
		if !IsScenarioTool(name) {
			t.Fatalf("expect prefix fallback to mark %q as scenario, got %s", name, LookupToolVisibility(name))
		}
	}

	normalCases := []string{
		"",
		"do_http_request",
		"grep",
		"read_file",
		"poc_searcher", // 不带 ssa 前缀, 应当 normal
	}
	for _, name := range normalCases {
		if !IsDefaultVisibleTool(name) {
			t.Fatalf("expect %q to be default-visible, got %s", name, LookupToolVisibility(name))
		}
	}
}

// TestToolVisibility_FilterDropsHiddenAndScenarioByDefault 验证默认 (无 whitelist)
// 情况下, FilterToolsByVisibility 会把 hidden + scenario 都剔除, 只留 normal.
// 顺序应保持稳定 (与入参顺序一致).
//
// 关键词: FilterToolsByVisibility default behavior, drop hidden and scenario,
//        stable order
func TestToolVisibility_FilterDropsHiddenAndScenarioByDefault(t *testing.T) {
	tools := []*aitool.Tool{
		mkVisTool("do_http_request"),  // normal
		mkVisTool("walking_plan"),     // hidden (amap)
		mkVisTool("ssa-grep"),         // scenario
		mkVisTool("grep"),             // normal
		mkVisTool("ssa-newcomer"),     // scenario (prefix)
		mkVisTool("url_content_summary"), // hidden
	}

	got := FilterToolsByVisibility(tools, nil)

	wantNames := []string{"do_http_request", "grep"}
	if len(got) != len(wantNames) {
		t.Fatalf("expect %d tools (only normal), got %d (%v)", len(wantNames), len(got), namesOf(got))
	}
	for i, name := range wantNames {
		if got[i].GetName() != name {
			t.Fatalf("expect index %d name=%q, got %q (full=%v)", i, name, got[i].GetName(), namesOf(got))
		}
	}
}

// TestToolVisibility_FilterKeepsWhitelistedScenario 验证当 whitelist 命中后,
// 对应的 scenario 工具会被保留 (顺序保持); 未命中的 scenario / 所有 hidden
// 仍然被过滤掉; whitelist 命中 hidden 不能让 hidden 复活 (语义: hidden 永远禁用).
//
// 关键词: FilterToolsByVisibility scenario whitelist, hidden never returns,
//        focus mode pull back scenario
func TestToolVisibility_FilterKeepsWhitelistedScenario(t *testing.T) {
	tools := []*aitool.Tool{
		mkVisTool("do_http_request"),
		mkVisTool("ssa-grep"),
		mkVisTool("ssa-list-files"),
		mkVisTool("walking_plan"),
		mkVisTool("read_file"),
	}

	// 期望 whitelist 命中的 ssa-grep 进入结果; ssa-list-files 仍然过滤掉;
	// walking_plan 因为是 hidden, 即便我们把它塞进 whitelist 也不可复活.
	got := FilterToolsByVisibility(tools, []string{"ssa-grep", "walking_plan", "  ", ""})

	wantNames := []string{"do_http_request", "ssa-grep", "read_file"}
	if len(got) != len(wantNames) {
		t.Fatalf("expect %d tools, got %d (%v)", len(wantNames), len(got), namesOf(got))
	}
	for i, name := range wantNames {
		if got[i].GetName() != name {
			t.Fatalf("expect index %d name=%q, got %q (full=%v)", i, name, got[i].GetName(), namesOf(got))
		}
	}
}

// TestToolVisibility_RuntimeRegister 验证 RegisterHiddenTool /
// RegisterScenarioTool 的运行时染色行为, 包括 hidden 优先于 scenario 这条
// 关键不变量.
//
// 关键词: runtime register, hidden wins over scenario, UnregisterToolVisibility cleanup
func TestToolVisibility_RuntimeRegister(t *testing.T) {
	const adHoc = "ad_hoc_visibility_test_xyz"
	defer UnregisterToolVisibility(adHoc)

	if !IsDefaultVisibleTool(adHoc) {
		t.Fatalf("precondition: %q should be normal initially", adHoc)
	}

	RegisterHiddenTool(adHoc)
	if !IsHiddenTool(adHoc) {
		t.Fatalf("after RegisterHiddenTool, %q should be hidden", adHoc)
	}

	// scenario 注册不能把它降级回 scenario (hidden 优先)
	RegisterScenarioTool(adHoc)
	if !IsHiddenTool(adHoc) {
		t.Fatalf("scenario register MUST NOT downgrade hidden, but %q now %s", adHoc, LookupToolVisibility(adHoc))
	}

	UnregisterToolVisibility(adHoc)
	if !IsDefaultVisibleTool(adHoc) {
		t.Fatalf("after Unregister, %q should be back to normal, got %s", adHoc, LookupToolVisibility(adHoc))
	}

	// 反向: 先 scenario 再 hidden, hidden 覆盖 scenario.
	RegisterScenarioTool(adHoc)
	if !IsScenarioTool(adHoc) {
		t.Fatalf("after RegisterScenarioTool, %q should be scenario", adHoc)
	}
	RegisterHiddenTool(adHoc)
	if !IsHiddenTool(adHoc) {
		t.Fatalf("RegisterHiddenTool MUST override scenario, but %q now %s", adHoc, LookupToolVisibility(adHoc))
	}
}

// TestToolVisibility_FilterNilSafety 验证 nil / 空入参不会 panic.
// 关键词: nil safety, empty input
func TestToolVisibility_FilterNilSafety(t *testing.T) {
	if got := FilterToolsByVisibility(nil, nil); got != nil {
		t.Fatalf("nil input should return nil, got %v", got)
	}
	if got := FilterToolsByVisibility([]*aitool.Tool{}, nil); len(got) != 0 {
		t.Fatalf("empty input should return empty, got %v", got)
	}

	// 入参里夹杂 nil tool, 不应 panic, 应跳过
	tools := []*aitool.Tool{nil, mkVisTool("grep"), nil, mkVisTool("walking_plan")}
	got := FilterToolsByVisibility(tools, nil)
	if len(got) != 1 || got[0].GetName() != "grep" {
		t.Fatalf("nil entries should be skipped, hidden dropped; got %v", namesOf(got))
	}
}

// mkVisTool 是单测专用的最小 tool 构造器, 只携带 name, 不挂 callback.
// 与 tool_inventory_budget_test.go 里的 mkBudgetTool 不冲突, 二者各自服务.
// 关键词: test helper, minimal aitool, no callback
func mkVisTool(name string) *aitool.Tool {
	return aitool.NewWithoutCallback(name)
}

func namesOf(tools []*aitool.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		if t == nil {
			names = append(names, "<nil>")
			continue
		}
		names = append(names, t.GetName())
	}
	return names
}
