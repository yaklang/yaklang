package yakgrpc

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// import (
// 	"context"
// 	"reflect"
// 	"testing"

// 	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
// )

func yaklangInspectInformationSend(client ypb.YakClient, inspectType, yakScriptType, code string, r *ypb.Range) *ypb.YaklangInspectInformationResponse {
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

func CheckKV(t *testing.T, wants map[string]string, infos []*ypb.YaklangInformation) {
	if len(wants) != len(infos) {
		t.Fatal("length of kvs and infos not match")
	}

	compare := func(s1, s2 string) bool {
		s1 = strings.ReplaceAll(s1, "\n", "")
		s1 = strings.ReplaceAll(s1, " ", "")
		s2 = strings.ReplaceAll(s2, "\n", "")
		s2 = strings.ReplaceAll(s2, " ", "")
		return s1 == s2
	}
	for _, info := range infos {
		if want, ok := wants[info.Name]; ok {
			if compare(want, info.String()) {
				t.Fatalf("want (%s) vs got (%s)", want, info.String())
			}
		} else {
			t.Fatalf("key %s in got, but not in want", info.Name)
		}
	}
}

func TestYakGetInfo(t *testing.T) {
	local, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	rsp := yaklangInspectInformationSend(local, "info", "yak",
		`
		cli.String("arg1", cli.setDefault("default variable"), cli.setHelp("help information"), cli.setRequired(true))
		cli.Int("arg2", cli.setDefault(1), cli.setHelp("help information 2"))

		risk.NewRisk(
				url,
				risk.title(sprintf("XSS for: %v", url)),
				risk.titleVerbose(sprintf("检测到xss漏洞: %v", url)),
				risk.details("report"),
				risk.description("description info "),
				risk.solution("solution info"),
				risk.type("xss"),
				risk.payload("payloadString"),
				risk.request("reqRaw"),
				risk.response("respRaw"),
				risk.severity("warning"),
			)
	`,
		nil)
	if rsp == nil {
		t.Fatal("no response")
	}

	CheckKV(t, map[string]string{
		"cli": `
		Name:"cli" 
				Data:{
					Key:"Name" Value:"\"arg1\"" 
					Extern:{Key:"Type" Value:"\"string\""} 
					Extern:{Key:"Help" Value:"\"help information\""} 
					Extern:{Key:"Required" Value:"false"} 
					Extern:{Key:"Default" Value:"\"default variable\""}
				} 
				Data:{
					Key:"Name" Value:"\"arg2\"" 
					Extern:{Key:"Type" Value:"\"int\""} 
					Extern:{Key:"Help" Value:"\"help information 2\""} 
					Extern:{Key:"Required" Value:"false"} 
					Extern:{Key:"Default" Value:"1"}
				}`,
		"risk": `
		Name:"risk" 
			Data:{
				Key:"Name" Value:"\"risk\"" 
				Extern:{Key:"Level" Value:"\"warning\""} 
				Extern:{Key:"CVE" Value:"\"\""} 
				Extern:{Key:"Type" Value:"\"xss\""} 
				Extern:{Key:"TypeVerbose" Value:"\"XSS\""}
			}`,
	}, rsp.Information)
}
