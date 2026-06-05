package yakgrpc

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// GetMCPToolList returns the merged list of builtin tools and bridge tools with
// their per-tool enable/disable state. The result is paged and filterable.
//
// Tool discovery strategy:
//   - "builtin" tools: legacy MCP registry (common/mcp globalTools).
//   - "aitool" tools: aitool-framework builtin registry.
//     Both are synced in-memory on each call; stale rows are pruned per source.
//   - "bridge" tools: two modes controlled by ForceSync:
//   - ForceSync=false (default): read entirely from the DB cache, no network.
//     Stale rows may persist until the next ForceSync. Frontend should call
//     with ForceSync=true whenever the user explicitly requests a refresh.
//   - ForceSync=true: dial every enabled external MCP server, perform a full
//     diff (insert new tools, refresh descriptions, delete removed tools).
func (s *Server) GetMCPToolList(ctx context.Context, req *ypb.GetMCPToolListRequest) (*ypb.GetMCPToolListResponse, error) {
	db := s.GetProfileDatabase()

	// 1. Sync legacy builtin and aitool-framework builtin rows separately.
	aitoolBuiltinMap := syncMCPToolConfigSources(db)

	// 2. Reconcile bridge tools only when explicitly requested.
	// ForceSync=false: skip all network calls, serve from DB cache.
	// ForceSync=true:  dial every server, full diff (insert/update/delete).
	if req.GetForceSync() {
		syncMCPBridgeTools(ctx, s, db)
	}

	// 3. Query the config table with the requested filters.
	paginator, cfgs, err := yakit.QueryMCPClientToolConfigs(db, req)
	if err != nil {
		return nil, err
	}

	tools := make([]*ypb.MCPClientToolConfig, 0, len(cfgs))
	for _, cfg := range cfgs {
		item := cfg.ToGRPC()
		attachToolMeta(item, cfg.Source, cfg.ToolName, cfg.ServerName, aitoolBuiltinMap)
		if cfg.Source == schema.MCPClientToolSourceBridge {
			attachBridgeToolMeta(db, item)
		}
		tools = append(tools, item)
	}

	return &ypb.GetMCPToolListResponse{
		Tools: tools,
		Pagination: &ypb.Paging{
			Page:  int64(paginator.Page),
			Limit: int64(paginator.Limit),
		},
		Total: int64(paginator.TotalRecord),
	}, nil
}

// GetMCPToolDetail returns the full configuration and metadata for a single tool.
// It always re-reads the live tool definition so that description/params are fresh.
func (s *Server) GetMCPToolDetail(ctx context.Context, req *ypb.GetMCPToolDetailRequest) (*ypb.MCPClientToolConfig, error) {
	if req.GetToolName() == "" {
		return nil, utils.Errorf("tool name is required")
	}

	db := s.GetProfileDatabase()
	cfg, err := yakit.GetMCPClientToolConfigByName(db, req.GetToolName())
	if err != nil {
		return nil, err
	}

	item := cfg.ToGRPC()
	attachToolMeta(item, cfg.Source, cfg.ToolName, cfg.ServerName, lookupAIToolFrameworkBuiltin(db, cfg.ToolName))
	if cfg.Source == schema.MCPClientToolSourceBridge {
		attachBridgeToolMeta(db, item)
	}

	// For bridge tools whose metadata is still missing, try a live lookup.
	if cfg.Source == schema.MCPClientToolSourceBridge && (item.Description == "" || len(item.Params) == 0) {
		srv, srvErr := yakit.GetMCPServerByName(db, cfg.ServerName)
		if srvErr == nil && srv != nil {
			tools, toolsErr := s.getMCPServerTools(ctx, srv)
			if toolsErr == nil {
				originalName := extractOriginalToolName(cfg.ToolName, cfg.ServerName)
				for _, t := range tools {
					if t.Name == originalName {
						item.Description = t.Description
						item.Params = t.Params
						break
					}
				}
			}
		}
	}

	return item, nil
}

// SetMCPToolEnabled enables or disables a single tool by name.
// The change takes effect on the next MCP server start.
func (s *Server) SetMCPToolEnabled(ctx context.Context, req *ypb.SetMCPToolEnabledRequest) (*ypb.GeneralResponse, error) {
	if req.GetToolName() == "" {
		return &ypb.GeneralResponse{Ok: false, Reason: "tool name is required"}, nil
	}
	if err := yakit.SetMCPClientToolEnabled(s.GetProfileDatabase(), req.GetToolName(), req.GetEnable()); err != nil {
		return &ypb.GeneralResponse{Ok: false, Reason: err.Error()}, nil
	}
	return &ypb.GeneralResponse{Ok: true}, nil
}

// GetDisabledMCPToolNamesFromDB is called by launchMcpServer to filter out
// disabled tools before registering them. Returns an empty map on DB errors
// so the server can still start in a degraded state.
func GetDisabledMCPToolNamesFromDB() (map[string]struct{}, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return map[string]struct{}{}, nil
	}
	return yakit.GetDisabledMCPClientToolNames(db)
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

