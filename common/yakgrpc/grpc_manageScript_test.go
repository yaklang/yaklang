package yakgrpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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
				err = yakit.CreateOrUpdateYakScript(consts.GetGormProfileDatabase().Debug(), 0, script.script)
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

func TestServer_QueryYakSript_ByImportance(t *testing.T) {
	type TestCase struct {
		param  bool
		script *schema.YakScript
	}

	checkByOrder := func(t *testing.T, scriptRequest *ypb.QueryYakScriptRequest, db *gorm.DB) {
		_, scripts, err := yakit.QueryYakScript(db, scriptRequest)
		require.NoError(t, err)
		for _, s := range scripts {
			log.Infof("scripts:%s", s.ScriptName)
		}
		for i := 0; i < len(scripts)-1; i++ {
			if !scripts[i].IsCorePlugin && scripts[i+1].IsCorePlugin {
				t.Fatalf("test failed: %s is not corePlugin,but its next plugin %s is  corePlugin", scripts[i].ScriptName, scripts[i+1].ScriptName)
			}
			if !scripts[i].IsCorePlugin && !scripts[i+1].IsCorePlugin {
				if !scripts[i].OnlineOfficial && scripts[i+1].OnlineOfficial {
					t.Fatalf("test failed: %s is not onlineOfficial,but its next plugin %s is onlineOfficial", scripts[i].ScriptName, scripts[i+1].ScriptName)
				}
			}
		}
	}

	createScript := func(scripts ...*TestCase) {
		for _, script := range scripts {
			err := yakit.CreateOrUpdateYakScript(consts.GetGormProfileDatabase(), 0, script.script)
			require.NoError(t, err)
		}
	}

	testcases := []*TestCase{
		{
			script: &schema.YakScript{
				ScriptName:   "test-script-1",
				Type:         "nuclei",
				Params:       "[{\"Field\":\"scan-url\",\"TypeVerbose\":\"text\",\"FieldVerbose\":\"请输入扫描目标\",\"Required\":true,\"MethodType\":\"text\"},{\"Field\":\"file-path\",\"TypeVerbose\":\"upload-path\",\"FieldVerbose\":\"请输入字典路径\",\"MethodType\":\"file\"}]",
				IsCorePlugin: true,
			},
			param: false},
		{script: &schema.YakScript{
			ScriptName:     "test-script-2",
			Type:           "port-scan",
			Params:         "[{\"Field\":\"scan-url\",\"TypeVerbose\":\"text\",\"FieldVerbose\":\"请输入扫描目标\",\"Required\":true,\"MethodType\":\"text\"},{\"Field\":\"file-path\",\"TypeVerbose\":\"upload-path\",\"FieldVerbose\":\"请输入字典路径\",\"MethodType\":\"file\"}]",
			OnlineOfficial: true,
		},
			param: false},
		{script: &schema.YakScript{
			ScriptName: "test-script-3",
			Type:       "mitm",
			Params:     "",
		},
			param: false}}

	createScript(testcases...)
	defer func() {
		lo.ForEach(testcases, func(item *TestCase, index int) {
			require.NoError(t, yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), item.script.ScriptName))
		})
	}()

	client, err := NewLocalClient()
	require.NoError(t, err)
	_ = client
	checkByOrder(t, &ypb.QueryYakScriptRequest{
		Pagination: &ypb.Paging{
			Page:     1,
			Limit:    30,
			OrderBy:  "",
			Order:    "",
			RawOrder: "is_core_plugin desc,online_official desc",
		},
	}, consts.GetGormProfileDatabase())

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

func TestQueryYakScript(t *testing.T) {
	type TestCase struct {
		script *schema.YakScript
	}
	createScript := func(scripts ...*TestCase) {
		for _, script := range scripts {
			err := yakit.CreateOrUpdateYakScript(consts.GetGormProfileDatabase(), 0, script.script)
			require.NoError(t, err)
		}
	}

	testcases := []*TestCase{
		{
			script: &schema.YakScript{
				ScriptName: "fileKeywords-test-script-1",
				Type:       "yak",
				Content:    "yakit.AutoInitYakit()\n\n# Input your code!\n\n// 测试",
			},
		},
		{script: &schema.YakScript{
			ScriptName: "fileKeywords-script-2",
			Type:       "yak",
			Content:    "yakit.AutoInitYakit()\n\n# Input your code!\n\n// fileKeywords-测试-2",
		},
		},
		{script: &schema.YakScript{
			ScriptName: "fileKeywords-test-3",
			Type:       "yak",
			Content:    "yakit.AutoInitYakit()\n\n# Input your code!\n\n// -fileKeywords-script-3",
		},
		}}

	createScript(testcases...)
	defer func() {
		lo.ForEach(testcases, func(item *TestCase, index int) {
			require.NoError(t, yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), item.script.ScriptName))
		})
	}()

	tests := []struct {
		filedKeywords string
		count         int
	}{
		{
			filedKeywords: "fileKeywords-test",
			count:         2,
		},
		{
			filedKeywords: "fileKeywords-script",
			count:         1,
		},
		{
			filedKeywords: "",
			count:         3,
		},
	}
	for _, tc := range tests {
		t.Run(tc.filedKeywords, func(t *testing.T) {
			var count int
			db := consts.GetGormProfileDatabase().Model(&schema.YakScript{})
			db = yakit.FilterYakScript(db, &ypb.QueryYakScriptRequest{
				FieldKeywords: tc.filedKeywords,
			})
			db.Count(&count)
			if tc.filedKeywords == "" {
				if count < tc.count {
					t.Errorf("yakScript  not found for filedKeywords=%s", tc.filedKeywords)
				}
			} else if tc.count != count {
				t.Errorf("yakScript  not found for filedKeywords=%s", tc.filedKeywords)
			}
		})

	}
}

