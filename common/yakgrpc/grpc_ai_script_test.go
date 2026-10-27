package yakgrpc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	_ "github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestYakScriptModelGRPC(t *testing.T) {
	config := &ypb.ThirdPartyApplicationConfig{Type: "yak-script", Script: "chat = func(req, emit) { return `{\"@action\":\"call-tool\",\"tool\":\"ai_config_health_check\",\"identifier\":\"check\",\"params\":{\"content\":\"桥接测试\",\"summary\":\"OK\"}}` }"}
	server := new(Server)
	resp, err := server.AIConfigHealthCheck(context.Background(), &ypb.AIConfigHealthCheckRequest{Config: config, Content: "桥接测试"})
	require.NoError(t, err)
	require.True(t, resp.GetSuccess(), resp.GetErrorMessage())
	require.Nil(t, resp.GetRecommendConfig())
	config.Script = `chat = func(req, emit) { return "invalid action" }`
	resp, err = server.AIConfigHealthCheck(context.Background(), &ypb.AIConfigHealthCheckRequest{Config: config, Content: "桥接测试"})
	require.NoError(t, err)
	require.False(t, resp.GetSuccess())
	require.Empty(t, buildRecommendedAIConfigs(config), "script protocols must not be retried with OpenAI URL suffixes")
	request, err := json.Marshal(map[string]string{"Type": "yak-script", "model": "script-model", "script": config.Script})
	require.NoError(t, err)
	models, err := server.ListAiModel(context.Background(), &ypb.ListAiModelRequest{Config: string(request)})
	require.NoError(t, err)
	require.Equal(t, []string{"script-model"}, models.ModelName)

	templates, err := buildAIGatewayTemplates()
	require.NoError(t, err)
	for _, template := range templates {
		if template.Name != "yak-script" {
			continue
		}
		require.Len(t, template.Items, 2, "script providers must not advertise connection or timeout fields")
		require.Equal(t, "enable_native_tool_calling", template.Items[0].Name)
		require.Equal(t, "false", template.Items[0].DefaultValue)
		require.Equal(t, "script", template.Items[1].Name)
		require.True(t, template.Items[1].Required)
		return
	}
	t.Fatal("missing required script configuration field")
}
