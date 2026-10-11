package reactloops

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCapabilityUsageGuidesMatchExecutionEntries(t *testing.T) {
	markdown := BuildCapabilityEnrichmentMarkdown([]CapabilityDetail{
		{CapabilityName: "read_file", CapabilityType: "tool"},
		{CapabilityName: "mcp_server_read", CapabilityType: "mcp-tool"},
		{CapabilityName: "smart_qa", CapabilityType: "focus_mode"},
	}, nil)
	require.Contains(t, markdown, "已广告工具专用 action")
	require.Contains(t, markdown, "`require_tool` 仅加载定义")
	require.Contains(t, markdown, "`directly_call_tool` 执行")
	require.Contains(t, markdown, "capability_identifier")
	require.NotContains(t, markdown, "enter_focus_mode")
	require.NotContains(t, markdown, "Use `require_tool` to invoke")
}
