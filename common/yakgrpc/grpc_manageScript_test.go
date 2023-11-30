package yakgrpc

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestQueryYakScriptRiskDetailByCWE(t *testing.T) {
	test := assert.New(t)
	client, err := NewLocalClient()
	if err != nil {
		test.FailNow(err.Error())
	}
	_, err = client.QueryYakScriptRiskDetailByCWE(context.Background(), &ypb.QueryYakScriptRiskDetailByCWERequest{CWEId: "502"})
	if err != nil {
		panic(err)
	}
}

func TestYakScriptRiskTypeList(t *testing.T) {
	test := assert.New(t)
	client, err := NewLocalClient()
	if err != nil {
		test.FailNow(err.Error())
	}
	_, err = client.YakScriptRiskTypeList(context.Background(), &ypb.Empty{})

}

func TestSaveNewYakScript(t *testing.T) {
	test := assert.New(t)

	client, err := NewLocalClient()
	if err != nil {
		test.FailNow(err.Error())
	}

	s, err := client.SaveNewYakScript(context.Background(), &ypb.SaveNewYakScriptRequest{
		Content:              "",
		Type:                 "",
		Params:               nil,
		ScriptName:           "测试插件新增接口",
		Help:                 "",
		Level:                "",
		Tags:                 "",
		IsHistory:            false,
		IsIgnore:             false,
		IsGeneralModule:      false,
		GeneralModuleVerbose: "",
		GeneralModuleKey:     "",
		FromGit:              "",
		EnablePluginSelector: false,
		PluginSelectorTypes:  "",
		IsCorePlugin:         false,
		RiskType:             "",
		RiskDetail:           nil,
		RiskAnnotation:       "",
	})
	if err != nil {
		test.FailNow(err.Error())
	}

	_ = s
}
