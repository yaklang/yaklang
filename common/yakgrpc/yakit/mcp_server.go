package yakit

import (
	"context"
	"regexp"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	// Register a database patch so that when the profile DB is initialised the
	// FTS5 virtual table and its triggers are created automatically.  This is
	// the same pattern used by consts/yakit.go for other profile-DB patches.
	schema.RegisterDatabasePatch(schema.KEY_SCHEMA_PROFILE_DATABASE, func(db *gorm.DB) {
		if db == nil {
			return
		}
		if !schema.IsSQLite(db) {
			return
		}
		if !db.HasTable((&schema.MCPServerToolConfig{}).TableName()) {
			// Base table not yet created; AutoMigrate will run next, skip for now.
			return
		}
		if err := EnsureMCPServerToolFTS5(db); err != nil {
			log.Warnf("failed to setup mcp_server_tool_configs fts5 index: %v", err)
		}
	})
}

// ── MCP tool FTS5 (BM25) ──────────────────────────────────────────────────────

var mcpToolFTSASCIITermRe = regexp.MustCompile(`[A-Za-z0-9_]{3,}`)
var mcpToolFTSHanTermRe = regexp.MustCompile(`[\p{Han}]{2,}`)

func MCPServerToolConfigFTSTableName() string {
	return (&schema.MCPServerToolConfig{}).TableName() + "_fts"
}

// defaultMCPToolFTS5 mirrors the pattern used by defaultAIYakToolFTS5 in
// ai_yak_tool_fts.go. It uses external-content mode so the FTS index always
// reflects the canonical data in mcp_server_tool_configs.
var defaultMCPToolFTS5 = &bizhelper.SQLiteFTS5Config{
	BaseModel:    &schema.MCPServerToolConfig{},
	FTSTable:     "", // filled at runtime via MCPServerToolConfigFTSTableName()
	Columns:      []string{"tool_name", "description"},
	ContentTable: "mcp_server_tool_configs",
	Tokenize:     "trigram",
}

func mcpToolFTS5Config() *bizhelper.SQLiteFTS5Config {
	cfg := *defaultMCPToolFTS5
	cfg.FTSTable = MCPServerToolConfigFTSTableName()
	return &cfg
}

// EnsureMCPServerToolFTS5 creates (or ensures) the FTS5 virtual table and
// associated triggers for mcp_server_tool_configs. Idempotent and non-fatal
// when FTS5 is not compiled into the SQLite build.
func EnsureMCPServerToolFTS5(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if !schema.IsSQLite(db) {
		return nil
	}
	cfg := mcpToolFTS5Config()
	if err := bizhelper.SQLiteFTS5Setup(db, cfg); err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			return nil
		}
		return err
	}
	return nil
}

// SearchMCPServerToolsBM25 uses SQLite FTS5 BM25 ranking to search the cached
// MCP tool metadata. Only records with enable=true and a non-empty description
// (i.e. the tool was seen on at least one successful connection) are returned.
//
// Falls back to LIKE-based search when FTS5 is unavailable or keyword is short.
func SearchMCPServerToolsBM25(db *gorm.DB, keyword string, limit int) ([]*schema.MCPServerToolConfig, error) {
	if db == nil {
		return nil, utils.Errorf("db is nil")
	}
	if limit <= 0 {
		limit = 10
	}

	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return []*schema.MCPServerToolConfig{}, nil
	}

	matches := expandMCPToolSearchTerms(keyword)
	if len(matches) == 0 {
		matches = []string{keyword}
	}

	baseQ := db.Model(&schema.MCPServerToolConfig{}).
		Where("mcp_server_tool_configs.enable = ? AND mcp_server_tool_configs.description != ?", true, "")

	// Short keyword or no FTS5: fall back to LIKE on both indexed columns.
	maxLen := 0
	for _, m := range matches {
		if len(m) > maxLen {
			maxLen = len(m)
		}
	}
	ftsTable := MCPServerToolConfigFTSTableName()
	if maxLen < 3 || !schema.IsSQLite(db) || !db.HasTable(ftsTable) {
		return likeSearchMCPTools(baseQ, matches, limit)
	}

	cfg := mcpToolFTS5Config()
	results, err := bizhelper.SQLiteFTS5BM25Match[*schema.MCPServerToolConfig](baseQ, cfg, matches, limit, 0)
	if err != nil {
		// BM25 failed (e.g. stale index); fall back to LIKE.
		return likeSearchMCPTools(baseQ, matches, limit)
	}
	return results, nil
}

