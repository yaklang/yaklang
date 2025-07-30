package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/information"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func yaklangInspectInformationSend(client ypb.YakClient, yakScriptType, code string, r *ypb.Range) *ypb.YaklangInspectInformationResponse {
	rsp, err := client.YaklangInspectInformation(context.Background(), &ypb.YaklangInspectInformationRequest{
		YakScriptType: yakScriptType,
		YakScriptCode: code,
		Range:         r,
	})
	if err != nil {
		log.Error(err)
		return nil
	}
	return rsp
}

func CompareScriptParams(got, want []*ypb.YakScriptParam) error {
	if len(got) != len(want) {
		return utils.Errorf("cli parameter length not match")
	}

	for i := range want {
		log.Infof("want: %v", want[i])
		log.Infof("got: %v", got[i])
		// compare want and got
		if got[i].Field != want[i].Field {
			return utils.Errorf("cli parameter %d field not match", i)
		}
		if got[i].DefaultValue != want[i].DefaultValue {
			return utils.Errorf("cli parameter %d default value not match", i)
		}
		if got[i].TypeVerbose != want[i].TypeVerbose {
			return utils.Errorf("cli parameter %d type verbose not match", i)
		}
		if got[i].FieldVerbose != want[i].FieldVerbose {
			return utils.Errorf("cli parameter %d field verbose not match", i)
		}
		if got[i].Help != want[i].Help {
			return utils.Errorf("cli parameter %d help not match", i)
		}
		if got[i].Required != want[i].Required {
			return utils.Errorf("cli parameter %d required not match", i)
		}
		if got[i].Group != want[i].Group {
			return utils.Errorf("cli parameter %d group not match", i)
		}
		if got[i].ExtraSetting == "" && want[i].ExtraSetting == "" {
			continue
		}

		var extraWant, extraGot *information.PluginParamSelect
		err1 := json.Unmarshal([]byte(want[i].ExtraSetting), &extraWant)
		err2 := json.Unmarshal([]byte(got[i].ExtraSetting), &extraGot)
		if err1 != nil {
			return utils.Errorf("cli parameter %d want extra setting unmarshal error %v", i, err1)
		}
		if err2 != nil {
			return utils.Errorf("cli parameter %d got extra setting unmarshal error %v", i, err2)
		}
		if extraWant.Double != extraGot.Double {
			return utils.Errorf("cli parameter %d extra setting double not match", i)
		}
		if len(extraWant.Data) != len(extraGot.Data) {
			return utils.Errorf("cli parameter %d extra setting data length not match", i)
		}
		// sort extra*.Data by label
		// sort.Slice(extraWant.Data, func(i, j int) bool {
		// 	return extraWant.Data[i].Key < extraWant.Data[j].Key
		// })
		// sort.Slice(extraGot.Data, func(i, j int) bool {
		// 	return extraGot.Data[i].Key < extraGot.Data[j].Key
		// })
		for j := range extraWant.Data {
			if extraWant.Data[j].Key != extraGot.Data[j].Key {
				return utils.Errorf("cli parameter %d extra setting data %d key not match", i, j)
			}
			if extraWant.Data[j].Label != extraGot.Data[j].Label {
				return utils.Errorf("cli parameter %d extra setting data %d label not match", i, j)
			}
			if extraWant.Data[j].Value != extraGot.Data[j].Value {
				return utils.Errorf("cli parameter %d extra setting data %d value not match", i, j)
			}
		}
	}
	return nil
}

func TestGRPCMUSTPASS_LANGUAGE_InspectInformation_Cli(t *testing.T) {
	local, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	check := func(code string, want []*ypb.YakScriptParam, t *testing.T, callbacks ...func(t *testing.T, params []*ypb.YakScriptParam)) {
		rsp := yaklangInspectInformationSend(local, "yak", code, nil)
		if rsp == nil {
			t.Fatal("no response")
		}
		// check cli parameter
		params := rsp.GetCliParameter()
		if err := CompareScriptParams(params, want); err != nil {
			t.Fatal(err)
		}
		if len(callbacks) > 0 {
			for _, callback := range callbacks {
				callback(t, params)
			}
		}
	}
	t.Run("simple cli parameter not string name", func(t *testing.T) {
		check(
			`
			cli.String('a') // skip 
			cli.String(1) // skip 
			cli.String('aa')
	`,

			[]*ypb.YakScriptParam{
				{Field: "aa", TypeVerbose: "string", FieldVerbose: "aa", MethodType: "string"},
			},
			t,
		)
	})

	t.Run("simple cli parameter", func(t *testing.T) {
		check(
			`
		cli.String(
			"arg1", 
			cli.setDefault("default variable"), 
			cli.setHelp("help information"), 
			cli.setRequired(true),
		)
		cli.Int(
			"arg2", 
			cli.setVerboseName("参数2"),
			cli.setCliGroup("group2"),
			cli.setDefault(1), 
			cli.setHelp("help information 2"),
		)
	`,

			[]*ypb.YakScriptParam{
				{
					Field:        "arg1",
					DefaultValue: "default variable",
					TypeVerbose:  "string",
					FieldVerbose: "arg1",
					Help:         "help information",
					Required:     true,
					Group:        "",
					ExtraSetting: "",
					MethodType:   "string",
				},
				{
					Field:        "arg2",
					DefaultValue: "1",
					TypeVerbose:  "uint",
					FieldVerbose: "参数2",
					Help:         "help information 2",
					Required:     false,
					Group:        "group2",
					ExtraSetting: "",
					MethodType:   "uint",
				},
			},
			t,
		)
	})

	t.Run("cli parameter with select", func(t *testing.T) {
		check(
			`
		cli.StringSlice(
			"arg1", 
			cli.setSelectOption("a", "A"),
			cli.setSelectOption("b", "B"),
			cli.setSelectOption("c", "c"),
			cli.setMultipleSelect(true),
			cli.setHelp("help information"),
		)
	`,

			[]*ypb.YakScriptParam{
				{
					Field:        "arg1",
					TypeVerbose:  "select",
					FieldVerbose: "arg1",
					Help:         "help information",
					Required:     false,
					Group:        "",
					ExtraSetting: "{\"double\":true,\"data\":[{\"key\":\"a\",\"label\":\"a\",\"value\":\"A\"},{\"key\":\"b\",\"label\":\"b\",\"value\":\"B\"},{\"key\":\"c\",\"label\":\"c\",\"value\":\"c\"}]}",
				},
			},
			t,
		)
	})

	t.Run("cli json schema", func(t *testing.T) {
		s := `
{
  "type": "object",
  "properties": {
    "kind": {
      "type": "string",
      "enum": [
        "local",
        "compression",
        "git",
        "svn",
        "jar"
      ],
      "default": "local"
    }
  },
  "allOf": [
    {
      "if": {
        "properties": {
          "kind": {
            "const": "local"
          }
        }
      },
      "then": {
        "properties": {
          "local_path": {
            "type": "string"
          }
        },
        "required": [
          "local_path"
        ]
      }
    },
    {
    // add other kind in this 
    }
    {
      "required": [
        "kind"
      ]
    }
  ],
}
		`
		check(fmt.Sprintf(`
	cli.Json("a", cli.setJsonSchema(<<<JSON
%s
JSON, cli.setUISchema(
    cli.uiGlobalFieldPosition(cli.uiPosDefault),
    cli.uiGroups(
        cli.uiGroup(
            cli.uiField(
				"field1",
				0.5,
				cli.uiFieldComponentStyle({"width":"50%%"}),
				cli.uiFieldPosition(cli.uiPosHorizontal),
				cli.uiFieldWidget(cli.uiWidgetTextarea),
				cli.uiFieldGroups(
					cli.uiGroup(
					cli.uiField("field11", 0.3, cli.uiFieldWidget(cli.uiWidgetPassword)),
					cli.uiField("field12", 0.7, cli.uiFieldWidget(cli.uiWidgetEmail)),
					),
				),
			),
            cli.uiField("field2", 0.5, cli.uiFieldPosition(cli.uiPosHorizontal), cli.uiFieldWidget(cli.uiWidgetPassword)),
        ),
        cli.uiGroup(
             cli.uiField("field3", 1),
        ),
    ),
)))
		`, s),

			[]*ypb.YakScriptParam{
				{
					Field:        "a",
					TypeVerbose:  "json",
					MethodType:   "json",
					FieldVerbose: "a",
					JsonSchema:   s,
				},
			},
			t,
			func(t *testing.T, params []*ypb.YakScriptParam) {
				require.Len(t, params, 1)
				uiSchema := params[0].UISchema
				wantJson := `{"field1":{"ui:classNames":"json-schema-row-form","ui:widget":"textarea","ui:component_style":{"width":"50%"},"field11":{"ui:widget":"password"},"field12":{"ui:widget":"email"},"ui:grid":[{"field11":7,"field12":17}]},"field2":{"ui:classNames":"json-schema-row-form","ui:widget":"password"},"ui:grid":[{"field1":12,"field2":12},{"field3":24}]}`
				got, want := make(map[string]any), make(map[string]any)
				err := json.Unmarshal([]byte(wantJson), &want)
				require.NoError(t, err)
				err = json.Unmarshal([]byte(uiSchema), &got)
				require.NoError(t, err)

				require.Equal(t, want, got)
			},
		)
	})
}

