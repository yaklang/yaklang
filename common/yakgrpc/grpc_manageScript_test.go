package yakgrpc

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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

func TestImportYakScript(t *testing.T) {
	test := assert.New(t)

	client, err := NewLocalClient()
	if err != nil {
		test.FailNow(err.Error())
	}
	s, err := client.ImportYakScript(context.Background(), &ypb.ImportYakScriptRequest{Dirs: []string{"/Users/limin/Downloads/yak_script"}})
	if err != nil {
		test.FailNow(err.Error())
	}
	_ = s
}

func TestServer_QueryYakScript(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	script, err := client.SaveNewYakScript(context.Background(),
		&ypb.SaveNewYakScriptRequest{
			Params: []*ypb.YakScriptParam{{
				Field:        "target",
				DefaultValue: "1",
				TypeVerbose:  "text",
				FieldVerbose: "",
				Help:         "",
				Required:     true,
				Group:        "",
				ExtraSetting: "",
				MethodType:   "",
			}},
			Type: "mitm",
			Content: `target = cli.String("target")
cli.check()


mirrorNewWebsitePathParams = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
    dump(target)
    yakit_output(target)
    poc.Get(target)~
}
`,
			ScriptName: "query_plugins",
		})
	if err != nil {
		panic(err)
	}
	id, err := client.GetYakScriptById(context.Background(), &ypb.GetYakScriptByIdRequest{Id: script.Id})
	if err != nil {
		panic(err)
	}
	client.DeleteYakScript(context.Background(), &ypb.DeleteYakScriptRequest{
		Id: script.Id,
	})
	assert.True(t, len(id.Params) == 1)
}

func TestExportLocalYakScriptStream(t *testing.T) {
	test := assert.New(t)

	client, err := NewLocalClient()
	if err != nil {
		test.FailNow(err.Error())
	}
	s, err := client.ExportLocalYakScriptStream(context.Background(), &ypb.ExportLocalYakScriptRequest{
		OutputDir:       "/Users/limin/Downloads/",
		OutputPluginDir: "",
		YakScriptIds:    nil,
		Keywords:        "",
		Type:            "",
		UserName:        "",
		Tags:            "",
	})
	if err != nil {
		test.FailNow(err.Error())
	}
	_ = s
}

func TestTempYakScriptQuery(t *testing.T) {
	scriptName, clearFunc, err := yakit.CreateTemporaryYakScriptEx("yak", "")
	require.NoError(t, err)
	defer clearFunc()

	client, err := NewLocalClient()
	require.NoError(t, err)

	res, err := client.QueryYakScript(context.Background(), &ypb.QueryYakScriptRequest{
		Keyword:  scriptName,
		IsIgnore: true,
	})
	require.NoError(t, err)

	require.Lenf(t, res.Data, 1, "just keyword query err, len(res)[%d] != 1", len(res.Data))

	res, err = client.QueryYakScript(context.Background(), &ypb.QueryYakScriptRequest{
		Keyword: scriptName,
	})
	require.NoError(t, err)
	require.Lenf(t, res.Data, 0, "ignore is ineffective, len(res)[%d] != 1", len(res.Data))
}
