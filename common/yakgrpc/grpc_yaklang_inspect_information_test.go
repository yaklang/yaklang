package yakgrpc

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"testing"

	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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

		var extraWant, extraGot *PluginParamSelect
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
		sort.Slice(extraWant.Data, func(i, j int) bool {
			return extraWant.Data[i].Label < extraWant.Data[j].Label
		})
		sort.Slice(extraGot.Data, func(i, j int) bool {
			return extraGot.Data[i].Label < extraGot.Data[j].Label
		})
		for j := range extraWant.Data {
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

	check := func(code string, want []*ypb.YakScriptParam, t *testing.T) {
		rsp := yaklangInspectInformationSend(local, "yak", code, nil)
		if rsp == nil {
			t.Fatal("no response")
		}
		// check cli parameter
		if err := CompareScriptParams(rsp.GetCliParameter(), want); err != nil {
			t.Fatal(err)
		}
	}

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
					ExtraSetting: "{\"double\":true,\"data\":[{\"label\":\"c\",\"value\":\"c\"},{\"label\":\"a\",\"value\":\"A\"},{\"label\":\"b\",\"value\":\"B\"}]}",
				},
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
					ExtraSetting: "{\"double\":true,\"data\":[{\"label\":\"c\",\"value\":\"c\"},{\"label\":\"a\",\"value\":\"A\"},{\"label\":\"b\",\"value\":\"B\"}]}",
				},
			},
		)
	})
}

func TestGRPCMUSTPASS_LANGUAGE_CLICompare(t *testing.T) {
	// getNeedReturn
	check := func(code string, param, want []*ypb.YakScriptParam, t *testing.T) {
		raw, _ := json.Marshal(param)
		jsonBytes := strconv.Quote(string(raw))

		ret, err := getNeedReturn(&yakit.YakScript{
			Content: code,
			Params:  jsonBytes,
		})
		if err != nil {
			t.Fatal(err)
		}
		log.Info("got: \n", ret)
		if err := CompareScriptParams(ret, want); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("code and database compare same", func(t *testing.T) {
		check(`
		cli.String(
			"arg1",
			cli.setDefault("default variable"),
			cli.setHelp("help information"),
		)`,
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
				},
			},
			[]*ypb.YakScriptParam{},
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
			[]*ypb.YakScriptParam{
				{
					Field:        "arg1",
					DefaultValue: "default variable",
					TypeVerbose:  "string",
					FieldVerbose: "arg1",
					Help:         "",
				},
			},
			[]*ypb.YakScriptParam{},
			t,
		)
	})

	t.Run("database more information", func(t *testing.T) {
		check(`
		cli.String(
			"arg1",
			cli.setDefault("default variable"),
		)`,
			[]*ypb.YakScriptParam{
				{
					Field:        "arg1",
					DefaultValue: "default variable",
					TypeVerbose:  "string",
					FieldVerbose: "arg1",
					Help:         "help information",
				},
			},
			[]*ypb.YakScriptParam{
				{
					Field:        "arg1",
					DefaultValue: "default variable",
					TypeVerbose:  "string",
					FieldVerbose: "arg1",
					Help:         "help information",
				},
			},
			t,
		)
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

func TestGRPCMUSTPASS_LANGUAGE_GetCliGRPC(t *testing.T) {
	local, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	yakit.CreateTemporaryYakScript("test", `cli.String("arg")`)
	local.SaveYakScript(context.Background(), &ypb.YakScript{
		ScriptName: "test",
		Content:    `cli.String("arg")`,
		Type:       "yak",
		Params: []*ypb.YakScriptParam{
			{
				Field:        "arg",
				DefaultValue: "\"aaa\"",
				TypeVerbose:  "string",
				FieldVerbose: "参数1",
				Help:         "这个是参数1",
			},
		},
	})
	if rsp, err := local.YaklangGetCliCodeFromDatabase(context.Background(), &ypb.YaklangGetCliCodeFromDatabaseRequest{
		ScriptName: "test",
	}); err == nil {
		log.Infof("rsp: %s", rsp)
		if !rsp.NeedHandle {
			t.Fatal("need handle should be true")
		}

		if rsp.Code == "" {
			t.Fatal("code should not be empty")
		}

	} else {
		t.Fatal(err)
	}
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
					TypeVerbose: "typeVerbose",
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
					TypeVerbose: "TYPE",
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

		M := cveresources.GetManager(tempFp.Name())
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
		got := riskInfo2grpc([]*information.RiskInfo{
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
