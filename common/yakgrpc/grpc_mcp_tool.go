package yakgrpc

import (
	"context"

	"github.com/jinzhu/gorm"
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
//   - "builtin" tools: always synced from the in-memory global registry on each
//     call; description/params come from the live Go definition.
//   - "bridge" tools: upserted from enabled external MCP servers only on first
//     encounter (i.e. when no row exists in the DB yet). Subsequent calls read
//     directly from the DB to avoid expensive network round-trips on every page
//     load. Callers that need a forced refresh can call SyncMCPBridgeTools (not
//     yet exposed as an RPC — add if needed).
func (s *Server) GetMCPToolList(ctx context.Context, req *ypb.GetMCPToolListRequest) (*ypb.GetMCPToolListResponse, error) {
	db := s.GetProfileDatabase()

	// 1. Sync builtin tools into DB (cheap: in-memory map iteration + upsert).
	// Description for builtin tools is always read from the live Go definition,
	// so we don't store it — pass empty string here.
	builtinTools := mcp.GlobalBuiltinTools()
	for name := range builtinTools {
		if _, err := yakit.GetOrCreateMCPToolConfig(db, name, schema.MCPToolSourceBuiltin, "", ""); err != nil {
			log.Warnf("GetMCPToolList: upsert builtin tool config %q: %v", name, err)
		}
	}

	// 2. Reconcile bridge tools from external MCP servers.
	// ForceSync=true: always dial every server and do a full diff (add/update/delete).
	// ForceSync=false: only dial servers that have at least one tool row with an
	//   empty description, keeping the common case free of network round-trips.
	syncMCPBridgeTools(ctx, s, db, req.GetForceSync())

	// 3. Query the config table with the requested filters.
	paginator, cfgs, err := yakit.QueryMCPToolConfigs(db, req)
	if err != nil {
		return nil, err
	}

	tools := make([]*ypb.MCPToolConfig, 0, len(cfgs))
	for _, cfg := range cfgs {
		item := cfg.ToGRPC()
		attachToolMeta(item, cfg.Source, cfg.ToolName, cfg.ServerName)
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
func (s *Server) GetMCPToolDetail(ctx context.Context, req *ypb.GetMCPToolDetailRequest) (*ypb.MCPToolConfig, error) {
	if req.GetToolName() == "" {
		return nil, utils.Errorf("tool name is required")
	}

	db := s.GetProfileDatabase()
	cfg, err := yakit.GetMCPToolConfigByName(db, req.GetToolName())
	if err != nil {
		return nil, err
	}

	item := cfg.ToGRPC()
	attachToolMeta(item, cfg.Source, cfg.ToolName, cfg.ServerName)

	// For bridge tools whose metadata is not in memory, try a live lookup.
	if cfg.Source == schema.MCPToolSourceBridge && item.Description == "" {
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
	if err := yakit.SetMCPToolEnabled(s.GetProfileDatabase(), req.GetToolName(), req.GetEnable()); err != nil {
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
	return yakit.GetDisabledMCPToolNames(db)
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

// syncMCPBridgeTools reconciles bridge tool rows against enabled external MCP servers.
//
// For each server:
//   - tools present remotely but missing locally  → inserted
//   - tools present both remotely and locally      → description refreshed
//   - tools present locally but gone from remote   → deleted
//
// When forceSync is false, a server is skipped if all its existing tool rows
// already have a non-empty description (i.e. it was fully synced before).
// When forceSync is true, every server is dialed regardless.
//
// If a server is unreachable, it is skipped to avoid false deletions.
func syncMCPBridgeTools(ctx context.Context, s *Server, db *gorm.DB, forceSync bool) {
	for srv := range yakit.YieldEnabledMCPServers(ctx, db) {
		if !forceSync {
			// Fast path: skip if all rows for this server have descriptions.
			var emptyDescCount int
			db.Model(&schema.MCPToolConfig{}).
				Where("source = ? AND server_name = ? AND (description = '' OR description IS NULL)",
					schema.MCPToolSourceBridge, srv.Name).
				Count(&emptyDescCount)

			var totalCount int
			db.Model(&schema.MCPToolConfig{}).
				Where("source = ? AND server_name = ?", schema.MCPToolSourceBridge, srv.Name).
				Count(&totalCount)

			if totalCount > 0 && emptyDescCount == 0 {
				continue
			}
		}

		remoteTools, err := s.getMCPServerTools(ctx, srv)
		if err != nil {
			// Server unreachable — leave existing rows untouched.
			log.Warnf("syncMCPBridgeTools: get tools from server %q: %v, skipping", srv.Name, err)
			continue
		}

		// Upsert every tool reported by the remote server.
		keepNames := make(map[string]struct{}, len(remoteTools))
		for _, t := range remoteTools {
			canonicalName := buildBridgeToolName(srv.Name, t.Name)
			keepNames[canonicalName] = struct{}{}
			if err := yakit.UpsertMCPToolConfigDescription(db, canonicalName, schema.MCPToolSourceBridge, srv.Name, t.Description); err != nil {
				log.Warnf("syncMCPBridgeTools: upsert %q: %v", canonicalName, err)
			}
		}

		// Prune rows for tools the remote server no longer provides.
		if err := yakit.DeleteMCPToolConfigsByServerAndNames(db, srv.Name, keepNames); err != nil {
			log.Warnf("syncMCPBridgeTools: prune stale tools for server %q: %v", srv.Name, err)
		}
	}
}

// attachToolMeta fills in Description and Params on an MCPToolConfig proto
// message from the live in-memory tool definition (for builtin tools).
// Bridge tool metadata is left empty here — the caller may enrich it separately
// if a live connection is available.
func attachToolMeta(item *ypb.MCPToolConfig, source, toolName, _ string) {
	if source != schema.MCPToolSourceBuiltin {
		return
	}
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
		log.Warnf("attachToolMeta: parse schema for %q: %v", toolName, err)
		return
	}
	item.Params = params
}

// buildBridgeToolName produces the canonical name for a bridge tool, matching
// the convention in common/ai/aid/aitool/mcp_server_loader.go:
//
//	mcp_{ServerName}_{ToolName}
func buildBridgeToolName(serverName, toolName string) string {
	return "mcp_" + serverName + "_" + toolName
}

// extractOriginalToolName reverses buildBridgeToolName:
//
//	"mcp_IDA-MCP_decompile" + "IDA-MCP" → "decompile"
func extractOriginalToolName(canonicalName, serverName string) string {
	prefix := "mcp_" + serverName + "_"
	if len(canonicalName) > len(prefix) {
		return canonicalName[len(prefix):]
	}
	return canonicalName
}