func likeSearchMCPTools(baseQ *gorm.DB, terms []string, limit int) ([]*schema.MCPServerToolConfig, error) {
	q := baseQ
	for _, term := range terms {
		pat := "%" + term + "%"
		q = q.Where("mcp_server_tool_configs.tool_name LIKE ? OR mcp_server_tool_configs.description LIKE ?", pat, pat)
	}
	var out []*schema.MCPServerToolConfig
	if err := q.Limit(limit).Find(&out).Error; err != nil {
		return nil, utils.Errorf("like search mcp tools failed: %s", err)
	}
	return out, nil
}

// expandMCPToolSearchTerms mirrors the logic of expandAIYakToolSearchTerms:
// ASCII tokens (≥3 chars) and Chinese character n-grams (3–6 chars).
func expandMCPToolSearchTerms(query string) []string {
	seen := make(map[string]struct{})
	var results []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		results = append(results, s)
	}

	if isMCPToolSafeFTS5Term(query) {
		add(query)
	}
	for _, tok := range mcpToolFTSASCIITermRe.FindAllString(query, -1) {
		add(strings.ToLower(tok))
	}
	for _, seq := range mcpToolFTSHanTermRe.FindAllString(query, -1) {
		runes := []rune(seq)
		if len(runes) <= 3 {
			add(seq)
			continue
		}
		for size := 3; size <= 6 && size <= len(runes); size++ {
			for start := 0; start+size <= len(runes); start++ {
				add(string(runes[start : start+size]))
			}
		}
	}
	return results
}

func isMCPToolSafeFTS5Term(term string) bool {
	return !strings.ContainsAny(term, `"'/:()[]{}\\`)
}

// CreateOrUpdateMCPServer 创建或更新MCP服务器（根据name）
func CreateOrUpdateMCPServer(db *gorm.DB, server *schema.MCPServer) error {
	if db == nil {
		return utils.Errorf("database connection is nil")
	}
	if server == nil {
		return utils.Errorf("mcp server is nil")
	}
	if server.Name == "" {
		return utils.Errorf("mcp server name cannot be empty")
	}
	if server.Type == "" {
		return utils.Errorf("mcp server type cannot be empty")
	}

	// 检查名称是否已存在
	var existing schema.MCPServer
	if err := db.Model(&schema.MCPServer{}).Where("name = ?", server.Name).First(&existing).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			// 不存在则创建
			// 保存Enable的原始值，因为Create后可能会被数据库默认值覆盖
			enableValue := server.Enable
			if err := db.Create(server).Error; err != nil {
				return err
			}
			// 如果Enable为false，需要显式更新（因为数据库默认值是true）
			if !enableValue {
				return db.Model(&schema.MCPServer{}).Where("id = ?", server.ID).Update("enable", false).Error
			}
			return nil
		}
		return utils.Errorf("query mcp server failed: %s", err)
	}

	// 存在则更新，使用map避免默认值问题
	updateData := map[string]interface{}{
		"type":    server.Type,
		"url":     server.URL,
		"command": server.Command,
		"enable":  server.Enable,
		"envs":    server.Envs,
		"headers": server.Headers,
	}
	return db.Model(&schema.MCPServer{}).Where("name = ?", server.Name).Updates(updateData).Error
}

