package aireact

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// hidden tool pattern 在 prompt 主路径 (GetLoopPromptBaseMaterials +
// frozen_block_section.txt 渲染) 上的端到端回归: 给一组混合工具, 默认
// 不带 scenario 白名单时, 默认 Tool Inventory 段不能出现 amap / 已被
// do_http_request 覆盖的几个 http 工具 / ssa-* 系列, 且 ToolsCount /
// MoreToolsCount 必须按"过滤后"统计.
//
// 关键词: hidden tool pattern regression, GetLoopPromptBaseMaterials filter,
//        frozen_block_section render, ToolsCount after filter

func mkVisToolWithDesc(name, desc string) *aitool.Tool {
	return aitool.NewWithoutCallback(name, aitool.WithDescription(desc))
}

// TestPromptManager_ToolInventory_ExcludesHiddenAndScenario_Default 默认 (无
// scenario 白名单) 场景下, GetLoopPromptBaseMaterials 应当把 hidden + scenario
// 工具全部从 TopTools 中剔除, ToolsCount 反映过滤后的数字, 渲染出的 frozen
// block 文本里不能出现这些工具名.
//
// 关键词: TestPromptManager_ToolInventory_ExcludesHiddenAndScenario_Default,
//        default inventory filter, amap excluded, ssa- excluded
func TestPromptManager_ToolInventory_ExcludesHiddenAndScenario_Default(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"ok"},"cumulative_summary":"ok","human_readable_thought":"ok"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	// 入参挑了一组覆盖 normal / hidden / scenario 三类的代表性工具:
	//   normal:   do_http_request, grep, read_file
	//   hidden:   url_content_summary, send_http_request_by_url,
	//             send_http_request_packet, walking_plan (amap)
	//   scenario: ssa-grep, ssa-list-files, check_syntaxflow_syntax (.yak)
	mixedTools := []*aitool.Tool{
		mkVisToolWithDesc("do_http_request", "send http request"),
		mkVisToolWithDesc("url_content_summary", "deprecated http summary"),
		mkVisToolWithDesc("send_http_request_by_url", "deprecated"),
		mkVisToolWithDesc("send_http_request_packet", "deprecated"),
		mkVisToolWithDesc("walking_plan", "amap walking"),
		mkVisToolWithDesc("ssa-grep", "ssa grep"),
		mkVisToolWithDesc("ssa-list-files", "ssa list files"),
		mkVisToolWithDesc("check_syntaxflow_syntax", "ssa .yak"),
		mkVisToolWithDesc("grep", "grep tool"),
		mkVisToolWithDesc("read_file", "read file"),
	}
	wantNormalNames := map[string]struct{}{
		"do_http_request": {},
		"grep":            {},
		"read_file":       {},
	}
	bannedNames := []string{
		"url_content_summary",
		"send_http_request_by_url",
		"send_http_request_packet",
		"walking_plan",
		"ssa-grep",
		"ssa-list-files",
		"check_syntaxflow_syntax",
	}

	materials, err := react.promptManager.GetLoopPromptBaseMaterials(mixedTools, "vis01")
	require.NoError(t, err)
	require.NotNil(t, materials)

	// ToolsCount 必须用过滤后的总数, 让模板里 "You have access to N built-in
	// tools" 不再算上被过滤掉的工具.
	require.Equal(t, len(wantNormalNames), materials.ToolsCount,
		"ToolsCount should reflect post-filter total (normal-only); got %d, materials=%+v",
		materials.ToolsCount, materials)

	require.NotEmpty(t, materials.TopTools, "expect at least normal tools to survive filter")
	require.LessOrEqual(t, len(materials.TopTools), len(wantNormalNames),
		"TopTools cannot exceed the post-filter total")

	gotTopNames := make(map[string]struct{}, len(materials.TopTools))
	for _, tl := range materials.TopTools {
		require.NotNil(t, tl)
		name := tl.GetName()
		gotTopNames[name] = struct{}{}
		_, ok := wantNormalNames[name]
		require.True(t, ok, "TopTools must only contain normal tools; got banned/unknown name %q", name)
	}
	for n := range wantNormalNames {
		_, ok := gotTopNames[n]
		require.True(t, ok, "TopTools should contain normal tool %q (post-filter list shrank to %d items, all should survive within budget)", n, len(materials.TopTools))
	}

	// MoreToolsCount 也要按过滤后统计, 不能误把 hidden/scenario 计入"还有 N 个工具
	// 未列入" 提示, 否则 search_capabilities 提示会说出明显不实的数字.
	wantMore := materials.ToolsCount - materials.TopToolsCount
	if wantMore < 0 {
		wantMore = 0
	}
	require.Equal(t, wantMore, materials.MoreToolsCount,
		"MoreToolsCount must be ToolsCount - TopToolsCount")

	// 最后用 frozen block 渲染做端到端断言: 输出文本里绝对不能出现 hidden /
	// scenario 工具名 (它们既没进 TopTools 也没进 More 提示).
	rendered, err := react.promptManager.renderLoopFrozenBlockSection(&reactloops.PromptPrefixMaterials{
		ToolInventory:  true,
		ToolsCount:     materials.ToolsCount,
		TopToolsCount:  materials.TopToolsCount,
		TopTools:       materials.TopTools,
		HasMoreTools:   materials.HasMoreTools,
		MoreToolsCount: materials.MoreToolsCount,
	})
	require.NoError(t, err)
	for _, banned := range bannedNames {
		require.NotContains(t, rendered, "`"+banned+"`",
			"frozen block rendering must NOT mention hidden/scenario tool %q", banned)
	}
	// 反向断言 normal 工具至少有一个出现.
	require.True(t,
		strings.Contains(rendered, "`do_http_request`") ||
			strings.Contains(rendered, "`grep`") ||
			strings.Contains(rendered, "`read_file`"),
		"frozen block rendering should contain at least one normal tool name",
	)
}
