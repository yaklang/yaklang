package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
)

type inspectAndSaveClient struct {
	YakClientInterface

	inspectCalls   int
	inspectRequest *ypb.YaklangInspectInformationRequest
	inspectErr     error
	inspectRsp     *ypb.YaklangInspectInformationResponse

	saveCalls   int
	saveRequest *ypb.SaveNewYakScriptRequest
}

func (c *inspectAndSaveClient) StaticAnalyzeError(
	context.Context, *ypb.StaticAnalyzeErrorRequest, ...grpc.CallOption,
) (*ypb.StaticAnalyzeErrorResponse, error) {
	return &ypb.StaticAnalyzeErrorResponse{}, nil
}

func (c *inspectAndSaveClient) YaklangInspectInformation(
	_ context.Context, req *ypb.YaklangInspectInformationRequest, _ ...grpc.CallOption,
) (*ypb.YaklangInspectInformationResponse, error) {
	c.inspectCalls++
	c.inspectRequest = req
	if c.inspectErr != nil {
		return nil, c.inspectErr
	}
	return c.inspectRsp, nil
}

func (c *inspectAndSaveClient) SaveNewYakScript(
	_ context.Context, req *ypb.SaveNewYakScriptRequest, _ ...grpc.CallOption,
) (*ypb.YakScript, error) {
	c.saveCalls++
	c.saveRequest = req
	return &ypb.YakScript{
		Id:         1,
		ScriptName: req.GetScriptName(),
		Type:       req.GetType(),
		Content:    req.GetContent(),
		Params:     req.GetParams(),
	}, nil
}

func newSaveYakScriptTestClient(rsp *ypb.YaklangInspectInformationResponse) *inspectAndSaveClient {
	return &inspectAndSaveClient{inspectRsp: rsp}
}

func saveYakScript(t *testing.T, client *inspectAndSaveClient, args map[string]any) (*mcp.CallToolResult, error) {
	t.Helper()
	srv := &MCPServer{grpcClient: client}
	request := mcp.CallToolRequest{}
	request.Params.Arguments = args
	return handleSaveYakScript(srv)(context.Background(), request)
}

// Yakit 表单由插件保存时解析出的 CLI 参数渲染，MCP 保存插件时必须和编辑器一样调用
// YaklangInspectInformation，否则保存出来的插件没有表单。
func TestSaveYakScriptParsesInspectInformation(t *testing.T) {
	code := "target = cli.String(\"target\")\ncli.check()\n"
	client := newSaveYakScriptTestClient(&ypb.YaklangInspectInformationResponse{
		CliParameter: []*ypb.YakScriptParam{{
			Field:        "target",
			TypeVerbose:  "string",
			DefaultValue: "http://example.com",
		}},
		PluginEnvKey: []string{"FOO_ENV"},
		Tags:         []string{"mitm", "mcp"},
		RiskInfo:     []*ypb.YakRiskInfo{{Level: "low", CVE: "CVE-2026-0001"}},
	})

	result, err := saveYakScript(t, client, map[string]any{
		"scriptName": "mcp-save-form",
		"type":       "mitm",
		"content":    code,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, 1, client.inspectCalls, "插件信息必须被解析")
	require.Equal(t, "mitm", client.inspectRequest.GetYakScriptType())
	require.Equal(t, code, client.inspectRequest.GetYakScriptCode())

	require.Equal(t, 1, client.saveCalls)
	require.Len(t, client.saveRequest.GetParams(), 1)
	require.Equal(t, "target", client.saveRequest.GetParams()[0].GetField())
	require.Equal(t, []string{"FOO_ENV"}, client.saveRequest.GetPluginEnvKey())
	require.Len(t, client.saveRequest.GetRiskInfo(), 1)
	require.Equal(t, "CVE-2026-0001", client.saveRequest.GetRiskInfo()[0].GetCVE())
	require.Equal(t, "mitm,mcp", client.saveRequest.GetTags())
}

func TestSaveYakScriptKeepsExplicitInformation(t *testing.T) {
	client := newSaveYakScriptTestClient(&ypb.YaklangInspectInformationResponse{
		CliParameter: []*ypb.YakScriptParam{{Field: "parsed"}},
		Tags:         []string{"parsed"},
	})

	_, err := saveYakScript(t, client, map[string]any{
		"scriptName": "mcp-save-explicit",
		"type":       "yak",
		"content":    `println("explicit")`,
		"tags":       "custom,keep",
		"params": []any{
			map[string]any{"Field": "explicit", "TypeVerbose": "string"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, client.inspectCalls)

	require.Len(t, client.saveRequest.GetParams(), 1)
	require.Equal(t, "explicit", client.saveRequest.GetParams()[0].GetField())
	require.Equal(t, "custom,keep", client.saveRequest.GetTags())
}

func TestSaveYakScriptSkipsInspectForNonYaklangSource(t *testing.T) {
	client := newSaveYakScriptTestClient(&ypb.YaklangInspectInformationResponse{
		CliParameter: []*ypb.YakScriptParam{{Field: "should-not-be-used"}},
	})

	_, err := saveYakScript(t, client, map[string]any{
		"scriptName": "mcp-save-nuclei",
		"type":       "nuclei",
		"content":    "id: mcp-save-nuclei\ninfo:\n  name: test\n",
	})
	require.NoError(t, err)
	require.Equal(t, 0, client.inspectCalls, "nuclei yaml 不做 yaklang 解析")
	require.Equal(t, 1, client.saveCalls)
	require.Empty(t, client.saveRequest.GetParams())
}

func TestSaveYakScriptInspectFailureRefusesSave(t *testing.T) {
	client := newSaveYakScriptTestClient(nil)
	client.inspectErr = errors.New("ssa parse error")

	_, err := saveYakScript(t, client, map[string]any{
		"scriptName": "mcp-save-bad-parse",
		"type":       "yak",
		"content":    `println("bad")`,
	})
	require.ErrorContains(t, err, "failed to inspect yak script information")
	require.Equal(t, 0, client.saveCalls)
}