// CreateMCPServer 创建MCP服务器
func CreateMCPServer(db *gorm.DB, server *schema.MCPServer) error {
	if db == nil {
		return utils.Errorf("database connection is nil")
	}
	if server == nil {
		return utils.Errorf("mcp server is nil")
	}
	if server.Name == "" {
		return utils.Errorf("mcp server name cannot be empty")
	}
	if server.Type == "" {
		return utils.Errorf("mcp server type cannot be empty")
	}

	// 检查名称是否已存在
	var existing schema.MCPServer
	if err := db.Model(&schema.MCPServer{}).Where("name = ?", server.Name).First(&existing).Error; err == nil {
		return utils.Errorf("mcp server name already exists")
	}

	// 保存Enable的原始值，因为Create后可能会被数据库默认值覆盖
	enableValue := server.Enable

	// 直接创建，GORM会处理所有字段包括Enable
	if err := db.Create(server).Error; err != nil {
		return err
	}

	// 如果Enable为false，需要显式更新（因为数据库默认值是true）
	if !enableValue {
		return db.Model(&schema.MCPServer{}).Where("id = ?", server.ID).Update("enable", false).Error
	}

	return nil
}

// UpdateMCPServer 更新MCP服务器
func UpdateMCPServer(db *gorm.DB, id int64, server *schema.MCPServer) error {
	if db == nil {
		return utils.Errorf("database connection is nil")
	}
	if server == nil {
		return utils.Errorf("mcp server is nil")
	}

	updateData := map[string]interface{}{
		"name":    server.Name,
		"type":    server.Type,
		"url":     server.URL,
		"command": server.Command,
		"enable":  server.Enable,
		"envs":    server.Envs,
		"headers": server.Headers,
	}
	return db.Model(&schema.MCPServer{}).Where("id = ?", id).Updates(updateData).Error
}

// DeleteMCPServer 删除MCP服务器
func DeleteMCPServer(db *gorm.DB, id int64) error {
	if db == nil {
		return utils.Errorf("database connection is nil")
	}

	return db.Model(&schema.MCPServer{}).Where("id = ?", id).Unscoped().Delete(&schema.MCPServer{}).Error
}

// GetMCPServer 根据ID获取MCP服务器
func GetMCPServer(db *gorm.DB, id int64) (*schema.MCPServer, error) {
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}

	var server schema.MCPServer
	if err := db.Model(&schema.MCPServer{}).Where("id = ?", id).First(&server).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, utils.Errorf("mcp server not found")
		}
		return nil, utils.Errorf("query mcp server failed: %s", err)
	}

	return &server, nil
}

func GetMCPServerByName(db *gorm.DB, name string) (*schema.MCPServer, error) {
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}

	var server schema.MCPServer
	if err := db.Model(&schema.MCPServer{}).Where("name = ?", name).First(&server).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, utils.Errorf("mcp server not found")
		}
		return nil, utils.Errorf("query mcp server failed: %s", err)
	}

	return &server, nil
}

// QueryMCPServers 查询MCP服务器列表（支持分页和关键词搜索）
func QueryMCPServers(db *gorm.DB, req *ypb.GetAllMCPServersRequest) (*bizhelper.Paginator, []*schema.MCPServer, error) {
	if db == nil {
		return nil, nil, utils.Errorf("database connection is nil")
	}
	if req == nil {
		return nil, nil, utils.Errorf("request cannot be nil")
	}

	var servers []*schema.MCPServer
	db = db.Model(&schema.MCPServer{})

	// 根据ID过滤
	if req.GetID() > 0 {
		db = db.Model(&schema.MCPServer{}).Where("id = ?", req.GetID())
	}

	// 根据启用状态过滤
	if req.GetIsEnable() {
		db = db.Where("enable = ?", true)
	}

	// 关键词搜索（在name、type、url、command字段中搜索）
	if req.GetKeyword() != "" {
		fields := []string{"name", "type", "url", "command"}
		db = bizhelper.FuzzSearchEx(db, fields, req.GetKeyword(), false)
	}

	// 分页处理
	page := int(req.GetPagination().GetPage())
	if page <= 0 {
		page = 1
	}
	limit := int(req.GetPagination().GetLimit())
	if limit <= 0 {
		limit = 20
	}

	// Order by
	db = bizhelper.OrderByPaging(db, req.GetPagination())

	p, db := bizhelper.Paging(db, page, limit, &servers)
	if db.Error != nil {
		return nil, nil, utils.Errorf("query mcp servers failed: %s", db.Error)
	}

	return p, servers, nil
}