// syncMCPBridgeTools dials every enabled external MCP server and performs a
// full reconciliation against the local DB:
//
//   - tools present remotely but missing locally  → inserted with description
//   - tools present both remotely and locally      → description refreshed
//   - tools present locally but gone from remote   → deleted
//
// If a server is unreachable it is skipped entirely to avoid false deletions.
// This function is only called when ForceSync=true; callers that want to read
// from cache should skip this call altogether.
func syncMCPBridgeTools(ctx context.Context, s *Server, db *gorm.DB) {
	for srv := range yakit.YieldEnabledMCPServers(ctx, db) {
		remoteTools, err := s.getMCPServerTools(ctx, srv)
		if err != nil {
			log.Warnf("syncMCPBridgeTools: get tools from server %q: %v, skipping", srv.Name, err)
			continue
		}

		keepNames := make(map[string]struct{}, len(remoteTools))
		for _, t := range remoteTools {
			canonicalName := buildBridgeToolName(srv.Name, t.Name)
			keepNames[canonicalName] = struct{}{}
			if err := yakit.UpsertMCPClientToolConfigDescription(db, canonicalName, schema.MCPClientToolSourceBridge, srv.Name, t.Description); err != nil {
				log.Warnf("syncMCPBridgeTools: upsert %q: %v", canonicalName, err)
			}
		}

		if err := yakit.DeleteMCPClientToolConfigsByServerAndNames(db, srv.Name, keepNames); err != nil {
			log.Warnf("syncMCPBridgeTools: prune stale tools for server %q: %v", srv.Name, err)
		}
	}
}

// syncBuiltinMCPToolConfigs upserts builtin tool rows from both the legacy MCP
// registry and the aitool-framework builtin registry, then prunes stale rows.
// Returns a name→tool map for live metadata attachment in the same request.
func lookupAIToolFrameworkBuiltin(db *gorm.DB, toolName string) map[string]*aitool.Tool {
	result := make(map[string]*aitool.Tool)
	for _, t := range buildinaitools.GetAllToolsDynamically(db) {
		if t != nil && t.Name == toolName {
			result[toolName] = t
			break
		}
	}
	return result
}

func syncMCPToolConfigSources(db *gorm.DB) map[string]*aitool.Tool {
	legacyNames := make(map[string]struct{})
	aitoolNames := make(map[string]struct{})
	aitoolBuiltinMap := make(map[string]*aitool.Tool)

	for name := range mcp.GlobalBuiltinTools() {
		legacyNames[name] = struct{}{}
		if _, err := yakit.GetOrCreateMCPClientToolConfig(db, name, schema.MCPClientToolSourceBuiltin, "", ""); err != nil {
			log.Warnf("GetMCPToolList: upsert legacy builtin tool config %q: %v", name, err)
		}
	}

	for _, t := range buildinaitools.GetAllToolsDynamically(db) {
		if t == nil || t.Name == "" {
			continue
		}
		aitoolNames[t.Name] = struct{}{}
		aitoolBuiltinMap[t.Name] = t
		desc := ""
		if t.Tool != nil {
			desc = t.Description
		}
		if _, err := yakit.GetOrCreateMCPClientToolConfig(db, t.Name, schema.MCPClientToolSourceAITool, "", desc); err != nil {
			log.Warnf("GetMCPToolList: upsert aitool-framework builtin %q: %v", t.Name, err)
		} else if err := yakit.EnsureMCPClientToolConfigSource(db, t.Name, schema.MCPClientToolSourceAITool); err != nil {
			log.Warnf("GetMCPToolList: migrate aitool source for %q: %v", t.Name, err)
		}
	}

	if err := yakit.DeleteStaleMCPClientBuiltinTools(db, legacyNames); err != nil {
		log.Warnf("GetMCPToolList: prune stale legacy builtin tools: %v", err)
	}
	if err := yakit.DeleteStaleMCPClientAITools(db, aitoolNames); err != nil {
		log.Warnf("GetMCPToolList: prune stale aitool-framework tools: %v", err)
	}
	return aitoolBuiltinMap
}

// attachToolMeta fills in Description and Params on an MCPToolConfig proto
// message from the live in-memory tool definition (for builtin tools).
// Bridge tool metadata is left empty here — the caller may enrich it separately
// if a live connection is available.
// attachBridgeToolMeta fills bridge tool description/params from the MCPServerToolConfig
// metadata cache populated by getMCPServerTools / ForceSync.
func attachBridgeToolMeta(db *gorm.DB, item *ypb.MCPClientToolConfig) {
	if item == nil || item.GetToolName() == "" {
		return
	}
	cached, err := yakit.GetMCPServerToolConfigByFullName(db, item.GetToolName())
	if err != nil || cached == nil {
		return
	}
	if item.GetDescription() == "" && cached.Description != "" {
		item.Description = cached.Description
	}
	if len(item.GetParams()) == 0 {
		item.Params = parseMCPToolParamsJSON(cached.ParamsJSON)
	}
}

func attachToolMeta(item *ypb.MCPClientToolConfig, source, toolName, _ string, aitoolBuiltin map[string]*aitool.Tool) {
	switch source {
	case schema.MCPClientToolSourceBuiltin:
		twh := mcp.GetBuiltinToolByName(toolName)
		if twh == nil {
			return
		}
		t := twh.Tool()
		if t == nil {
			return
		}
		item.Description = t.Description
		params, err := parseMCPToolInputSchema(&t.InputSchema)
		if err != nil {
			log.Warnf("attachToolMeta: parse legacy schema for %q: %v", toolName, err)
			return
		}
		item.Params = params
	case schema.MCPClientToolSourceAITool:
		at, ok := aitoolBuiltin[toolName]
		if !ok || at == nil || at.Tool == nil {
			return
		}
		item.Description = at.Description
		params, err := parseMCPToolInputSchema(&at.InputSchema)
		if err != nil {
			log.Warnf("attachToolMeta: parse aitool-framework schema for %q: %v", toolName, err)
			return
		}
		item.Params = params
	}
}

func buildBridgeToolName(serverName, toolName string) string {
	return yakit.MCPBridgeToolCanonicalName(serverName, toolName)
}

func extractOriginalToolName(canonicalName, serverName string) string {
	if orig := yakit.MCPBridgeToolOriginalName(canonicalName, serverName); orig != "" {
		return orig
	}
	return canonicalName
}
