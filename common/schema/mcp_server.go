package schema

import (
	"fmt"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// MCPServer MCP服务器配置模型
type MCPServer struct {
	gorm.Model

	Name    string       `gorm:"index;not null" json:"name"` // 服务器名称
	Type    string       `gorm:"index;not null" json:"type"` // 服务器类型 (stdio/sse/streamable_http)
	URL     string       `gorm:"type:text" json:"url"`       // 服务器URL (for sse/streamable_http type)
	Command string       `gorm:"type:text" json:"command"`   // 启动命令 (for stdio type)
	Enable  bool         `gorm:"default:true" json:"enable"` // 是否启用
	Envs    MapStringAny `gorm:"type:text" json:"env"`       // 环境变量
	Headers MapStringAny `gorm:"type:text" json:"headers"`   // 自定义请求头
}

func (m *MCPServer) TableName() string {
	return "mcp_servers"
}

// ToGRPC 转换为gRPC响应格式
func (m *MCPServer) ToGRPC() *ypb.MCPServer {
	envs := make([]*ypb.KVPair, 0, len(m.Envs))
	for k, v := range m.Envs {
		envs = append(envs, &ypb.KVPair{Key: k, Value: fmt.Sprintf("%v", v)})
	}
	headers := make([]*ypb.KVPair, 0, len(m.Headers))
	for k, v := range m.Headers {
		headers = append(headers, &ypb.KVPair{Key: k, Value: fmt.Sprintf("%v", v)})
	}
	return &ypb.MCPServer{
		ID:      int64(m.ID),
		Name:    m.Name,
		Type:    m.Type,
		URL:     m.URL,
		Command: m.Command,
		Enable:  m.Enable,
		Tools:   []*ypb.MCPServerTool{}, // 工具列表将在需要时动态获取
		Envs:    envs,
		Headers: headers,
	}
}

// ToGRPCWithTools 转换为包含工具列表的gRPC响应格式
func (m *MCPServer) ToGRPCWithTools(tools []*ypb.MCPServerTool) *ypb.MCPServer {
	envs := make([]*ypb.KVPair, 0, len(m.Envs))
	for k, v := range m.Envs {
		envs = append(envs, &ypb.KVPair{Key: k, Value: fmt.Sprintf("%v", v)})
	}
	headers := make([]*ypb.KVPair, 0, len(m.Headers))
	for k, v := range m.Headers {
		headers = append(headers, &ypb.KVPair{Key: k, Value: fmt.Sprintf("%v", v)})
	}
	return &ypb.MCPServer{
		ID:      int64(m.ID),
		Name:    m.Name,
		Type:    m.Type,
		URL:     m.URL,
		Command: m.Command,
		Enable:  m.Enable,
		Tools:   tools,
		Envs:    envs,
		Headers: headers,
	}
}

func init() {
	// 注册MCP服务器表到Profile数据库
	RegisterDatabaseSchema(KEY_SCHEMA_PROFILE_DATABASE, &MCPServer{})
}