// YieldEnabledMCPServers 生成器函数，用于遍历所有启用的MCP服务器
func YieldEnabledMCPServers(ctx context.Context, db *gorm.DB) chan *schema.MCPServer {
	db = db.Model(&schema.MCPServer{}).Where("enable = ?", true)
	return bizhelper.YieldModel[*schema.MCPServer](ctx, db)
}

// YieldAllMCPServers yields all MCP servers from the database
func YieldAllMCPServers(ctx context.Context, db *gorm.DB) chan *schema.MCPServer {
	db = db.Model(&schema.MCPServer{})
	return bizhelper.YieldModel[*schema.MCPServer](ctx, db)
}

// GetMCPServerToolConfig retrieves the persisted config for a specific tool within a server.
// If no record exists, a default config with Enable=true is returned.
func GetMCPServerToolConfig(db *gorm.DB, serverName, toolName string) (*schema.MCPServerToolConfig, error) {
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}
	var cfg schema.MCPServerToolConfig
	err := db.Model(&schema.MCPServerToolConfig{}).
		Where("server_name = ? AND tool_name = ?", serverName, toolName).
		First(&cfg).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			// No override stored; return default values.
			return &schema.MCPServerToolConfig{
				ServerName: serverName,
				ToolName:   toolName,
				Enable:     true,
			}, nil
		}
		return nil, utils.Errorf("query mcp server tool config failed: %s", err)
	}
	return &cfg, nil
}

// UpsertMCPServerToolConfig creates or updates the user-controlled Enable flag
// for a tool. Metadata fields are left unchanged.
func UpsertMCPServerToolConfig(db *gorm.DB, serverName, toolName string, enable bool) error {
	if db == nil {
		return utils.Errorf("database connection is nil")
	}
	return upsertMCPServerToolConfigFields(db, serverName, toolName, map[string]interface{}{
		"enable": enable,
	})
}

// UpsertMCPServerToolMetadata refreshes the cached metadata (Description, ParamsJSON)
// for a tool after a successful connection to the remote MCP server. The user-controlled
// Enable flag is preserved.
func UpsertMCPServerToolMetadata(db *gorm.DB, serverName, toolName, description, paramsJSON string) error {
	if db == nil {
		return utils.Errorf("database connection is nil")
	}
	return upsertMCPServerToolConfigFields(db, serverName, toolName, map[string]interface{}{
		"description": description,
		"params_json": paramsJSON,
	})
}

// upsertMCPServerToolConfigFields is the shared upsert helper. On insert it sets
// Enable=true as default; on update it only touches the provided fields so that
// the Enable flag and metadata are independently updatable.
func upsertMCPServerToolConfigFields(db *gorm.DB, serverName, toolName string, fields map[string]interface{}) error {
	var existing schema.MCPServerToolConfig
	err := db.Model(&schema.MCPServerToolConfig{}).
		Where("server_name = ? AND tool_name = ?", serverName, toolName).
		First(&existing).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			cfg := &schema.MCPServerToolConfig{
				ServerName: serverName,
				ToolName:   toolName,
				Enable:     true,
			}
			if desc, ok := fields["description"].(string); ok {
				cfg.Description = desc
			}
			if params, ok := fields["params_json"].(string); ok {
				cfg.ParamsJSON = params
			}
			if createErr := db.Create(cfg).Error; createErr != nil {
				return createErr
			}
			// Explicitly re-apply all fields including booleans (GORM skips false on Create).
			return db.Model(&schema.MCPServerToolConfig{}).
				Where("id = ?", cfg.ID).
				Updates(fields).Error
		}
		return utils.Errorf("query mcp server tool config failed: %s", err)
	}
	return db.Model(&schema.MCPServerToolConfig{}).
		Where("server_name = ? AND tool_name = ?", serverName, toolName).
		Updates(fields).Error
}