func TestGRPCMUSTPASS_LANGUAGE_InspectInformation_Cli_UI(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	check := func(code string, want []*ypb.YakUIInfo, t *testing.T) {
		rsp := yaklangInspectInformationSend(local, "yak", code, nil)
		if rsp == nil {
			t.Fatal("no response")
		}
		// check ui
		got := rsp.GetUIInfo()
		require.Len(t, got, len(want), "ui length not match")

		for i := range want {
			require.Equal(t, want[i].Typ, got[i].Typ, "ui typ not match")
			require.Equal(t, want[i].Effected, got[i].Effected, "ui effected parameter names not match")
			require.Equal(t, want[i].WhenExpression, got[i].WhenExpression, "ui when expression not match")
		}
	}

	t.Run("show/hide group", func(t *testing.T) {
		check(
			`
		cli.String("a", cli.setCliGroup("group1"))
		cli.Int("b", cli.setCliGroup("group1"))
		cli.String("a2", cli.setCliGroup("group2"))
		cli.Int("b2", cli.setCliGroup("group2"))
		cli.Bool("c")
		cli.UI(cli.showGroup("group1"), cli.whenTrue("c"))
		cli.UI(cli.hideGroup("group2"), cli.whenFalse("c"))
		`,
			[]*ypb.YakUIInfo{
				{Typ: "show", Effected: []string{"a", "b"}, WhenExpression: "c == true"},
				{Typ: "hide", Effected: []string{"a2", "b2"}, WhenExpression: "c == false"},
			},
			t,
		)
	})

	t.Run("show/hide params", func(t *testing.T) {
		check(
			`
		cli.String("a", cli.setCliGroup("group1"))
		cli.Int("b", cli.setCliGroup("group1"))
		cli.String("a2", cli.setCliGroup("group2"))
		cli.Int("b2", cli.setCliGroup("group2"))
		cli.Bool("c")
		cli.UI(cli.showParams("a", "b"), cli.whenTrue("c"))
		cli.UI(cli.hideParams("a2", "b2"), cli.whenFalse("c"))
		`,
			[]*ypb.YakUIInfo{
				{Typ: "show", Effected: []string{"a", "b"}, WhenExpression: "c == true"},
				{Typ: "hide", Effected: []string{"a2", "b2"}, WhenExpression: "c == false"},
			},
			t,
		)
	})

	t.Run("when expression test", func(t *testing.T) {
		check(
			`
		cli.UI(cli.when("zxc && qwe"))
		cli.UI(cli.whenTrue("zxc"))
		cli.UI(cli.whenFalse("qwe"))
		cli.UI(cli.whenEqual("zxc", "qwe"))
		cli.UI(cli.whenNotEqual("zxc", "qwe"))
		cli.UI(cli.whenDefault())
		`,
			[]*ypb.YakUIInfo{
				{WhenExpression: "zxc && qwe"},
				{WhenExpression: "zxc == true"},
				{WhenExpression: "qwe == false"},
				{WhenExpression: "zxc == qwe"},
				{WhenExpression: "zxc != qwe"},
				{WhenExpression: "true"},
			},
			t,
		)
	})
}

