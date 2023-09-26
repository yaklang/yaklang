package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-rod/rod/lib/utils"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestGRPCMUSTPASS_StringFuzzer(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	token := utils.RandString(10)
	token2 := utils.RandString(20)
	filename := consts.TempFileFast(token, token2)

	result, err := client.StringFuzzer(context.Background(), &ypb.StringFuzzerRequest{
		Template: "{{file:line(" + filename + ")}}",
		Limit:    0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Results[0]) != token || string(result.Results[1]) != token2 {
		t.Fatal("string (filetag) fuzzer fail")
	}

	result, err = client.StringFuzzer(context.Background(), &ypb.StringFuzzerRequest{
		Template: "{{yak(handle1|{{file:line(" + filename + ")}})}}",
		HotPatchCode: `
handle1 = s => {
	return "__" + s + "__"
}
`,
		Limit: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Results[0]) != "__"+token+"__" || string(result.Results[1]) != "__"+token2+"__" {
		t.Fatal("string (filetag) fuzzer fail")
	}

	result, err = client.StringFuzzer(context.Background(), &ypb.StringFuzzerRequest{
		Template: "{{yak(handle1|{{file:line(" + filename + ")}})}}",
		HotPatchCode: `
handle1 = s => {
	return ["__" + s + "__", "__" + s + "__", "__" + s + "__"]
}
`,
		Limit: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 3 {
		spew.Dump(result.Results)
		t.Fatal("string (filetag + hotpatch) fuzzer fail")
	}
}
func TestGRPCMUSTPASS_WithHotPatch(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	for _, itestCase := range []any{
		[]any{
			`handle=(a)=>{
				assert a =="a|b"
				return "ok"
			}`,
			`{{yak(handle|a|b)}}`,
		},
		[]any{
			`handle=(a,b)=>{
				assert a =="a" && b=="b|c"
				return "ok"
			}`,
			`{{yak(handle|a|b|c)}}`,
		},
		[]any{
			`handle=(a,b,c,d)=>{
				assert a =="a" && b=="b" && c=="" && d==""
				return "ok"
			}`,
			`{{yak(handle|a|b)}}`,
		},
		[]any{
			`handle=(params...)=>{
				data = ["a","b","c"]
				for i=0;i<3;i++ {
					assert params[i] == data[i]
				}
				return "ok"
			}`,
			`{{yak(handle|a|b|c)}}`,
		},
	} {
		testCase := itestCase.([]any)
		code := testCase[0].(string)
		template := testCase[1].(string)
		res, err := client.StringFuzzer(context.Background(), &ypb.StringFuzzerRequest{
			Template:     template,
			HotPatchCode: code,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Results) != 1 || string(res.Results[0]) != "ok" {
			t.Fatal(spew.Sprintf("hotpatch fail: %v,%v", template, code))
		}
	}
}
