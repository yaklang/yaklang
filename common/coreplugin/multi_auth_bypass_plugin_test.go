package coreplugin_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/coreplugin"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/information"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func inspectCorePluginParams(t *testing.T, pluginName, pluginType string) (string, map[string]*ypb.YakScriptParam) {
	t.Helper()

	codeBytes := coreplugin.GetCorePluginData(pluginName)
	require.NotNil(t, codeBytes)

	client, err := yakgrpc.NewLocalClient()
	require.NoError(t, err)

	rsp, err := client.YaklangInspectInformation(context.Background(), &ypb.YaklangInspectInformationRequest{
		YakScriptType: pluginType,
		YakScriptCode: string(codeBytes),
	})
	require.NoError(t, err)
	require.NotNil(t, rsp)

	params := make(map[string]*ypb.YakScriptParam)
	for _, param := range rsp.GetCliParameter() {
		params[param.Field] = param
	}
	return string(codeBytes), params
}

func decodeSelectExtra(t *testing.T, raw string) *information.PluginParamSelect {
	t.Helper()

	var extra information.PluginParamSelect
	require.NoError(t, json.Unmarshal([]byte(raw), &extra))
	return &extra
}

func TestMultiAuthBypassPlugin_FilterParamsSupportMultiValue(t *testing.T) {
	_, params := inspectCorePluginParams(t, "多认证综合越权测试", "mitm")

	checkMultiValueFilter := func(field string) {
		t.Helper()

		param, ok := params[field]
		require.Truef(t, ok, "missing cli param: %s", field)
		require.Equal(t, "select", param.TypeVerbose)
		require.Equal(t, "select", param.MethodType)
		require.Equal(t, "高级（可选参数）", param.Group)

		extra := decodeSelectExtra(t, param.ExtraSetting)
		require.True(t, extra.Double)
		require.Empty(t, extra.Data)
	}

	for _, field := range []string{
		"disable-domain",
		"disable-path",
		"disable-suffix",
		"enable-domain",
		"enable-path",
	} {
		checkMultiValueFilter(field)
	}

	require.Contains(t, params["disable-domain"].Help, "支持多条输入")
	require.Contains(t, params["disable-path"].Help, "支持多条输入")
	require.Contains(t, params["disable-suffix"].Help, ".pdf、.zip")
	require.Contains(t, params["enable-domain"].Help, "支持多条输入")
	require.Contains(t, params["enable-path"].Help, "支持多条输入")
}

func TestMultiAuthBypassPlugin_CompatibilityAndCleanup(t *testing.T) {
	code, params := inspectCorePluginParams(t, "多认证综合越权测试", "mitm")

	require.NotContains(t, code, "dump(replaceResults)")

	kvParam, ok := params["kv"]
	require.True(t, ok)
	require.Equal(t, "json", kvParam.TypeVerbose)
	require.Equal(t, "json", kvParam.MethodType)
	require.True(t, kvParam.Required)
	require.Contains(t, kvParam.Help, "配置参与测试的认证信息")
	require.Contains(t, kvParam.JsonSchema, "支持多行")

	enableUnauth, ok := params["enable-unauth"]
	require.True(t, ok)
	require.Equal(t, "boolean", enableUnauth.TypeVerbose)
	require.Equal(t, "boolean", enableUnauth.MethodType)
	require.True(t, enableUnauth.Required)
	require.Contains(t, enableUnauth.Help, "未授权访问检测")

	responseContent, ok := params["enable-response-content"]
	require.True(t, ok)
	require.Equal(t, "text", responseContent.TypeVerbose)
	require.Equal(t, "text", responseContent.MethodType)
	require.Contains(t, responseContent.Help, "按行输入")

	onlyBody, ok := params["only-body"]
	require.True(t, ok)
	require.Equal(t, "boolean", onlyBody.TypeVerbose)
	require.Equal(t, "高级（可选参数）", onlyBody.Group)
	require.Contains(t, onlyBody.Help, "仅比较响应体")

	disableTypes, ok := params["disable-types"]
	require.True(t, ok)
	extra := decodeSelectExtra(t, disableTypes.ExtraSetting)
	require.True(t, extra.Double)
	require.Len(t, extra.Data, 4)
}
