package yakgrpc

import (
	"context"
	"reflect"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func send(client ypb.YakClient, typ, code string, startPos, endPos *ypb.Position) *YaklangInformationResponse {
	rsp, err := client.YaklangInspectInformation(context.Background(), &ypb.YaklangInspectInformationRequest{
		YakScriptType: typ,
		YakScriptCode: code,
		StartPos:      startPos,
		EndPos:        endPos,
	})
	if err != nil {
		return nil
	}
	// fmt.Println(rsp)
	// from rsp to rspStruct
	rspStruct, err := fromGrpcModuleToYaklangInformationResponse(rsp)
	if err != nil {
		return nil
	} else {
		return rspStruct
	}
}

func TestYaklangInspectInformation_Cli(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	check := func(code string, cliLs []*CliParameter) {
		rsp := send(client, "", code, nil, nil)
		if rsp == nil {
			t.Fatal("local client error")
		}
		if len(rsp.CliParameter) != len(cliLs) {
			t.Fatalf("cli parameter length error: got(%d) vs want(%d)", len(rsp.CliParameter), len(cliLs))
		}

		for i := 0; i < len(cliLs); i++ {
			gotItem := rsp.CliParameter[i]
			wantItem := cliLs[i]
			rv := reflect.ValueOf(gotItem.Default)
			rTyp := reflect.TypeOf(wantItem.Default)
			if !rv.CanConvert(rTyp) {
				t.Fatalf("cli parameter Default type error: got(%v) vs want(%v)", gotItem, wantItem)
			}
			gotDefault := rv.Convert(rTyp).Interface()
			if gotDefault != wantItem.Default {
				t.Fatalf("cli parameter Default value error: got(%v) vs want(%v)", gotItem, wantItem)
			}
			if gotItem.Help != wantItem.Help {
				t.Fatalf("cli parameter Help error: got(%v) vs want(%v)", gotItem, wantItem)
			}
			if gotItem.Name != wantItem.Name {
				t.Fatalf("cli parameter Name error: got(%v) vs want(%v)", gotItem, wantItem)
			}
			if gotItem.Required != wantItem.Required {
				t.Fatalf("cli parameter Required error: got(%v) vs want(%v)", gotItem, wantItem)
			}
			if gotItem.Type != wantItem.Type {
				t.Fatalf("cli parameter Type error: got(%v) vs want(%v)", gotItem, wantItem)
			}
		}
	}

	t.Run("basic cli", func(t *testing.T) {
		check(
			`
		cli.String("arg1", cli.setDefault("default variable"), cli.setHelp("help information"), cli.setRequired(true))
		cli.Int("arg2", cli.setDefault(1), cli.setHelp("help information 2"))
		`, []*CliParameter{
				{
					Name:     "arg1",
					Type:     "string",
					Help:     "help information",
					Required: true,
					Default:  "default variable",
				},
				{
					Name:     "arg2",
					Type:     "int",
					Help:     "help information 2",
					Required: false,
					Default:  1,
				},
			})
	})
}
