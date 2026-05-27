package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	bizhelper "github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// GetOrCreateMCPToolConfig returns the MCPToolConfig row for the given tool,
// creating it (with Enable=true) if it does not yet exist.
// description is stored on creation; existing rows are NOT updated here —
// use UpsertMCPToolConfigDescription to refresh the description on an existing row.
func GetOrCreateMCPToolConfig(db *gorm.DB, toolName, source, serverName, description string) (*schema.MCPToolConfig, error) {
	cfg := &schema.MCPToolConfig{}
	err := db.Where("tool_name = ?", toolName).First(cfg).Error
	if err == nil {
		return cfg, nil
	}
	if !gorm.IsRecordNotFoundError(err) {
		return nil, utils.Errorf("query mcp_tool_configs failed: %s", err)
	}
	cfg = &schema.MCPToolConfig{
		ToolName:    toolName,
		Source:      source,
		ServerName:  serverName,
		Enable:      true,
		Description: description,
	}
	if err := db.Create(cfg).Error; err != nil {
		return nil, utils.Errorf("create mcp_tool_config failed: %s", err)
	}
	return cfg, nil
}

// UpsertMCPToolConfigDescription updates the cached description for an existing
// tool row, or creates the row if it doesn't exist yet.
func UpsertMCPToolConfigDescription(db *gorm.DB, toolName, source, serverName, description string) error {
	// Use UpdateColumn so gorm v1 does not skip empty-string description values.
	result := db.Model(&schema.MCPToolConfig{}).
		Where("tool_name = ?", toolName).
		UpdateColumn("description", description)
	if result.Error != nil {
		return utils.Errorf("update mcp tool description failed: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		_, err := GetOrCreateMCPToolConfig(db, toolName, source, serverName, description)
		return err
	}
	return nil
}

// SetMCPToolEnabled updates the enable flag for the specified tool name.
// If no row exists yet it is created first (source/serverName are left empty
// — callers that need accurate metadata should upsert via GetOrCreateMCPToolConfig).
func SetMCPToolEnabled(db *gorm.DB, toolName string, enable bool) error {
	// gorm v1 skips zero-value fields in Update(struct), so use UpdateColumn to
	// guarantee the bool is written even when enable=false.
	result := db.Model(&schema.MCPToolConfig{}).
		Where("tool_name = ?", toolName).
		UpdateColumn("enable", enable)
	if result.Error != nil {
		return utils.Errorf("update mcp tool enabled failed: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		// Row does not exist yet — create with explicit enable value.
		// Use Exec to bypass gorm v1's zero-value skipping for bool fields.
		if err := db.Exec(
			"INSERT INTO mcp_tool_configs (tool_name, enable, source, server_name, description, created_at, updated_at) VALUES (?, ?, '', '', '', datetime('now'), datetime('now'))",
			toolName, enable,
		).Error; err != nil {
			return utils.Errorf("create mcp_tool_config failed: %s", err)
		}
	}
	return nil
}

// DeleteMCPToolConfigsByServerAndNames removes tool rows for the given server
// whose canonical names are NOT in the keepNames set. This is used during
// reconciliation to prune tools that an external MCP server no longer provides.
func DeleteMCPToolConfigsByServerAndNames(db *gorm.DB, serverName string, keepNames map[string]struct{}) error {
	if len(keepNames) == 0 {
		// Remove all bridge tools for this server.
		return db.Where("source = ? AND server_name = ?", schema.MCPToolSourceBridge, serverName).
			Delete(&schema.MCPToolConfig{}).Error
	}

	// Fetch all rows for this server, then delete those not in keepNames.
	var existing []*schema.MCPToolConfig
	if err := db.Where("source = ? AND server_name = ?", schema.MCPToolSourceBridge, serverName).
		Find(&existing).Error; err != nil {
		return utils.Errorf("fetch tool configs for server %q: %s", serverName, err)
	}

	var toDelete []uint
	for _, cfg := range existing {
		if _, ok := keepNames[cfg.ToolName]; !ok {
			toDelete = append(toDelete, cfg.ID)
		}
	}
	if len(toDelete) == 0 {
		return nil
	}
	return db.Where("id IN (?)", toDelete).Delete(&schema.MCPToolConfig{}).Error
}

// GetDisabledMCPToolNames returns the set of tool names that are explicitly
// disabled in the configuration table.
func GetDisabledMCPToolNames(db *gorm.DB) (map[string]struct{}, error) {
	var cfgs []*schema.MCPToolConfig
	if err := db.Where("enable = ?", false).Find(&cfgs).Error; err != nil {
		return nil, utils.Errorf("query disabled mcp tools failed: %s", err)
	}
	result := make(map[string]struct{}, len(cfgs))
	for _, c := range cfgs {
		result[c.ToolName] = struct{}{}
	}
	return result, nil
}

// GetMCPToolConfigByName returns the MCPToolConfig row for the given tool name.
// Returns an error if the row does not exist.
func GetMCPToolConfigByName(db *gorm.DB, toolName string) (*schema.MCPToolConfig, error) {
	cfg := &schema.MCPToolConfig{}
	err := db.Where("tool_name = ?", toolName).First(cfg).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, utils.Errorf("mcp tool %q not found", toolName)
		}
		return nil, utils.Errorf("query mcp_tool_config %q failed: %s", toolName, err)
	}
	return cfg, nil
}

// QueryMCPToolConfigs returns a paginated list of MCPToolConfig rows filtered by
// the provided request parameters.
func QueryMCPToolConfigs(db *gorm.DB, req *ypb.GetMCPToolListRequest) (*bizhelper.Paginator, []*schema.MCPToolConfig, error) {
	db = db.Model(&schema.MCPToolConfig{})

	if req.GetSource() != "" {
		db = db.Where("source = ?", req.GetSource())
	}
	if req.GetServerName() != "" {
		db = db.Where("server_name = ?", req.GetServerName())
	}
	if req.GetOnlyEnabled() {
		db = db.Where("enable = ?", true)
	}
	if req.GetKeyword() != "" {
		db = bizhelper.FuzzSearchEx(db, []string{"tool_name", "description"}, req.GetKeyword(), false)
	}

	page := int(req.GetPagination().GetPage())
	if page <= 0 {
		page = 1
	}
	limit := int(req.GetPagination().GetLimit())
	if limit <= 0 {
		limit = 50
	}

	db = bizhelper.OrderByPaging(db, req.GetPagination())

	var cfgs []*schema.MCPToolConfig
	p, db := bizhelper.Paging(db, page, limit, &cfgs)
	if db.Error != nil {
		return nil, nil, utils.Errorf("query mcp_tool_configs failed: %s", db.Error)
	}

	return p, cfgs, nil
}
