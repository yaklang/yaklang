package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

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

	return db.Model(&schema.MCPServer{}).Create(server).Error
}

// UpdateMCPServer 更新MCP服务器
func UpdateMCPServer(db *gorm.DB, id int64, server *schema.MCPServer) error {
	if db == nil {
		return utils.Errorf("database connection is nil")
	}
	if server == nil {
		return utils.Errorf("mcp server is nil")
	}
	copyServer := *server
	copyServer.ID = uint(id)
	return db.Model(&schema.MCPServer{}).Save(&copyServer).Error
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
