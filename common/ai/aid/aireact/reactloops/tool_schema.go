package reactloops

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
)

// LoadToolSchemas only loads definitions. It shares cache admission, protocol
// rendering and MCP policy with require_tool and never executes a tool.
func (r *ReActLoop) LoadToolSchemas(ctx context.Context, names []string) []map[string]string {
	return r.loadToolSchemas(ctx, names, false)
}

// PreloadToolSchemas considers only already enabled tools and preserves the
// current cache working set. Missing optional tools do not enter DB lookup.
func (r *ReActLoop) PreloadToolSchemas(ctx context.Context, names []string) []map[string]string {
	return r.loadToolSchemas(ctx, names, true)
}

func (r *ReActLoop) loadToolSchemas(ctx context.Context, names []string, preload bool) []map[string]string {
	config := r.GetConfig()
	if config == nil || config.GetAiToolManager() == nil {
		return nil
	}
	if ctx == nil {
		ctx = config.GetContext()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	names = uniqueToolSchemaNames(names)
	manager := config.GetAiToolManager()
	enabled := make(map[string]*aitool.Tool)
	if preload {
		tools, _ := manager.GetEnableTools()
		for _, tool := range tools {
			if tool != nil {
				enabled[tool.Name] = tool
			}
		}
	}
	results := make([]map[string]string, 0, len(names))
	for _, name := range names {
		if preload && (enabled[name] == nil || buildinaitools.IsMCPPendingStub(enabled[name])) {
			continue
		}
		entry := map[string]string{"tool_name": name, "status": "error"}
		results = append(results, entry)
		if err := ctx.Err(); err != nil {
			entry["detail"] = err.Error()
			continue
		}
		if buildinaitools.IsMCPToolName(name) && !aicommon.IsMCPServersAllowedConfig(config) {
			entry["detail"] = "MCP tools are disabled"
			continue
		}
		tool, err := manager.GetToolByName(name)
		if err == nil && tool != nil && buildinaitools.IsMCPPendingStub(tool) {
			tool, err = buildinaitools.WaitForMCPLiveTool(ctx, manager, name, buildinaitools.MCPToolInitWaitTimeout, buildinaitools.MCPToolInitPollInterval, nil)
		}
		if err != nil || tool == nil {
			entry["detail"] = fmt.Sprintf("Tool unavailable: %v", err)
			continue
		}
		var mutation buildinaitools.RecentToolCacheMutation
		if preload {
			if recorder, ok := config.(interface {
				PreloadRecentlyUsedToolForMode(*aitool.Tool, bool) buildinaitools.RecentToolCacheMutation
			}); ok {
				mutation = recorder.PreloadRecentlyUsedToolForMode(tool, r.FunctionCallModeEnabled())
			}
		} else {
			mutation = r.RecordRecentlyUsedTool(tool)
		}
		if mutation.Upsert == nil && mutation.Reuse == nil {
			entry["detail"] = "Schema could not be cached within the current tool-cache budget"
			continue
		}
		entry["status"] = "schema_loaded"
		entry["detail"] = fmt.Sprintf("工具 %q 的参数 Schema 已加载到 CACHE_TOOL_CALL，尚未执行。现在按 Schema 和当前任务构造完整参数，立即调用 directly_call_tool 继续完成本任务；不要重复加载，不要把加载当作任务完成或单独交付的阶段，也不要等待用户说继续。", name)
		if preload {
			entry["detail"] = "已预加载到 CACHE_TOOL_CALL，未执行工具；仅按当前任务需要复用 Schema 并显式调用，无需重复加载。"
		}
	}
	// Loading later entries can evict earlier ones. Do not report those entries
	// as available in the final cache state of this load batch.
	for _, entry := range results {
		if entry["status"] == "schema_loaded" && !manager.IsRecentlyUsedTool(entry["tool_name"]) {
			entry["status"], entry["detail"] = "evicted", "Schema was evicted by the tool-cache budget; load a smaller set."
		}
	}

	return results
}

func uniqueToolSchemaNames(names []string) []string {
	seen := make(map[string]bool, len(names))
	result := make([]string, 0, len(names))
	for _, name := range names {
		if name != "" && !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	return result
}
