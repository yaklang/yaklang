package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// MCPServer MCP服务器配置模型
type MCPServer struct {
	gorm.Model

	Name    string `gorm:"index;not null" json:"name"` // 服务器名称
	Type    string `gorm:"index;not null" json:"type"` // 服务器类型 (stdio/sse)
	URL     string `gorm:"type:text" json:"url"`       // 服务器URL (for sse type)
	Command string `gorm:"type:text" json:"command"`   // 启动命令 (for stdio type)
	Enable  bool   `gorm:"default:true" json:"enable"` // 是否启用
}

func (m *MCPServer) TableName() string {
	return "mcp_servers"
}

// ToGRPC 转换为gRPC响应格式
func (m *MCPServer) ToGRPC() *ypb.MCPServer {
	return &ypb.MCPServer{
		ID:      int64(m.ID),
		Name:    m.Name,
		Type:    m.Type,
		URL:     m.URL,
		Command: m.Command,
		Enable:  m.Enable,
		Tools:   []*ypb.MCPServerTool{}, // 工具列表将在需要时动态获取
	}
}

// ToGRPCWithTools 转换为包含工具列表的gRPC响应格式
func (m *MCPServer) ToGRPCWithTools(tools []*ypb.MCPServerTool) *ypb.MCPServer {
	return &ypb.MCPServer{
		ID:      int64(m.ID),
		Name:    m.Name,
		Type:    m.Type,
		URL:     m.URL,
		Command: m.Command,
		Enable:  m.Enable,
		Tools:   tools,
	}
}

func init() {
	// 注册MCP服务器表到Profile数据库
	RegisterDatabaseSchema(KEY_SCHEMA_PROFILE_DATABASE, &MCPServer{})
}
