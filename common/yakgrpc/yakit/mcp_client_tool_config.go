package yakit

import (
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	bizhelper "github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// MCPBridgeToolCanonicalName is the tool_name stored in mcp_client_tool_configs for
// bridge tools (same convention as MCPServerToolFullName / aitool loader).
func MCPBridgeToolCanonicalName(serverName, remoteToolName string) string {
	return MCPServerToolFullName(serverName, remoteToolName)
}

// MCPBridgeToolOriginalName reverses MCPBridgeToolCanonicalName for a given server name.
func MCPBridgeToolOriginalName(canonicalName, serverName string) string {
	prefix := "mcp_" + serverName + "_"
	if strings.HasPrefix(canonicalName, prefix) {
		return canonicalName[len(prefix):]
	}
	return ""
}

// GetOrCreateMCPClientToolConfig returns the MCPClientToolConfig row for the given tool,
// creating it (with Enable=true) if it does not yet exist.
// description is stored on creation; existing rows are NOT updated here —
// use UpsertMCPClientToolConfigDescription to refresh the description on an existing row.
func GetOrCreateMCPClientToolConfig(db *gorm.DB, toolName, source, serverName, description string) (*schema.MCPClientToolConfig, error) {
	cfg := &schema.MCPClientToolConfig{}
	err := db.Where("tool_name = ?", toolName).First(cfg).Error
	if err == nil {
		return cfg, nil
	}
	if !gorm.IsRecordNotFoundError(err) {
		return nil, utils.Errorf("query mcp_client_tool_configs failed: %s", err)
	}
	cfg = &schema.MCPClientToolConfig{
		ToolName:    toolName,
		Source:      source,
		ServerName:  serverName,
		Enable:      true,
		Description: description,
	}
	if err := db.Create(cfg).Error; err != nil {
		return nil, utils.Errorf("create mcp_client_tool_config failed: %s", err)
	}
	return cfg, nil
}

// UpsertMCPClientToolConfigDescription updates the cached description for an existing
// tool row, or creates the row if it doesn't exist yet.
func UpsertMCPClientToolConfigDescription(db *gorm.DB, toolName, source, serverName, description string) error {
	// Use UpdateColumn so gorm v1 does not skip empty-string description values.
	result := db.Model(&schema.MCPClientToolConfig{}).
		Where("tool_name = ?", toolName).
		UpdateColumn("description", description)
	if result.Error != nil {
		return utils.Errorf("update mcp client tool description failed: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		_, err := GetOrCreateMCPClientToolConfig(db, toolName, source, serverName, description)
		return err
	}
	return nil
}

// SetMCPClientToolEnabled updates the enable flag for the specified tool name.
// Returns an error if the tool has not been discovered yet (i.e. no row exists).
// Callers must ensure the tool is known via GetOrCreateMCPClientToolConfig or
// GetMCPToolList before toggling its state.
func SetMCPClientToolEnabled(db *gorm.DB, toolName string, enable bool) error {
	// gorm v1 skips zero-value fields in Update(struct), so use UpdateColumn to
	// guarantee the bool is written even when enable=false.
	result := db.Model(&schema.MCPClientToolConfig{}).
		Where("tool_name = ?", toolName).
		UpdateColumn("enable", enable)
	if result.Error != nil {
		return utils.Errorf("update mcp client tool enabled failed: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return utils.Errorf("mcp client tool %q not found; call GetMCPToolList first to discover tools", toolName)
	}
	return nil
}

// DeleteStaleMCPClientBuiltinTools removes builtin tool rows whose tool_name is
// not present in the provided keepNames set. Called after syncing the live
// builtin registry so that renamed/removed tools do not persist in the DB.
func DeleteStaleMCPClientBuiltinTools(db *gorm.DB, keepNames map[string]struct{}) error {
	return deleteStaleMCPClientToolsBySource(db, schema.MCPClientToolSourceBuiltin, keepNames)
}

// DeleteStaleMCPClientAITools removes aitool-framework builtin rows whose tool_name
// is not present in the provided keepNames set.
func DeleteStaleMCPClientAITools(db *gorm.DB, keepNames map[string]struct{}) error {
	return deleteStaleMCPClientToolsBySource(db, schema.MCPClientToolSourceAITool, keepNames)
}

func deleteStaleMCPClientToolsBySource(db *gorm.DB, source string, keepNames map[string]struct{}) error {
	var existing []*schema.MCPClientToolConfig
	if err := db.Where("source = ?", source).Find(&existing).Error; err != nil {
		return utils.Errorf("fetch %s tool configs: %s", source, err)
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
	return db.Where("id IN (?)", toDelete).Unscoped().Delete(&schema.MCPClientToolConfig{}).Error
}

// EnsureMCPClientToolConfigSource updates the source column when a tool migrates
// between registries (e.g. legacy builtin vs aitool-framework builtin).
func EnsureMCPClientToolConfigSource(db *gorm.DB, toolName, source string) error {
	result := db.Model(&schema.MCPClientToolConfig{}).
		Where("tool_name = ? AND source <> ?", toolName, source).
		UpdateColumn("source", source)
	if result.Error != nil {
		return utils.Errorf("update mcp client tool source failed: %s", result.Error)
	}
	return nil
}

// DeleteAllMCPClientBridgeToolConfigsForServer removes every bridge tool row for serverName.
// Used when the upstream MCP server is deleted or its endpoint changes.
func DeleteAllMCPClientBridgeToolConfigsForServer(db *gorm.DB, serverName string) error {
	return DeleteMCPClientToolConfigsByServerAndNames(db, serverName, nil)
}

// MigrateMCPClientBridgeToolConfigsServerName rewrites outbound bridge tool rows when the
// parent MCP server name changes but the remote endpoint is unchanged.
func MigrateMCPClientBridgeToolConfigsServerName(db *gorm.DB, oldServerName, newServerName string) error {
	if db == nil {
		return utils.Errorf("database connection is nil")
	}
	oldServerName = strings.TrimSpace(oldServerName)
	newServerName = strings.TrimSpace(newServerName)
	if oldServerName == "" || newServerName == "" {
		return utils.Errorf("mcp server name cannot be empty")
	}
	if oldServerName == newServerName {
		return nil
	}

	var cfgs []*schema.MCPClientToolConfig
	if err := db.Where("source = ? AND server_name = ?", schema.MCPClientToolSourceBridge, oldServerName).
		Find(&cfgs).Error; err != nil {
		return utils.Errorf("query bridge tool configs for server %q failed: %s", oldServerName, err)
	}

	for _, cfg := range cfgs {
		remoteTool := MCPBridgeToolOriginalName(cfg.ToolName, oldServerName)
		if remoteTool == "" {
			return utils.Errorf(
				"bridge tool %q does not match server %q naming convention",
				cfg.ToolName, oldServerName,
			)
		}
		newToolName := MCPBridgeToolCanonicalName(newServerName, remoteTool)

		var conflict schema.MCPClientToolConfig
		qErr := db.Where("tool_name = ? AND id != ?", newToolName, cfg.ID).First(&conflict).Error
		if qErr == nil {
			return utils.Errorf("bridge tool name %q already exists", newToolName)
		}
		if !gorm.IsRecordNotFoundError(qErr) {
			return utils.Errorf("check bridge tool name conflict failed: %s", qErr)
		}

		if err := db.Model(&schema.MCPClientToolConfig{}).Where("id = ?", cfg.ID).
			Updates(map[string]interface{}{
				"tool_name":   newToolName,
				"server_name": newServerName,
			}).Error; err != nil {
			return utils.Errorf(
				"migrate bridge tool config %s to server %q failed: %s",
				cfg.ToolName, newServerName, err,
			)
		}
	}
	return nil
}

// DeleteMCPClientToolConfigsByServerAndNames removes tool rows for the given server
// whose canonical names are NOT in the keepNames set. This is used during
// reconciliation to prune tools that an external MCP server no longer provides.
func DeleteMCPClientToolConfigsByServerAndNames(db *gorm.DB, serverName string, keepNames map[string]struct{}) error {
	if len(keepNames) == 0 {
		// Remove all bridge tools for this server.
		return db.Where("source = ? AND server_name = ?", schema.MCPClientToolSourceBridge, serverName).
			Unscoped().Delete(&schema.MCPClientToolConfig{}).Error
	}

	// Fetch all rows for this server, then delete those not in keepNames.
	var existing []*schema.MCPClientToolConfig
	if err := db.Where("source = ? AND server_name = ?", schema.MCPClientToolSourceBridge, serverName).
		Find(&existing).Error; err != nil {
		return utils.Errorf("fetch client tool configs for server %q: %s", serverName, err)
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
	return db.Where("id IN (?)", toDelete).Unscoped().Delete(&schema.MCPClientToolConfig{}).Error
}

// GetDisabledMCPClientToolNames returns the set of tool names that are explicitly
// disabled in the configuration table.
func GetDisabledMCPClientToolNames(db *gorm.DB) (map[string]struct{}, error) {
	var cfgs []*schema.MCPClientToolConfig
	if err := db.Where("enable = ?", false).Find(&cfgs).Error; err != nil {
		return nil, utils.Errorf("query disabled mcp client tools failed: %s", err)
	}
	result := make(map[string]struct{}, len(cfgs))
	for _, c := range cfgs {
		result[c.ToolName] = struct{}{}
	}
	return result, nil
}

// GetMCPClientToolConfigByName returns the MCPClientToolConfig row for the given tool name.
// Returns an error if the row does not exist.
func GetMCPClientToolConfigByName(db *gorm.DB, toolName string) (*schema.MCPClientToolConfig, error) {
	cfg := &schema.MCPClientToolConfig{}
	err := db.Where("tool_name = ?", toolName).First(cfg).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, utils.Errorf("mcp client tool %q not found", toolName)
		}
		return nil, utils.Errorf("query mcp_client_tool_config %q failed: %s", toolName, err)
	}
	return cfg, nil
}

// QueryMCPClientToolConfigs returns a paginated list of MCPClientToolConfig rows filtered by
// the provided request parameters.
func QueryMCPClientToolConfigs(db *gorm.DB, req *ypb.GetMCPToolListRequest) (*bizhelper.Paginator, []*schema.MCPClientToolConfig, error) {
	db = db.Model(&schema.MCPClientToolConfig{})

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

	var cfgs []*schema.MCPClientToolConfig
	p, db := bizhelper.Paging(db, page, limit, &cfgs)
	if db.Error != nil {
		return nil, nil, utils.Errorf("query mcp_client_tool_configs failed: %s", db.Error)
	}

	return p, cfgs, nil
}
