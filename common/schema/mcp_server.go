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

// MCPServerToolConfig stores per-tool configuration and cached metadata for MCP tools.
//
// The tool list is fetched dynamically from the remote MCP server at runtime; this table
// serves two purposes:
//  1. User overrides: Enable flag that persists across sessions.
//  2. Metadata cache: Description and ParamsJSON populated on first successful connection,
//     so the tool is discoverable via search even when the server is offline.
//
// Key: (ServerName, ToolName)
type MCPServerToolConfig struct {
	gorm.Model

	ServerName string `gorm:"index;not null" json:"server_name"`     // owning MCP server name
	ToolName   string `gorm:"index;not null" json:"tool_name"`       // tool name as reported by the server
	FullName   string `gorm:"uniqueIndex;not null" json:"full_name"` // "mcp_{server_name}_{tool_name}" for direct lookup
	Enable     bool   `gorm:"default:true" json:"enable"`            // whether the tool is loaded by the AI agent

	// Metadata cached from the remote server on last successful tool list refresh.
	// These fields are never edited by the user; they are overwritten on every refresh.
	Description string `gorm:"type:text" json:"description"` // tool description
	ParamsJSON  string `gorm:"type:text" json:"params_json"` // JSON-encoded input schema (MCPServerToolParamInfo[])
}

func (m *MCPServerToolConfig) TableName() string {
	return "mcp_server_tool_configs"
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
	RegisterDatabaseSchema(KEY_SCHEMA_PROFILE_DATABASE, &MCPServerToolConfig{})
}
