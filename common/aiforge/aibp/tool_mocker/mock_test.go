package tool_mocker

import (
	"context"
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func init() {
	yakit.LoadGlobalNetworkConfig()
}

func TestGetSuggestions(t *testing.T) {
	mockServer := NewAiToolMockServer(aispec.WithDebugStream(true))
	ctx := context.Background()
	suggestions, err := mockServer.QueryToolSuggestion(ctx, "查询dns信息", aicommon.WithDebugPrompt(true))
	if err != nil {
		t.Fatal(err)
		return
	}
	for _, suggestion := range suggestions {
		fmt.Println("tool name: ", suggestion.Name)
	}

	tool, err := mockServer.SearchTool(ctx, suggestions[0].Name)
	if err != nil {
		t.Fatal(err)
		return
	}
	println(tool.ToJSONSchemaString())
}

func TestCallTool(t *testing.T) {
	mockServer := NewAiToolMockServer(aispec.WithDebugStream(true))
	result, err := mockServer.CallTool(&aitool.Tool{
		Tool: &mcp.Tool{
			Name:        "dns_query",
			Description: "Query all A records for the input domain name.",
		},
	}, map[string]any{
		"domain": "yaklang.com",
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
		return
	}
	spew.Dump(result)
}
