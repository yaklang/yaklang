package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// MCPToolSource identifies which category a tool belongs to.
const (
	// MCPToolSourceBuiltin means the tool is registered via Go init() in common/mcp.
	MCPToolSourceBuiltin = "builtin"
	// MCPToolSourceBridge means the tool originates from an external MCPServer entry in the
	// DB and was bridged through the aitool layer (name format: mcp_{server}_{tool}).
	MCPToolSourceBridge = "bridge"
)

// MCPToolConfig stores per-tool enable/disable state that is persisted across
// MCP server restarts. One row per unique tool name.
type MCPToolConfig struct {
	gorm.Model

	// ToolName is the canonical MCP tool name, e.g. "port_scan" or "mcp_IDA-MCP_decompile".
	ToolName string `gorm:"uniqueIndex;not null" json:"tool_name"`

	// Source distinguishes builtin tools from bridge tools.
	// Values: MCPToolSourceBuiltin / MCPToolSourceBridge.
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

func (m *MCPToolConfig) TableName() string {
	return "mcp_tool_configs"
}

// ToGRPC converts the model to the gRPC wire format.
func (m *MCPToolConfig) ToGRPC() *ypb.MCPToolConfig {
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
	RegisterDatabaseSchema(KEY_SCHEMA_PROFILE_DATABASE, &MCPToolConfig{})
}
