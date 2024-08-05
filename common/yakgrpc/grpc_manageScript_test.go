package yakgrpc

import (
	"context"
	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
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

func TestServer_Cli_YakSript(t *testing.T) {
	type TestCase struct {
		param  bool
		script *schema.YakScript
	}
	check := func(t *testing.T, scriptRequest *ypb.QueryYakScriptRequest, want []string, db *gorm.DB) {
		_, scripts, err := yakit.QueryYakScript(db, scriptRequest)
		require.NoError(t, err)
		var names []string
		for _, script := range scripts {
			names = append(names, script.ScriptName)
		}
		for _, s := range want {
			require.True(t, lo.Contains(names, s))
		}
	}
	t.Run("test", func(t *testing.T) {
		client, err := NewLocalClient()
		require.NoError(t, err)
		_ = client
		createHandler := func(scripts ...*TestCase) {
			for _, script := range scripts {
				err = yakit.CreateOrUpdateYakScript(consts.GetGormProfileDatabase(), 0, script.script)
				require.NoError(t, err)
			}
		}
		testcases := []*TestCase{
			{
				script: &schema.YakScript{
					ScriptName: "test-nuclei-cli",
					Type:       "nuclei",
					Params:     "[{\"Field\":\"scan-url\",\"TypeVerbose\":\"text\",\"FieldVerbose\":\"请输入扫描目标\",\"Required\":true,\"MethodType\":\"text\"},{\"Field\":\"file-path\",\"TypeVerbose\":\"upload-path\",\"FieldVerbose\":\"请输入字典路径\",\"MethodType\":\"file\"}]",
				},
				param: false},
			{script: &schema.YakScript{
				ScriptName: "test-port-scan-cli",
				Type:       "port-scan",
				Params:     "[{\"Field\":\"scan-url\",\"TypeVerbose\":\"text\",\"FieldVerbose\":\"请输入扫描目标\",\"Required\":true,\"MethodType\":\"text\"},{\"Field\":\"file-path\",\"TypeVerbose\":\"upload-path\",\"FieldVerbose\":\"请输入字典路径\",\"MethodType\":\"file\"}]",
			},
				param: false},
			{script: &schema.YakScript{
				ScriptName: "test-mitm-cli",
				Type:       "mitm",
				Params:     "[{\"Field\":\"scan-url\",\"TypeVerbose\":\"text\",\"FieldVerbose\":\"请输入扫描目标\",\"Required\":true,\"MethodType\":\"text\"},{\"Field\":\"file-path\",\"TypeVerbose\":\"upload-path\",\"FieldVerbose\":\"请输入字典路径\",\"MethodType\":\"file\"}]",
			},
				param: true},
			{script: &schema.YakScript{
				ScriptName: "test-mitm-no-cli",
				Type:       "mitm",
				Params:     "",
			},
				param: false}}
		defer func() {
			lo.ForEach(testcases, func(item *TestCase, index int) {
				require.NoError(t, yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), item.script.ScriptName))
			})
		}()
		createHandler(testcases...)

		//filter mitm has cli
		check(t, &ypb.QueryYakScriptRequest{
			Type:               "mitm,port-scan,nuclei",
			IsMITMParamPlugins: 2,
		}, []string{"test-mitm-no-cli", "test-port-scan-cli", "test-nuclei-cli"}, consts.GetGormProfileDatabase())

		check(t, &ypb.QueryYakScriptRequest{
			Type:               "mitm",
			IsMITMParamPlugins: 1,
		},
			[]string{"test-mitm-cli"}, consts.GetGormProfileDatabase())

		check(t, &ypb.QueryYakScriptRequest{
			Type:               "mitm,port-scan,nuclei",
			IsMITMParamPlugins: 0,
		},
			[]string{"test-nuclei-cli", "test-port-scan-cli", "test-mitm-cli", "test-mitm-no-cli"}, consts.GetGormProfileDatabase(),
		)
	})
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
				Required:     true,
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