// BatchGetMCPServerToolConfigs retrieves all persisted tool configs for a given server,
// returned as a map keyed by tool name for O(1) lookup.
func BatchGetMCPServerToolConfigs(db *gorm.DB, serverName string) (map[string]*schema.MCPServerToolConfig, error) {
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}
	var cfgs []*schema.MCPServerToolConfig
	if err := db.Model(&schema.MCPServerToolConfig{}).
		Where("server_name = ?", serverName).
		Find(&cfgs).Error; err != nil {
		return nil, utils.Errorf("batch query mcp server tool configs failed: %s", err)
	}
	result := make(map[string]*schema.MCPServerToolConfig, len(cfgs))
	for _, c := range cfgs {
		result[c.ToolName] = c
	}
	return result, nil
}

// DeleteMCPServerToolConfigs removes all persisted tool configs for a given server,
// typically called when the server itself is deleted.
func DeleteMCPServerToolConfigs(db *gorm.DB, serverName string) error {
	if db == nil {
		return utils.Errorf("database connection is nil")
	}
	return db.Model(&schema.MCPServerToolConfig{}).
		Where("server_name = ?", serverName).
		Unscoped().
		Delete(&schema.MCPServerToolConfig{}).Error
}

// MCPToolEntry carries the live tool data passed to SyncAndCacheMCPServerTools.
type MCPToolEntry struct {
	ToolName    string
	FullName    string // "mcp_{serverName}_{toolName}", generated by the caller
	Description string
	ParamsJSON  string
}

// SyncAndCacheMCPServerTools is the single entry-point for reconciling the local
// DB cache against a freshly-fetched tool list from a remote MCP server. Call it
// once after every successful ListTools response.
//
// Reconciliation rules (keyed on tool_name):
//   - Tool present in liveTools but not in DB  → INSERT with enable=true.
//   - Tool present in both                      → UPDATE description, params_json and full_name only;
//     the user-controlled enable flag is preserved.
//   - Tool present in DB but not in liveTools   → hard-DELETE; no stale rows accumulate.
func SyncAndCacheMCPServerTools(db *gorm.DB, serverName string, liveTools []MCPToolEntry) error {
	if db == nil {
		return utils.Errorf("database connection is nil")
	}

	// Build a lookup map from the live list.
	liveMap := make(map[string]*MCPToolEntry, len(liveTools))
	for i := range liveTools {
		liveMap[liveTools[i].ToolName] = &liveTools[i]
	}

	// Load all existing rows for this server in one query.
	var existing []*schema.MCPServerToolConfig
	if err := db.Model(&schema.MCPServerToolConfig{}).
		Where("server_name = ?", serverName).
		Find(&existing).Error; err != nil {
		return utils.Errorf("query mcp server tool configs failed: %s", err)
	}

	existingMap := make(map[string]*schema.MCPServerToolConfig, len(existing))
	for _, cfg := range existing {
		existingMap[cfg.ToolName] = cfg
	}

	// Delete rows for tools that have disappeared from the remote server.
	for _, cfg := range existing {
		if _, alive := liveMap[cfg.ToolName]; alive {
			continue
		}
		if err := db.Unscoped().Delete(cfg).Error; err != nil {
			return utils.Errorf("delete stale tool config %s/%s failed: %s", serverName, cfg.ToolName, err)
		}
	}

	// Insert new tools or update metadata for existing ones.
	for _, entry := range liveTools {
		if old, exists := existingMap[entry.ToolName]; exists {
			// Only update metadata; preserve the user-controlled enable flag.
			if old.Description == entry.Description && old.ParamsJSON == entry.ParamsJSON && old.FullName == entry.FullName {
				continue
			}
			if err := db.Model(&schema.MCPServerToolConfig{}).
				Where("id = ?", old.ID).
				Updates(map[string]interface{}{
					"description": entry.Description,
					"params_json": entry.ParamsJSON,
					"full_name":   entry.FullName,
				}).Error; err != nil {
				return utils.Errorf("update tool config %s/%s failed: %s", serverName, entry.ToolName, err)
			}
		} else {
			cfg := &schema.MCPServerToolConfig{
				ServerName:  serverName,
				ToolName:    entry.ToolName,
				FullName:    entry.FullName,
				Enable:      true,
				Description: entry.Description,
				ParamsJSON:  entry.ParamsJSON,
			}
			if err := db.Create(cfg).Error; err != nil {
				return utils.Errorf("insert tool config %s/%s failed: %s", serverName, entry.ToolName, err)
			}
		}
	}
	return nil
}

