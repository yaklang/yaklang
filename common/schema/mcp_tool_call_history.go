package schema

import "github.com/yaklang/gorm"

// MCPToolCallHistory stores calls made by external MCP clients to Yaklang tools.
type MCPToolCallHistory struct {
	gorm.Model

	ToolName       string `gorm:"index;not null" json:"tool_name"`
	Arguments      string `gorm:"type:text" json:"arguments"`
	Result         string `gorm:"type:text" json:"result"`
	Success        bool   `gorm:"index" json:"success"`
	ErrorMessage   string `gorm:"type:text" json:"error_message"`
	DurationMillis int64  `json:"duration_millis"`
	ClientID       string `gorm:"index" json:"client_id"`
	SessionID      string `gorm:"index" json:"session_id"`
	ClientName     string `gorm:"index" json:"client_name"`
	ClientVersion  string `json:"client_version"`
}

func (m *MCPToolCallHistory) TableName() string {
	return "mcp_tool_call_histories"
}

func init() {
	RegisterDatabaseSchema(KEY_SCHEMA_PROFILE_DATABASE, &MCPToolCallHistory{})
}
