package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// MCPClientToolSource identifies which category a tool belongs to.
const (
	// MCPClientToolSourceBuiltin means the tool is registered via Go init() in common/mcp.
	MCPClientToolSourceBuiltin = "builtin"
	// MCPClientToolSourceBridge means the tool originates from an external MCPServer entry in the
	// DB and was bridged through the aitool layer (name format: mcp_{server}_{tool}).
	MCPClientToolSourceBridge = "bridge"
)

// MCPClientToolConfig stores per-tool enable/disable state for tools exposed by
// Yaklang acting as an MCP server (i.e. tools provided to MCP clients).
// One row per unique tool name, persisted across MCP server restarts.
type MCPClientToolConfig struct {
	gorm.Model

	// ToolName is the canonical MCP tool name, e.g. "port_scan" or "mcp_IDA-MCP_decompile".
	ToolName string `gorm:"uniqueIndex;not null" json:"tool_name"`

	// Source distinguishes builtin tools from bridge tools.
	// Values: MCPClientToolSourceBuiltin / MCPClientToolSourceBridge.
	Source string `gorm:"index;not null" json:"source"`

	// ServerName is non-empty only for bridge tools; it equals the MCPServer.Name
	// from which this tool was loaded.
	ServerName string `gorm:"index" json:"server_name"`

	// Enable controls whether this tool is exposed when the MCP server starts.
	// Default true — new rows created on first encounter are enabled.
	Enable bool `gorm:"default:true" json:"enable"`

	// Description caches the tool's human-readable description so that the list
	// API does not need to dial external servers on every request. Builtin tools
	// do not use this field — their description is always read from the live Go
	// definition. Bridge tools populate this on first discovery.
	Description string `gorm:"type:text" json:"description"`
}

func (m *MCPClientToolConfig) TableName() string {
	return "mcp_client_tool_configs"
}

// ToGRPC converts the model to the gRPC wire format.
func (m *MCPClientToolConfig) ToGRPC() *ypb.MCPToolConfig {
	return &ypb.MCPToolConfig{
		ID:          int64(m.ID),
		ToolName:    m.ToolName,
		Source:      m.Source,
		ServerName:  m.ServerName,
		Enable:      m.Enable,
		Description: m.Description,
	}
}

func init() {
	RegisterDatabaseSchema(KEY_SCHEMA_PROFILE_DATABASE, &MCPClientToolConfig{})
}
