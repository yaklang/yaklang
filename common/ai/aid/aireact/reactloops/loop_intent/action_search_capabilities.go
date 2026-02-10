package loop_intent

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// searchCapabilitiesAction creates the search_capabilities action
// that searches for tools, forges, and focus modes matching the user's intent.
var searchCapabilitiesAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return makeSearchCapabilitiesAction(r)
}

func makeSearchCapabilitiesAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	desc := "搜索与用户意图匹配的可用能力（工具、AI 蓝图、专注模式）。" +
		"使用 BM25 Trigram 搜索工具和蓝图，关键词匹配专注模式。/ " +
		"Search for available capabilities (tools, AI forges, focus modes) matching the user's intent."

	toolOpts := []aitool.ToolOption{
		aitool.WithStringParam("search_query",
			aitool.WithParam_Description("搜索关键词，从用户输入中提取核心动作词和领域术语。/ Keywords to search for relevant capabilities."),
			aitool.WithParam_Required(true),
		),
	}

	return reactloops.WithRegisterLoopAction(
		"search_capabilities",
		desc,
		toolOpts,
		// Verifier
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			query := strings.TrimSpace(action.GetString("search_query"))
			if query == "" {
				return utils.Error("search_query is required for capability search")
			}
			return nil
		},
		// Handler
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			query := strings.TrimSpace(action.GetString("search_query"))
			log.Infof("intent loop: searching capabilities with query: %s", query)

			var results strings.Builder
			results.WriteString(fmt.Sprintf("## Capability Search Results for: %s\n\n", query))

			// 使用 ToolRecommender 执行 BM25 搜索
			// 通过类型断言获取 ToolRecommender
			type toolRecommenderGetter interface {
				GetToolRecommender() *reactloops.ToolRecommender
			}

			getter, ok := r.(toolRecommenderGetter)
			if !ok {
				log.Warnf("intent loop: runtime does not support GetToolRecommender")
				results.WriteString("### Error\nToolRecommender is not available.\n\n")
				op.Feedback(results.String())
				op.Continue()
				return
			}

			recommender := getter.GetToolRecommender()
			if recommender == nil {
				log.Warnf("intent loop: ToolRecommender is nil")
				results.WriteString("### Error\nToolRecommender is not initialized.\n\n")
				op.Feedback(results.String())
				op.Continue()
				return
			}

			// 执行搜索并更新缓存
			toolLimit := 10
			forgeLimit := 10
			loopLimit := 10

			if err := recommender.SearchAndUpdateCache(query, toolLimit, forgeLimit, loopLimit); err != nil {
				log.Warnf("intent loop: capability search failed: %v", err)
				results.WriteString(fmt.Sprintf("### Error\nSearch failed: %v\n\n", err))
				op.Feedback(results.String())
				op.Continue()
				return
			}

			// 获取搜索结果
			tools, forges := recommender.GetCachedToolsAndForges()
			loops := recommender.GetCachedLoops()

			// 1. 显示匹配的工具
			if len(tools) > 0 {
				results.WriteString("### Matched Tools\n")
				var toolNames []string
				for _, tool := range tools {
					name := tool.VerboseName
					if name == "" {
						name = tool.Tool.Name
					} else {
						name = name + " (" + tool.Tool.Name + ")"
					}
					desc := tool.Tool.Description
					if len(desc) > 200 {
						desc = desc[:200] + "..."
					}
					results.WriteString(fmt.Sprintf("- **%s**: %s", name, desc))
					if len(tool.Keywords) > 0 {
						results.WriteString(fmt.Sprintf(" [keywords: %s]", strings.Join(tool.Keywords, ", ")))
					}
					results.WriteString("\n")
					toolNames = append(toolNames, tool.Tool.Name)
				}
				results.WriteString("\n")
				log.Infof("intent loop: found %d tools via BM25", len(tools))
				loop.Set("matched_tool_names", strings.Join(toolNames, ","))
			} else {
				results.WriteString("### Tools\nNo matching tools found.\n\n")
			}

			// 2. 显示匹配的 forges
			if len(forges) > 0 {
				results.WriteString("### Matched AI Forges (Blueprints)\n")
				var forgeNames []string
				for _, forge := range forges {
					name := forge.ForgeName
					if forge.ForgeVerboseName != "" {
						name = forge.ForgeVerboseName + " (" + forge.ForgeName + ")"
					}
					desc := forge.Description
					if len(desc) > 200 {
						desc = desc[:200] + "..."
					}
					results.WriteString(fmt.Sprintf("- **%s**: %s\n", name, desc))
					forgeNames = append(forgeNames, forge.ForgeName)
				}
				results.WriteString("\n")
				log.Infof("intent loop: found %d forges", len(forges))
				loop.Set("matched_forge_names", strings.Join(forgeNames, ","))
			} else {
				results.WriteString("### AI Forges\nNo matching forges found.\n\n")
			}

			// 3. 显示匹配的 loops
			if len(loops) > 0 {
				results.WriteString("### Matched Focus Modes\n")
				var loopNames []string
				for _, meta := range loops {
					results.WriteString(fmt.Sprintf("- **%s**: %s\n", meta.Name, meta.Description))
					loopNames = append(loopNames, meta.Name)
				}
				results.WriteString("\n")
				log.Infof("intent loop: found %d matching focus modes", len(loops))
				loop.Set("matched_loop_names", strings.Join(loopNames, ","))
			} else {
				// 也显示可用的专注模式供参考
				availableModes := loop.Get("available_focus_modes")
				if availableModes != "" {
					results.WriteString("### Available Focus Modes (no direct match)\n")
					results.WriteString(availableModes)
					results.WriteString("\n")
				}
			}

			// 存储搜索结果
			existingResults := loop.Get("search_results")
			if existingResults != "" {
				existingResults += "\n---\n\n"
			}
			loop.Set("search_results", existingResults+results.String())

			op.Feedback(results.String())
			op.Continue()
		},
	)
}
