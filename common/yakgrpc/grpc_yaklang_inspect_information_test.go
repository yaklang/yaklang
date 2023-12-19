package yakgrpc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func yaklangInspectInformationSend(client ypb.YakClient, yakScriptType, code string, r *ypb.Range) *ypb.YaklangInspectInformationResponse {
	rsp, err := client.YaklangInspectInformation(context.Background(), &ypb.YaklangInspectInformationRequest{
		YakScriptType: yakScriptType,
		YakScriptCode: code,
		Range:         r,
	})
	if err != nil {
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

func TestGRPCMUSTPASS_LANGUAGE_InspectInformation(t *testing.T) {
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