func TestGRPCMUSTPASS_LANGUAGE_GetCliCode(t *testing.T) {
	local, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	check := func(t *testing.T, paras []*ypb.YakScriptParam) {
		// get code
		code := getCliCodeFromParam(paras)
		t.Logf("got: \n%s", code)

		// parse params
		rsp := yaklangInspectInformationSend(local, "yak", code, nil)
		if rsp == nil {
			t.Fatal("no response")
		}
		// check  parameter
		if err := CompareScriptParams(rsp.GetCliParameter(), paras); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("simple cli parameter", func(t *testing.T) {
		check(t,
			[]*ypb.YakScriptParam{
				{
					Field:        "arg1",
					DefaultValue: "default variable",
					TypeVerbose:  "string",
					FieldVerbose: "arg1",
					Help:         "help information",
					Required:     true,
					Group:        "",
					ExtraSetting: "",
					MethodType:   "string",
				},
				{
					Field:        "arg2",
					DefaultValue: "1",
					TypeVerbose:  "uint",
					FieldVerbose: "参数2",
					Help:         "help information 2",
					Required:     false,
					Group:        "group2",
					ExtraSetting: "",
					MethodType:   "uint",
				},
			},
		)
	})

	t.Run("cli parameter with select", func(t *testing.T) {
		check(t,
			[]*ypb.YakScriptParam{
				{
					Field:        "arg1",
					TypeVerbose:  "select",
					FieldVerbose: "arg1",
					Help:         "help information",
					Required:     false,
					Group:        "",
					ExtraSetting: "{\"double\":true,\"data\":[{\"key\":\"c\",\"label\":\"c\",\"value\":\"c\"},{\"key\":\"a\",\"label\":\"a\",\"value\":\"A\"},{\"key\":\"b\",\"label\":\"b\",\"value\":\"B\"}]}",
					MethodType:   "select",
				},
			},
		)
	})

	t.Run("cli parameter with file-content", func(t *testing.T) {
		check(t,
			[]*ypb.YakScriptParam{
				{
					Field:        "arg",
					DefaultValue: "",
					TypeVerbose:  "upload-file-content",
					FieldVerbose: "arg",
					Help:         "",
					Required:     false,
					Group:        "",
					ExtraSetting: "",
					MethodType:   "file_content",
				},
			},
		)
	})

	t.Run("cli json schema", func(t *testing.T) {
		json := `
{
  "type": "object",
  "properties": {
    "kind": {
      "type": "string",
      "enum": [
        "local",
        "compression",
        "git",
        "svn",
        "jar"
      ],
      "default": "local"
    }
  },
  "allOf": [
    {
      "if": {
        "properties": {
          "kind": {
            "const": "local"
          }
        }
      },
      "then": {
        "properties": {
          "local_path": {
            "type": "string"
          }
        },
        "required": [
          "local_path"
        ]
      }
    },
    {
    // add other kind in this 
    }
    {
      "required": [
        "kind"
      ]
    }
  ],

}
		`

		check(t, []*ypb.YakScriptParam{
			{
				Field:        "a",
				DefaultValue: "",
				TypeVerbose:  "json",
				FieldVerbose: "aa",
				Help:         "",
				Required:     false,
				Group:        "",
				ExtraSetting: "",
				MethodType:   "json",
				JsonSchema:   json,
			},
		})
	})
}

func TestGRPCMUSTPASS_LANGUAGE_CLICompare(t *testing.T) {
	// getNeedReturn
	check := func(code, want string, param []*ypb.YakScriptParam, t *testing.T) {
		raw, _ := json.Marshal(param)
		jsonBytes := strconv.Quote(string(raw))

		ret, err := getNeedReturn(&schema.YakScript{
			Content: code,
			Params:  jsonBytes,
		})
		if err != nil {
			t.Fatal(err)
		}
		log.Info("got: \n", ret)
		got := getCliCodeFromParam(ret)
		if got != want {
			t.Fatalf("want: \n%s, got: \n%s", want, got)
		}
	}

	t.Run("code and database compare same", func(t *testing.T) {
		check(`
		cli.String(
			"arg1",
			cli.setDefault("default variable"),
			cli.setHelp("help information"),
		)`,
			``,
			[]*ypb.YakScriptParam{
				{
					Field:        "arg1",
					DefaultValue: "default variable",
					TypeVerbose:  "string",
					FieldVerbose: "arg1",
					Help:         "help information",
					Required:     false,
					Group:        "",
					ExtraSetting: "",
					MethodType:   "string",
				},
			},
			t,
		)
	})

	t.Run("code more information", func(t *testing.T) {
		check(`
		cli.String(
			"arg1",
			cli.setDefault("default variable"),
			cli.setHelp("help information"),
		)`,
			``,
			[]*ypb.YakScriptParam{
				{
					Field:        "arg1",
					DefaultValue: "default variable",
					TypeVerbose:  "string",
					FieldVerbose: "arg1",
					Help:         "",
					MethodType:   "string",
				},
			},
			t,
		)
	})

	t.Run("database more information", func(t *testing.T) {
		check(`
		cli.String(
			"arg1",
			cli.setDefault("default variable"),
		)`,
			`cli.String("arg1", cli.setDefault("default variable"),cli.setHelp("help information"))
`,
			[]*ypb.YakScriptParam{
				{
					Field:        "arg1",
					DefaultValue: "default variable",
					TypeVerbose:  "string",
					FieldVerbose: "arg1",
					Help:         "help information",
					MethodType:   "string",
				},
			},
			t,
		)
	})

	t.Run("database same variable but more information", func(t *testing.T) {
		check(`
		domains = cli.String("domains")
		domains = str.ParseStringToLines(domains)
		thread = cli.Int("thread", cli.setDefault(10))
		dnsServer = cli.String("dnsServer", cli.setDefault("114.114.114.114"))
		dnsServer = str.Split(dnsServer, ",")
		cli.check()
		`,
			`cli.Text("domains", cli.setVerboseName("域名"),cli.setRequired(true))
cli.Int("thread", cli.setDefault(10),cli.setVerboseName("线程"),cli.setRequired(true))
cli.String("dnsServer", cli.setDefault("114.114.114.114"),cli.setHelp("逗号分隔多个dns服务器"),cli.setVerboseName("dns服务器"))
`,
			[]*ypb.YakScriptParam{
				{
					Field: "domains", TypeVerbose: "text", FieldVerbose: "域名", Required: true,
				},
				{
					Field: "thread", DefaultValue: "10", TypeVerbose: "uint", FieldVerbose: "线程", Required: true,
				},
				{
					Field: "dnsServer", DefaultValue: "114.114.114.114", TypeVerbose: "string", FieldVerbose: "dns服务器", Help: "逗号分隔多个dns服务器",
				},
			},
			t)
	})
}

func TestGRPCMUSTPASS_LANGUAGE_CLIALL(t *testing.T) {
	code := `
	cli.String(
		"string-arg1", 
		cli.setDefault("default variable"), 
		cli.setHelp("help information"), 
		cli.setRequired(true),
	)
	cli.Int(
		"int-arg2", 
		cli.setVerboseName("参数2"),
		cli.setCliGroup("group2"),
		cli.setDefault(1), 
		cli.setHelp("help information 2"),
	)
	cli.StringSlice(
		"StringSlice-arg1", 
		cli.setSelectOption("a", "A"),
		cli.setSelectOption("b", "B"),
		cli.setSelectOption("c", "c"),
		cli.setMultipleSelect(true),
		cli.setHelp("help information"),
	)

	cli.Text("text-arg", 
		cli.setDefault("text default value"), 
		cli.setHelp("text help information"),
		cli.setVerboseName("text verbose name"),
	)
	cli.YakCode(
		"yakCode-arg",
		cli.setDefault("yakCode default value"),
		cli.setHelp("yakCode help information"),
		cli.setVerboseName("yakCode verbose name"),
		cli.setCliGroup("yakCode group"),
	)
	cli.HTTPPacket(
		"httpPacket-arg",
		cli.setDefault("httpPacket default value"),
		cli.setHelp("httpPacket help information"),
		cli.setVerboseName("httpPacket verbose name"),
		cli.setCliGroup("httpPacket group"),
	)
	`

	local, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	// get params
	rsp := yaklangInspectInformationSend(local, "yak", code, nil)
	if rsp == nil {
		t.Fatal("no response")
	}
	// get params by generate code
	{
		gotCode := getCliCodeFromParam(rsp.GetCliParameter())
		if gotCode == "" {
			t.Fatal("no code generated by rsp")
		}
		log.Info("got code: \n", gotCode)
		// get params by code generated
		gotRsp := yaklangInspectInformationSend(local, "yak", gotCode, nil)
		if gotRsp == nil {
			t.Fatal("no response")
		}
		// check parameter
		if err := CompareScriptParams(rsp.GetCliParameter(), gotRsp.GetCliParameter()); err != nil {
			t.Fatal(err)
		}
	}

	// get params by json saved in database

	{
		// json saved in database, this same like: "GRPCYakScriptToYakitScript" in common/yakgrpc/grpc_manageScript.go
		raw, _ := json.Marshal(rsp.CliParameter)
		jsonBytes := strconv.Quote(string(raw))
		log.Info("jsonBytes: \n", jsonBytes)

		// _ = jsonBytes
		gotParameter, err := getParameterFromParamJson(jsonBytes)
		if err != nil {
			t.Fatal(err)
		}
		gotCode := getCliCodeFromParam(gotParameter)
		if gotCode == "" {
			t.Fatal("no code generated by json")
		}
		log.Info("got code: \n", gotCode)
		// get params by code generated
		gotRsp := yaklangInspectInformationSend(local, "yak", gotCode, nil)
		if gotRsp == nil {
			t.Fatal("no response")
		}
		// check parameter
		if err := CompareScriptParams(rsp.GetCliParameter(), gotRsp.GetCliParameter()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGRPCMUSTPASS_LANGUAGE_CLI_SORT(t *testing.T) {
	test := assert.New(t)
	code := `
// this code generated by yaklang from database
cli.String("target", cli.setDefault("172.24.145.120"),cli.setHelp("设置爬虫的目标信息，可以支持比较自由的格式，支持逗号分隔，可以输入 IP / 域名 / 主机名 / URL "),cli.setVerboseName("爬虫扫描目标"),cli.setRequired(true))
cli.String("proxy", cli.setHelp("设置代理"),cli.setVerboseName("设置代理"),cli.setCliGroup("网络参数"))
cli.Int("timeout",  cli.setDefault(10),cli.setHelp("每个请求的最大超时时间"),cli.setVerboseName("超时时间"),cli.setCliGroup("网络参数"))
cli.Int("max-depth", cli.setDefault(4),cli.setHelp("设置爬虫的最大深度（逻辑深度，并不是级数）"),cli.setVerboseName("最大深度"),cli.setCliGroup("速率与限制"))
cli.Int("concurrent", cli.setDefault(50),cli.setHelp("爬虫的并发请求量（可以理解为线程数）"),cli.setVerboseName("并发量"),cli.setCliGroup("速率与限制"))
cli.Int("max-links", cli.setDefault(10000),cli.setHelp("爬虫获取到的最大量URL（这个选项一般用来限制无限制的爬虫，一般不需要改动）\n"),cli.setVerboseName("最大URL数"),cli.setCliGroup("速率与限制"))
cli.Int("max-requests", cli.setDefault(2000),cli.setHelp("本次爬虫最多发出多少个请求？（一般用于限制爬虫行为）"),cli.setVerboseName("最大请求数"),cli.setCliGroup("速率与限制"))
cli.String("login-user", cli.setDefault("admin"),cli.setHelp("如果遇到了登录名的话，可以通过这个登陆自动设置，但是无法保证成功"),cli.setVerboseName("尝试登录名"),cli.setCliGroup("登陆"))
cli.String("login-pass", cli.setDefault("password"),cli.setHelp("如果遇到登陆密码，也许这个可以帮助你登陆，但是这个登陆并不一定能生效"),cli.setVerboseName("尝试登陆密码"),cli.setCliGroup("登陆"))
cli.String("cookie", cli.setHelp("设置原始 Cookie，一般用来解决登陆以后的情况"),cli.setVerboseName("原始 Cookie"),cli.setCliGroup("登陆"))
cli.String("user-agent", cli.setDefault("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36"),cli.setVerboseName("用户代理"))
cli.Int("retry", cli.setDefault(2),cli.setVerboseName("重试次数"))
cli.Int("redirectTimes", cli.setDefault(3),cli.setVerboseName("重定向次数"))
cli.Bool("basic-auth", cli.setDefault(false),cli.setHelp("是否开启基础认证"),cli.setVerboseName("基础认证开关"))
cli.String("basic-auth-user", cli.setVerboseName("基础认证用户名"))
cli.String("basic-auth-pass", cli.setVerboseName("基础认证密码"))
cli.check()
	`
	client, err := NewLocalClient()
	test.Nil(err)

	rsp := yaklangInspectInformationSend(client, "yak", code, nil)
	got := rsp.CliParameter
	log.Infof("got: %v", got)

	want := []*ypb.YakScriptParam{
		{Field: "target", DefaultValue: "172.24.145.120", TypeVerbose: "string", FieldVerbose: "爬虫扫描目标", Help: "设置爬虫的目标信息，可以支持比较自由的格式，支持逗号分隔，可以输入 IP / 域名 / 主机名 / URL ", Required: true, MethodType: "string"},
		{Field: "proxy", TypeVerbose: "string", FieldVerbose: "设置代理", Help: "设置代理", Group: "网络参数", MethodType: "string"},
		{Field: "login-user", DefaultValue: "admin", TypeVerbose: "string", FieldVerbose: "尝试登录名", Help: "如果遇到了登录名的话，可以通过这个登陆自动设置，但是无法保证成功", Group: "登陆", MethodType: "string"},
		{Field: "login-pass", DefaultValue: "password", TypeVerbose: "string", FieldVerbose: "尝试登陆密码", Help: "如果遇到登陆密码，也许这个可以帮助你登陆，但是这个登陆并不一定能生效", Group: "登陆", MethodType: "string"},
		{Field: "cookie", TypeVerbose: "string", FieldVerbose: "原始 Cookie", Help: "设置原始 Cookie，一般用来解决登陆以后的情况", Group: "登陆", MethodType: "string"},
		{Field: "user-agent", DefaultValue: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36", TypeVerbose: "string", FieldVerbose: "用户代理", MethodType: "string"},
		{Field: "basic-auth-user", TypeVerbose: "string", FieldVerbose: "基础认证用户名", MethodType: "string"},
		{Field: "basic-auth-pass", TypeVerbose: "string", FieldVerbose: "基础认证密码", MethodType: "string"},
		{Field: "timeout", DefaultValue: "10", TypeVerbose: "uint", FieldVerbose: "超时时间", Help: "每个请求的最大超时时间", Group: "网络参数", MethodType: "uint"},
		{Field: "max-depth", DefaultValue: "4", TypeVerbose: "uint", FieldVerbose: "最大深度", Help: "设置爬虫的最大深度（逻辑深度，并不是级数）", Group: "速率与限制", MethodType: "uint"},
		{Field: "concurrent", DefaultValue: "50", TypeVerbose: "uint", FieldVerbose: "并发量", Help: "爬虫的并发请求量（可以理解为线程数）", Group: "速率与限制", MethodType: "uint"},
		{Field: "max-links", DefaultValue: "10000", TypeVerbose: "uint", FieldVerbose: "最大URL数", Help: "爬虫获取到的最大量URL（这个选项一般用来限制无限制的爬虫，一般不需要改动）\n", Group: "速率与限制", MethodType: "uint"},
		{Field: "max-requests", DefaultValue: "2000", TypeVerbose: "uint", FieldVerbose: "最大请求数", Help: "本次爬虫最多发出多少个请求？（一般用于限制爬虫行为）", Group: "速率与限制", MethodType: "uint"},
		{Field: "retry", DefaultValue: "2", TypeVerbose: "uint", FieldVerbose: "重试次数", MethodType: "uint"},
		{Field: "redirectTimes", DefaultValue: "3", TypeVerbose: "uint", FieldVerbose: "重定向次数", MethodType: "uint"},
		{Field: "basic-auth", DefaultValue: "false", TypeVerbose: "boolean", FieldVerbose: "基础认证开关", Help: "是否开启基础认证", MethodType: "boolean"},
	}

	test.Nil(CompareScriptParams(got, want))
}

func TestGRPCMUSTPASS_LANGUAGE_GetCliGRPC(t *testing.T) {
	local, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	check := func(code string, param []*ypb.YakScriptParam, want string, t *testing.T) {
		name, clearFunc, err := yakit.CreateTemporaryYakScriptEx("test", code)
		require.NoError(t, err)
		defer clearFunc()

		_, err = local.SaveYakScript(context.Background(), &ypb.YakScript{
			ScriptName: name,
			Content:    code,
			Type:       "yak",
			Params:     param,
		})
		require.NoError(t, err, "save yak script error")

		if rsp, err := local.YaklangGetCliCodeFromDatabase(context.Background(), &ypb.YaklangGetCliCodeFromDatabaseRequest{
			ScriptName: name,
		}); err == nil {
			log.Infof("rsp: %s", rsp)

			if rsp.NeedHandle {
				if len(want) == 0 {
					t.Fatal("need handle should be false")
				}

				if rsp.Code != want {
					t.Fatalf("want: %s, got: %s", want, rsp.Code)
				}
			} else {
				if len(want) != 0 {
					t.Fatalf("need handle should be true, but got false")
				}
			}
		} else {
			t.Fatal(err)
		}
	}

	t.Run("database information more", func(t *testing.T) {
		// should return code
		check(
			`cli.String("arg")`,
			[]*ypb.YakScriptParam{
				{
					Field:        "arg",
					DefaultValue: "\"aaa\"",
					TypeVerbose:  "string",
					FieldVerbose: "参数1",
					Help:         "这个是参数1",
					MethodType:   "string",
				},
			},
			`/*
// this code generated by yaklang from database
cli.String("arg", cli.setDefault("\"aaa\""),cli.setHelp("这个是参数1"),cli.setVerboseName("参数1"))

*/`,
			t,
		)
	})

	t.Run("code information more", func(t *testing.T) {
		// should return false
		check(
			`cli.Int("arg", cli.setHelp("这个是参数1"),cli.setVerboseName("参数1"))`,
			[]*ypb.YakScriptParam{},
			"", t,
		)
	})

	t.Run("code and database information same", func(t *testing.T) {
		// should return false
		check(
			`cli.String("arg1",cli.setDefault("default variable"),cli.setHelp("help information"))`,
			[]*ypb.YakScriptParam{
				{
					Field:        "arg1",
					DefaultValue: "default variable",
					TypeVerbose:  "string",
					FieldVerbose: "arg1",
					Help:         "help information",
					Required:     false,
					Group:        "",
					ExtraSetting: "",
					MethodType:   "string",
				},
			},
			"", t,
		)
	})
}

func TestGRPCMUSTPASS_LANGUAGE_InspectInformation_Risk(t *testing.T) {
	local, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	compareRisks := func(got, want []*ypb.YakRiskInfo, t *testing.T) {
		// compare got and want
		if len(got) != len(want) {
			t.Errorf("risk info length not match")
		}
		log.Info("got: \n", got)
		log.Info("want: \n", want)
		for i := range want {
			if got[i].Level != want[i].Level {
				t.Errorf("risk info %d level not match", i)
			}
			if got[i].TypeVerbose != want[i].TypeVerbose {
				t.Errorf("risk info %d type verbose not match", i)
			}
			if got[i].CVE != want[i].CVE {
				t.Errorf("risk info %d CVE not match", i)
			}
			if got[i].Description != want[i].Description {
				t.Errorf("risk info %d description not match", i)
			}
			if got[i].Solution != want[i].Solution {
				t.Errorf("risk info %d solution not match", i)
			}
		}
	}

	check := func(code string, want []*ypb.YakRiskInfo, t *testing.T) {
		req := yaklangInspectInformationSend(local, "yak", code, nil)
		if req == nil {
			t.Fatal("no response")
		}

		got := req.RiskInfo
		compareRisks(got, want, t)
	}

	t.Run("simple risk info", func(t *testing.T) {
		check(
			`
		risk.NewRisk("a",
			risk.severity("high"),
			risk.cve("CVE-2020-1234"),
			risk.description("description"),
			risk.typeVerbose("typeVerbose"),
			risk.solution("solution"),
		)
		`,
			[]*ypb.YakRiskInfo{
				{
					Level:       "high",
					TypeVerbose: "其他",
					CVE:         "CVE-2020-1234",
					Description: "description",
					Solution:    "solution",
				},
			},
			t,
		)
	})

	t.Run("risk info with risk.type", func(t *testing.T) {
		check(
			`
		risk.NewRisk("a",
			risk.severity("high"),
			risk.cve("CVE-2020-1234"),
			risk.description("description"),
			risk.type("type"),
			risk.solution("solution"),
		)
		`,
			[]*ypb.YakRiskInfo{
				{
					Level:       "high",
					TypeVerbose: "其他",
					CVE:         "CVE-2020-1234",
					Description: "description",
					Solution:    "solution",
				},
			},
			t,
		)
	})

	t.Run("risk info with cve", func(t *testing.T) {
		tempFp, err := os.CreateTemp("", "Date.db")
		if err != nil {
			log.Errorf("%v", err)
		}
		defer tempFp.Close()

		M := cveresources.GetManager(tempFp.Name(), true)
		db := M.DB
		if db == nil {
			t.Fatal("no database")
		}
		cve := "CVE-9090-1234"
		// create cve
		cveresources.CreateOrUpdateCVE(db, cve, &cveresources.CVE{
			CVE:               "CVE-9090-1234",
			DescriptionMain:   "description",
			DescriptionMainZh: "中文描述",
			Solution:          "solution",
			Severity:          "high",
		})
		got := information.RiskInfo2grpc([]*information.RiskInfo{
			{
				CVE: cve,
			},
		}, db)
		want := []*ypb.YakRiskInfo{
			{
				Level:       "high",
				CVE:         cve,
				Description: "中文描述",
				Solution:    "solution",
			},
		}
		compareRisks(got, want, t)
	})
}

type pluginTagCheck struct {
	code       string
	expectTag  []string
	pluginType string
}

func TestGRPCMUSTPASS_LANGUAGE_InspectInformation_Tag(t *testing.T) {
	testcase := []pluginTagCheck{
		{
			code: `
handle = func(a){
ai.Chat()
}
`,
			expectTag:  []string{information.AI_PLUGIN},
			pluginType: "codec",
		},
		{
			code: `
handle = func(a){
println(ai.Chat)
}
`,
			expectTag:  []string{},
			pluginType: "codec",
		},
		{
			code: `
handle = func(a){
ai.OpenAI()
}
`,
			expectTag:  []string{},
			pluginType: "codec",
		},
		{
			code: `
hijackHTTPRequest = func(isHttps, url, req, forward /*func(modifiedRequest []byte)*/, drop /*func()*/) {
	forward()	
}
hijackHTTPResponseEx = func(isHttps, url, req, rsp, forward, drop) {
	drop()
}
`,
			expectTag:  []string{information.DROP_HTTP_PACKET, information.FORWARD_HTTP_PACKET},
			pluginType: "mitm",
		},
		{
			code: `
hijackHTTPRequest = func(isHttps, url, req, forward /*func(modifiedRequest []byte)*/, drop /*func()*/) {
	drop()	
}
hijackHTTPResponseEx = func(isHttps, url, req, rsp, forward, drop) {
	println(forward)
}
`,
			expectTag:  []string{information.DROP_HTTP_PACKET},
			pluginType: "mitm",
		},
	}

	for i, check := range testcase {
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			prog, err := static_analyzer.SSAParse(check.code, check.pluginType)
			if err != nil {
				t.Fatal(err)
			}
			require.ElementsMatch(t, check.expectTag, information.ParseTags(prog))
		})
	}
}

func TestGRPCMUSTPASS_LANGUAGE_InspectInformation_CLI_SelectOption(t *testing.T) {

	t.Run("code gen by both", func(t *testing.T) {
		json := `"[{\"Field\":\"types\",\"DefaultValue\":\"admin,backup,robots-100\",\"TypeVerbose\":\"select\",\"FieldVerbose\":\"检查项目\",\"Help\":\"选择内置的字典来完成测试\",\"Required\":true,\"ExtraSetting\":\"{\\\"double\\\":true,\\\"data\\\":[{\\\"key\\\":\\\"爆破备份文件\\\",\\\"label\\\":\\\"爆破备份文件\\\",\\\"value\\\":\\\"backup\\\"},{\\\"key\\\":\\\"\\\", \\\"label\\\":\\\"爆破后台\\\",\\\"value\\\":\\\"admin\\\"}]}\"}]"`
		params, err := getParameterFromParamJson(json)
		assert.NoError(t, err)
		log.Infof("params: %v", params)

		code := getCliCodeFromParam(params)
		log.Infof("code: %s", code)
		if !strings.Contains(code, `cli.StringSlice("types", cli.setMultipleSelect(true),cli.setSelectOption("爆破备份文件", "backup"),cli.setSelectOption("爆破后台", "admin"),cli.setHelp("选择内置的字典来完成测试"),cli.setVerboseName("检查项目"),cli.setRequired(true))`) {
			t.Fatalf("code not match")
		}
	})

	t.Run("code gen by key", func(t *testing.T) {
		json := `"[{\"Field\":\"types\",\"DefaultValue\":\"admin,backup,robots-100\",\"TypeVerbose\":\"select\",\"FieldVerbose\":\"检查项目\",\"Help\":\"选择内置的字典来完成测试\",\"Required\":true,\"ExtraSetting\":\"{\\\"double\\\":true,\\\"data\\\":[{\\\"key\\\":\\\"爆破备份文件\\\",\\\"value\\\":\\\"backup\\\"},{\\\"key\\\":\\\"爆破后台\\\",\\\"value\\\":\\\"admin\\\"}]}\"}]"`
		params, err := getParameterFromParamJson(json)
		assert.NoError(t, err)
		log.Infof("params: %v", params)

		code := getCliCodeFromParam(params)
		log.Infof("code: %s", code)
		if !strings.Contains(code, `cli.StringSlice("types", cli.setMultipleSelect(true),cli.setSelectOption("爆破备份文件", "backup"),cli.setSelectOption("爆破后台", "admin"),cli.setHelp("选择内置的字典来完成测试"),cli.setVerboseName("检查项目"),cli.setRequired(true))`) {
			t.Fatalf("code not match")
		}
	})

	t.Run("code gen by label", func(t *testing.T) {
		json := `"[{\"Field\":\"types\",\"DefaultValue\":\"admin,backup,robots-100\",\"TypeVerbose\":\"select\",\"FieldVerbose\":\"检查项目\",\"Help\":\"选择内置的字典来完成测试\",\"Required\":true,\"ExtraSetting\":\"{\\\"double\\\":true,\\\"data\\\":[{\\\"label\\\":\\\"爆破备份文件\\\",\\\"value\\\":\\\"backup\\\"},{\\\"label\\\":\\\"爆破后台\\\",\\\"value\\\":\\\"admin\\\"}]}\"}]"`
		params, err := getParameterFromParamJson(json)
		assert.NoError(t, err)
		log.Infof("params: %v", params)

		code := getCliCodeFromParam(params)
		log.Infof("code: %s", code)
		if !strings.Contains(code, `cli.StringSlice("types", cli.setMultipleSelect(true),cli.setSelectOption("爆破备份文件", "backup"),cli.setSelectOption("爆破后台", "admin"),cli.setHelp("选择内置的字典来完成测试"),cli.setVerboseName("检查项目"),cli.setRequired(true))`) {
			t.Fatalf("code not match")
		}
	})

	t.Run("code parse", func(t *testing.T) {
		code := `
		cli.StringSlice(
			"types", 
			cli.setSelectOption("", "backup"), 
		)
		`

		client, err := NewLocalClient()
		assert.NoError(t, err)

		rsp := yaklangInspectInformationSend(client, "yak", code, nil)
		got := rsp.CliParameter
		log.Infof("got: %v", got)

		want := []*ypb.YakScriptParam{
			{
				Field: "types", TypeVerbose: "select",
				FieldVerbose: "types",
				ExtraSetting: "{\"double\":false,\"data\":[{\"key\":\"backup\",\"label\":\"backup\",\"value\":\"backup\"}]}",
				MethodType:   "select",
			},
		}
		assert.NoError(t, CompareScriptParams(got, want))
	})
}

func TestGRPCMUSTPASS_LANGUAGE_YakCode_StringLiteral_Have_CRLF(t *testing.T) {
	codeCRLF := "info = cli.Json(\r\n\r\n    \"info\", \r\n    cli.setVerboseName(\"项目信息\"), \r\n    \r\n    cli.setJsonSchema(\r\n        `\r\n{\r\n    \"type\": \"object\",\r\n    \"properties\": {\r\n        \"DataPacket\": {\r\n            \"title\":\"数据包\",\r\n            \"type\": \"string\"\r\n        },    \r\n        \"StatusCodet\": {\r\n        \"title\": \"- 保存状态码 -不支持输入\",\r\n        \"type\": \"array\",\r\n        \"uniqueItems\": false,\r\n        \"items\": [\r\n                {   \r\n                    \"title\":\"选择状态码\",\r\n                    \"type\": \"array\",\r\n                    \"items\": {\r\n                        \"type\": \"number\",\r\n                        \"enum\": [200, 302, 400, 401, 403,500]\r\n                    },\r\n                    \"uniqueItems\": true\r\n                },\r\n                {\r\n\r\n                    \"type\": \"boolean\"\r\n                }\r\n            ],\r\n        \"default\": [[],false]\r\n        },        \r\n        \"IncludeKeyWord\": {\r\n        \"title\": \"关键字\",\r\n        \"type\": \"array\",\r\n        \"uniqueItems\": false,\r\n        \"items\": [\r\n                {   \r\n                    \"title\":\"选择状态码\",\r\n                    \"type\": \"array\",\r\n                    \"items\": {\r\n                        \"type\": \"number\",\r\n                        \"enum\": [200, 302, 400, 401, 403,500]\r\n                    },\r\n                    \"uniqueItems\": true\r\n                },\r\n                {\r\n\r\n                    \"type\": \"boolean\"\r\n                }\r\n            ],\r\n        \"default\": [[],false]\r\n        }\r\n    },\r\n    \"allOf\": [\r\n\r\n        {\r\n            \"if\": {\r\n                \"properties\": {\r\n                    \"StatusCodet\": {\r\n                        \"items\": [\r\n                            {\r\n                                \"contains\":{\r\n                                    \"const\": 302\r\n                                }\r\n                            }\r\n                        ]\r\n                    }\r\n                }\r\n            },\r\n            \"then\": {\r\n                \"properties\": {\r\n                    \"Isredirect\": {\r\n                        \"title\":\"开启重定向\",\r\n                        \"type\": \"boolean\",\r\n                        \"default\": false,\r\n                        \"description\": \"重定向下一个页面\"\r\n                    }\r\n                }\r\n            }\r\n        },\r\n                {\r\n            \"if\": {\r\n                \"properties\": {\r\n                    \"DataPacket\": {\r\n                        \"const\": \"\"\r\n                    }\r\n                }\r\n            },\r\n            \"then\": {\r\n                \"required\": [\r\n                    \"DataPacket\",\r\n                    \"IsHttp\"\r\n                ]\r\n            }\r\n        },\r\n        {\r\n            \"if\": {\r\n                \"properties\": {\r\n                    \"StatusCodet\": {\r\n                        \"items\": [\r\n                            {\r\n\r\n                            },\r\n                            {\r\n                                \"const\": true\r\n                            }\r\n                        ]\r\n                    }\r\n                }\r\n            },\r\n            \"then\": {\r\n                \"properties\": {\r\n                    \"StatusCodet\": {\r\n                        \"items\": [\r\n                            {\r\n\r\n                            },\r\n                            {\r\n                                \"title\": \"丢弃\",\r\n                                \"description\":\"已设置为【【丢弃】】\"\r\n                            }\r\n                        ]\r\n                    }\r\n\r\n                }\r\n            },\r\n            \"else\": {\r\n                \"properties\": {\r\n                    \"StatusCodet\": {\r\n                        \"items\": [\r\n                            {\r\n\r\n                            },\r\n                            {\r\n                                \"title\": \"保存\",\r\n                                \"description\":\"已设置为【【保存】】\"\r\n                            }\r\n                        ]\r\n                    }\r\n                }\r\n            }\r\n        },\r\n        {\r\n            \"if\": {\r\n                \"properties\": {\r\n                    \"IncludeKeyWord\": {\r\n                        \"items\": [\r\n                            {\r\n\r\n                            },\r\n                            {\r\n                                \"const\": true\r\n                            }\r\n                        ]\r\n                    }\r\n                }\r\n            },\r\n            \"then\": {\r\n                \"properties\": {\r\n                    \"IncludeKeyWord\": {\r\n                        \"items\": [\r\n                            {\r\n\r\n                            },\r\n                            {\r\n                                \"title\": \"丢弃1\",\r\n                                \"description\":\"已设置为【【丢弃】】\"\r\n                            }\r\n                        ]\r\n                    }\r\n                }\r\n            },\r\n            \r\n            \"else\": {\r\n                \"properties\": {\r\n                    \"IncludeKeyWord\": {\r\n                        \"items\": [\r\n                            {\r\n\r\n                            },\r\n                            {\r\n                                \"title\": \"保存1\",\r\n                                \"description\":\"已设置为【【保存】】\"\r\n                            }\r\n                        ]\r\n                    }\r\n                }\r\n            }\r\n        }\r\n\r\n\r\n    ]\r\n    \r\n}\r\n`, \r\n\r\n        cli.setUISchema(\r\n            cli.uiGlobalFieldPosition(cli.uiPosHorizontal), \r\n            cli.uiGroups(\r\n                cli.uiGroup(cli.uiField(\"DataPacket\", 1, cli.uiFieldWidget(cli.uiWidgetTextarea))), \r\n                cli.uiGroup(cli.uiField(\"Key_Word\", 1, cli.uiFieldWidget(cli.uiWidgetTextarea))),\r\n                cli.uiGroup(cli.uiField(\"IsHttp\", 1), \r\n                cli.uiField(\"OpenLoger\", 1), \r\n                cli.uiField(\"UserALL\", 1),\r\n                cli.uiField(\"StatusCodet\", 1),\r\n                cli.uiField(\"Isredirect\", 1),\r\n                cli.uiField(\"IncludeKeyWord\", 1),\r\n\r\n                ), \r\n            ), \r\n        ), \r\n    ), \r\n    cli.setRequired(true), \r\n)\r\ncli.check()"

	codeLF := "info = cli.Json(\n\n    \"info\", \n    cli.setVerboseName(\"项目信息\"), \n    \n    cli.setJsonSchema(\n        `\n{\n    \"type\": \"object\",\n    \"properties\": {\n        \"DataPacket\": {\n            \"title\":\"数据包\",\n            \"type\": \"string\"\n        },    \n        \"StatusCodet\": {\n        \"title\": \"- 保存状态码 -不支持输入\",\n        \"type\": \"array\",\n        \"uniqueItems\": false,\n        \"items\": [\n                {   \n                    \"title\":\"选择状态码\",\n                    \"type\": \"array\",\n                    \"items\": {\n                        \"type\": \"number\",\n                        \"enum\": [200, 302, 400, 401, 403,500]\n                    },\n                    \"uniqueItems\": true\n                },\n                {\n\n                    \"type\": \"boolean\"\n                }\n            ],\n        \"default\": [[],false]\n        },        \n        \"IncludeKeyWord\": {\n        \"title\": \"关键字\",\n        \"type\": \"array\",\n        \"uniqueItems\": false,\n        \"items\": [\n                {   \n                    \"title\":\"选择状态码\",\n                    \"type\": \"array\",\n                    \"items\": {\n                        \"type\": \"number\",\n                        \"enum\": [200, 302, 400, 401, 403,500]\n                    },\n                    \"uniqueItems\": true\n                },\n                {\n\n                    \"type\": \"boolean\"\n                }\n            ],\n        \"default\": [[],false]\n        }\n    },\n    \"allOf\": [\n\n        {\n            \"if\": {\n                \"properties\": {\n                    \"StatusCodet\": {\n                        \"items\": [\n                            {\n                                \"contains\":{\n                                    \"const\": 302\n                                }\n                            }\n                        ]\n                    }\n                }\n            },\n            \"then\": {\n                \"properties\": {\n                    \"Isredirect\": {\n                        \"title\":\"开启重定向\",\n                        \"type\": \"boolean\",\n                        \"default\": false,\n                        \"description\": \"重定向下一个页面\"\n                    }\n                }\n            }\n        },\n                {\n            \"if\": {\n                \"properties\": {\n                    \"DataPacket\": {\n                        \"const\": \"\"\n                    }\n                }\n            },\n            \"then\": {\n                \"required\": [\n                    \"DataPacket\",\n                    \"IsHttp\"\n                ]\n            }\n        },\n        {\n            \"if\": {\n                \"properties\": {\n                    \"StatusCodet\": {\n                        \"items\": [\n                            {\n\n                            },\n                            {\n                                \"const\": true\n                            }\n                        ]\n                    }\n                }\n            },\n            \"then\": {\n                \"properties\": {\n                    \"StatusCodet\": {\n                        \"items\": [\n                            {\n\n                            },\n                            {\n                                \"title\": \"丢弃\",\n                                \"description\":\"已设置为【【丢弃】】\"\n                            }\n                        ]\n                    }\n\n                }\n            },\n            \"else\": {\n                \"properties\": {\n                    \"StatusCodet\": {\n                        \"items\": [\n                            {\n\n                            },\n                            {\n                                \"title\": \"保存\",\n                                \"description\":\"已设置为【【保存】】\"\n                            }\n                        ]\n                    }\n                }\n            }\n        },\n        {\n            \"if\": {\n                \"properties\": {\n                    \"IncludeKeyWord\": {\n                        \"items\": [\n                            {\n\n                            },\n                            {\n                                \"const\": true\n                            }\n                        ]\n                    }\n                }\n            },\n            \"then\": {\n                \"properties\": {\n                    \"IncludeKeyWord\": {\n                        \"items\": [\n                            {\n\n                            },\n                            {\n                                \"title\": \"丢弃1\",\n                                \"description\":\"已设置为【【丢弃】】\"\n                            }\n                        ]\n                    }\n                }\n            },\n            \n            \"else\": {\n                \"properties\": {\n                    \"IncludeKeyWord\": {\n                        \"items\": [\n                            {\n\n                            },\n                            {\n                                \"title\": \"保存1\",\n                                \"description\":\"已设置为【【保存】】\"\n                            }\n                        ]\n                    }\n                }\n            }\n        }\n\n\n    ]\n    \n}\n`, \n\n        cli.setUISchema(\n            cli.uiGlobalFieldPosition(cli.uiPosHorizontal), \n            cli.uiGroups(\n                cli.uiGroup(cli.uiField(\"DataPacket\", 1, cli.uiFieldWidget(cli.uiWidgetTextarea))), \n                cli.uiGroup(cli.uiField(\"Key_Word\", 1, cli.uiFieldWidget(cli.uiWidgetTextarea))),\n                cli.uiGroup(cli.uiField(\"IsHttp\", 1), \n                cli.uiField(\"OpenLoger\", 1), \n                cli.uiField(\"UserALL\", 1),\n                cli.uiField(\"StatusCodet\", 1),\n                cli.uiField(\"Isredirect\", 1),\n                cli.uiField(\"IncludeKeyWord\", 1),\n\n                ), \n            ), \n        ), \n    ), \n    cli.setRequired(true), \n)\ncli.check()"

	client, err := NewLocalClient()
	require.NoError(t, err)

	t.Run("CRLF_version", func(t *testing.T) {
		t.Log("Testing code with CRLF...")
		rsp := yaklangInspectInformationSend(client, "yak", codeCRLF, nil)
		require.NotNil(t, rsp, "response should not be nil")
		params := rsp.CliParameter
		require.Len(t, params, 1)
		jsonData := params[0].GetJsonSchema()
		t.Logf("CRLF JSON length: %d", len(jsonData))

		var data any
		err = json.Unmarshal([]byte(jsonData), &data)
		if err != nil {
			t.Logf("❌ CRLF version failed with error: %v", err)
		} else {
			t.Log("✅ CRLF version succeeded")
		}
		require.NoError(t, err, "CRLF version should parse JSON successfully")
	})

	t.Run("LF_version", func(t *testing.T) {
		t.Log("Testing code with LF...")
		rsp := yaklangInspectInformationSend(client, "yak", codeLF, nil)
		require.NotNil(t, rsp, "response should not be nil")
		params := rsp.CliParameter
		require.Len(t, params, 1)
		jsonData := params[0].GetJsonSchema()
		t.Logf("LF JSON length: %d", len(jsonData))

		var data any
		err = json.Unmarshal([]byte(jsonData), &data)
		if err != nil {
			t.Logf("❌ LF version failed with error: %v", err)
		} else {
			t.Log("✅ LF version succeeded")
		}
		require.NoError(t, err, "LF version should parse JSON successfully")
	})
}