func TestYakScriptDelete(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	create := func(corePlugin bool) int64 {
		name := uuid.NewString()
		err := yakit.CreateOrUpdateYakScript(consts.GetGormProfileDatabase(), 0, &schema.YakScript{
			Type:         "mitm",
			Content:      `target = cli.String("target")`,
			ScriptName:   name,
			IsCorePlugin: corePlugin,
		})
		require.NoError(t, err)
		script, err := yakit.GetYakScriptByName(consts.GetGormProfileDatabase(), name)
		require.NoError(t, err)
		return int64(script.ID)
	}

	delete := func(ids ...int64) {
		req := &ypb.DeleteYakScriptRequest{}
		if len(ids) == 1 {
			req.Id = ids[0]
		} else {
			req.Ids = ids
		}
		_, err := client.DeleteYakScript(context.Background(), req)
		require.NoError(t, err)
	}

	query := func(id int64) (*ypb.YakScript, error) {
		return client.GetYakScriptById(context.Background(), &ypb.GetYakScriptByIdRequest{Id: id})
	}

	t.Run("test query", func(t *testing.T) {
		id := create(false)
		script, err := query(id)
		require.NoError(t, err)
		require.NotNil(t, script)
	})

	t.Run("test delete", func(t *testing.T) {
		id := create(false)
		log.Infof("id:%d", id)
		delete(id)
		_, err := query(id)
		require.Error(t, err)
	})

	t.Run("test delete multiple", func(t *testing.T) {
		id1 := create(false)
		id2 := create(false)
		delete(id1, id2)
		_, err := query(id1)
		require.Error(t, err)
		_, err = query(id2)
		require.Error(t, err)
	})

	t.Run("con't delete core plugin", func(t *testing.T) {
		id1 := create(false)
		id2 := create(true)
		delete(id1, id2)
		_, err := query(id1)
		require.Error(t, err)
		script2, err := query(id2)
		require.NoError(t, err)
		require.NotNil(t, script2)
	})

}

func TestYakScriptSkipUpdate(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	type TestCase struct {
		script *schema.YakScript
	}

	createScript := func(scripts ...*TestCase) {
		for _, script := range scripts {
			err := yakit.CreateOrUpdateYakScript(consts.GetGormProfileDatabase(), 0, script.script)
			require.NoError(t, err)
		}
	}

	deleteScript := func(scripts ...*TestCase) {
		for _, script := range scripts {
			require.NoError(t, yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), script.script.ScriptName))
		}
	}

	querySkipUpdate := func(scriptName string) bool {
		resp, err := client.QueryYakScriptSkipUpdate(context.Background(), &ypb.QueryYakScriptRequest{
			IncludedScriptNames: []string{scriptName},
		})
		require.NoError(t, err)
		return resp.SkipUpdate
	}

	setSkipUpdate := func(scriptName string, skipUpdate bool) {
		req := &ypb.SetYakScriptSkipUpdateRequest{
			Field:      &ypb.QueryYakScriptRequest{IncludedScriptNames: []string{scriptName}},
			SkipUpdate: skipUpdate,
		}
		_, err := client.SetYakScriptSkipUpdate(context.Background(), req)
		require.NoError(t, err)
	}

	testcases := []*TestCase{
		{
			script: &schema.YakScript{
				ScriptName: "fileKeywords-test-script-1",
				Type:       "yak",
				Content:    "yakit.AutoInitYakit()\n\n# Input your code!\n\n// 测试",
			},
		},
		{
			script: &schema.YakScript{
				ScriptName: "fileKeywords-script-2",
				Type:       "yak",
				Content:    "yakit.AutoInitYakit()\n\n# Input your code!\n\n// fileKeywords-测试-2",
			},
		},
		{
			script: &schema.YakScript{
				ScriptName: "fileKeywords-test-3",
				Type:       "yak",
				Content:    "yakit.AutoInitYakit()\n\n# Input your code!\n\n// -fileKeywords-script-3",
			},
		},
	}

	createScript(testcases...)
	defer deleteScript(testcases...)

	t.Run("test query skip update by name", func(t *testing.T) {
		for _, tc := range testcases {
			skipUpdate := querySkipUpdate(tc.script.ScriptName)
			require.False(t, skipUpdate)
		}
	})

	t.Run("test set skip update by name", func(t *testing.T) {
		for _, tc := range testcases {
			setSkipUpdate(tc.script.ScriptName, true)
			skipUpdate := querySkipUpdate(tc.script.ScriptName)
			require.True(t, skipUpdate)
		}
	})

	t.Run("test unset skip update by name", func(t *testing.T) {
		for _, tc := range testcases {
			setSkipUpdate(tc.script.ScriptName, true)
			require.True(t, querySkipUpdate(tc.script.ScriptName))

			setSkipUpdate(tc.script.ScriptName, false)
			require.False(t, querySkipUpdate(tc.script.ScriptName))
		}
	})
}