// SyncMCPServerToolMetadata deletes cached rows for tools absent from liveToolNames.
// Prefer SyncAndCacheMCPServerTools which handles the full reconciliation in one call.
func SyncMCPServerToolMetadata(db *gorm.DB, serverName string, liveToolNames []string) error {
	if db == nil {
		return utils.Errorf("database connection is nil")
	}

	liveSet := make(map[string]struct{}, len(liveToolNames))
	for _, n := range liveToolNames {
		liveSet[n] = struct{}{}
	}

	var cfgs []*schema.MCPServerToolConfig
	if err := db.Model(&schema.MCPServerToolConfig{}).
		Where("server_name = ?", serverName).
		Find(&cfgs).Error; err != nil {
		return utils.Errorf("query mcp server tool configs failed: %s", err)
	}

	for _, cfg := range cfgs {
		if _, alive := liveSet[cfg.ToolName]; alive {
			continue
		}
		if err := db.Unscoped().Delete(cfg).Error; err != nil {
			return utils.Errorf("delete stale tool config %s/%s failed: %s", serverName, cfg.ToolName, err)
		}
	}
	return nil
}

// GetAllEnabledMCPServerToolConfigs returns every tool config row where
// enable=true and a cached description exists (i.e. the remote server has been
// contacted at least once). Intended for capability catalog building.
func GetAllEnabledMCPServerToolConfigs(db *gorm.DB) ([]*schema.MCPServerToolConfig, error) {
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}
	var cfgs []*schema.MCPServerToolConfig
	if err := db.Model(&schema.MCPServerToolConfig{}).
		Where("enable = ? AND description != ?", true, "").
		Find(&cfgs).Error; err != nil {
		return nil, utils.Errorf("get all enabled mcp tool configs failed: %s", err)
	}
	return cfgs, nil
}

// GetEnabledMCPServerToolConfigsByServer returns all enabled tool configs for a
// specific server name, used when enumerating all tools of one MCP server.
func GetEnabledMCPServerToolConfigsByServer(db *gorm.DB, serverName string) ([]*schema.MCPServerToolConfig, error) {
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}
	var cfgs []*schema.MCPServerToolConfig
	if err := db.Model(&schema.MCPServerToolConfig{}).
		Where("server_name = ? AND enable = ?", serverName, true).
		Order("tool_name ASC").
		Find(&cfgs).Error; err != nil {
		return nil, utils.Errorf("get mcp server tool configs by server failed: %s", err)
	}
	return cfgs, nil
}

// GetAllMCPServerNames returns distinct server names present in the tool config table.
func GetAllMCPServerNames(db *gorm.DB) ([]string, error) {
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}
	var names []string
	if err := db.Raw("SELECT DISTINCT server_name FROM mcp_server_tool_configs WHERE deleted_at IS NULL").
		Pluck("server_name", &names).Error; err != nil {
		return nil, utils.Errorf("get all mcp server names failed: %s", err)
	}
	return names, nil
}

// GetMCPServerToolConfigByFullName retrieves a single cached tool config by its
// pre-computed full_name ("mcp_{serverName}_{toolName}"). The full_name column
// carries a unique index so this is an O(log n) index lookup, not a table scan.
func GetMCPServerToolConfigByFullName(db *gorm.DB, fullName string) (*schema.MCPServerToolConfig, error) {
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}
	var cfg schema.MCPServerToolConfig
	if err := db.Model(&schema.MCPServerToolConfig{}).
		Where("full_name = ?", fullName).
		First(&cfg).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, utils.Errorf("mcp tool config not found for full name: %s", fullName)
		}
		return nil, utils.Errorf("query mcp tool config by full name failed: %s", err)
	}
	return &cfg, nil
}
