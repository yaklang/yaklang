package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
)

// RecordRecentlyUsedTool keeps cached examples consistent with the actual loop
// protocol, including a loop option that overrides the parent Config.
func (r *ReActLoop) RecordRecentlyUsedTool(tool *aitool.Tool) buildinaitools.RecentToolCacheMutation {
	if tool != nil && r.pinnedToolEntries[tool.Name] != nil {
		entry := *r.pinnedToolEntries[tool.Name]
		return buildinaitools.RecentToolCacheMutation{Reuse: &entry}
	}
	if config, ok := r.config.(interface {
		RecordRecentlyUsedToolForMode(*aitool.Tool, bool) buildinaitools.RecentToolCacheMutation
	}); ok {
		return config.RecordRecentlyUsedToolForMode(tool, r.FunctionCallModeEnabled())
	}
	return r.config.RecordRecentlyUsedTool(tool)
}
