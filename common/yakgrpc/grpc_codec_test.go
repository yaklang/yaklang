package yakgrpc

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_COMMON_CODEC_AUTODECODE(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := client.AutoDecode(utils.TimeoutContextSeconds(1), &ypb.AutoDecodeRequest{Data: `MTI3LjAuMC4xICAgbG9jYWxob3N0IGxvY2FsaG9zdC5sb2NhbGRvbWFpbiBsb2NhbGhvc3Q0IGxvY2FsaG9zdDQubG9jYWxkb21haW40Cjo6MSAgICAgICAgIGxvY2FsaG9zdCBsb2NhbGhvc3QubG9jYWxkb21haW4gbG9jYWxob3N0NiBsb2NhbGhvc3Q2LmxvY2FsZG9tYWluNgoxMC4xOC4zLjEzNyBob3N0LTEwLTE4LTMtMTM3CjEwLjE4LjMuNzcgeXVubmFuCjEwLjE4LjMuMjU0IG1hc3Rlcgo=`})
	if err != nil {
		panic(err)
	}
	var check = false
	for _, r := range rsp.GetResults() {
		if strings.Contains(string(r.Result), `ocalhost.localdoma`) {
			check = true
		}
	}
	if !check {
		t.Fatal("AUTO DECODE BASE64 SMOKING TEST FAILED")
	}
}

func TestGRPCMUSTPASS_COMMON_CODEC_Filetag(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	fp, err := consts.TempFile("test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	token := utils.RandStringBytes(10)
	fp.WriteString("asdfasdfas\r\nabc\r\n" + token)
	fp.Close()

	rsp, err := client.Codec(context.Background(), &ypb.CodecRequest{
		Text: "{{file:line(" + fp.Name() + ")}}",
		Type: "fuzz",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.GetResult(), token) {
		t.Fatal("filetag codec fail")
	}
}

func TestGRPCNewCodec(t *testing.T) {
	workFlow := []*ypb.CodecWork{
		{
			CodecType:  "HtmlEncode",
			Script:     "",
			PluginName: "",
			Params: []*ypb.ExecParamItem{
				{
					Key:   "entityRef",
					Value: "named",
				},
				{
					Key:   "fullEncode",
					Value: "false",
				},
			},
		},
	}
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := client.NewCodec(utils.TimeoutContextSeconds(1),
		&ypb.CodecRequestFlow{
			Text:       "<a>",
			Auto:       false,
			WorkFlow:   workFlow,
			InputBytes: nil,
		},
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(rsp.GetResult())
	if rsp.GetResult() != "&lt;a&gt;" {
		t.Fatal("workflow codec fail")
	}
}

func TestGRPCNewCodec_YakScript(t *testing.T) {
	workFlow := []*ypb.CodecWork{
		{
			CodecType: "CustomCodecPlugin",
			Params: []*ypb.ExecParamItem{
				{
					Key: "pluginContent",
					Value: `
handle = func(origin /*string*/) {
    if type(origin).String() != "string"{
		println(type(origin))
        return "no"
    }
    return "ok"
}
`,
				},
			},
		},
	}
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := client.NewCodec(utils.TimeoutContextSeconds(1),
		&ypb.CodecRequestFlow{
			Text:       "text",
			Auto:       false,
			WorkFlow:   workFlow,
			InputBytes: nil,
		},
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(rsp.GetResult())
	if rsp.GetResult() != "ok" {
		t.Fatal("check yak script input type fail")
	}
}

func TestGRPCNewCodec_Find(t *testing.T) {
	workFlow := []*ypb.CodecWork{
		{
			CodecType: "Find",
			Params: []*ypb.ExecParamItem{
				{
					Key:   "find",
					Value: "a.*",
				}, {
					Key:   "findType",
					Value: "regexp",
				}, {
					Key:   "Global",
					Value: "",
				}, {
					Key:   "Multiline",
					Value: "",
				}, {
					Key:   "IgnoreCase",
					Value: "",
				},
			},
		},
	}
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := client.NewCodec(utils.TimeoutContextSeconds(1),
		&ypb.CodecRequestFlow{
			Text:       "acccccc",
			Auto:       false,
			WorkFlow:   workFlow,
			InputBytes: nil,
		},
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(rsp.GetResult())
	if rsp.GetResult() != "acccccc" {
		t.Fatal("check find method fail")
	}
	client, err = NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err = client.NewCodec(utils.TimeoutContextSeconds(1),
		&ypb.CodecRequestFlow{
			Text:       "cccccc",
			Auto:       false,
			WorkFlow:   workFlow,
			InputBytes: nil,
		},
	)
	if err != nil {
		panic(err)
	}
	if rsp.GetResult() != "" {
		t.Fatal("check find method fail")
	}
}

func TestGRPCNewCodec_Replace(t *testing.T) {
	workFlow := []*ypb.CodecWork{
		{
			CodecType: "Replace",
			Params: []*ypb.ExecParamItem{
				{
					Key:   "find",
					Value: "a.*",
				},
				{
					Key:   "replace",
					Value: "c",
				}, {
					Key:   "findType",
					Value: "regexp",
				}, {
					Key:   "Global",
					Value: "true",
				}, {
					Key:   "Multiline",
					Value: "",
				}, {
					Key:   "IgnoreCase",
					Value: "",
				},
			},
		},
	}
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := client.NewCodec(utils.TimeoutContextSeconds(1),
		&ypb.CodecRequestFlow{
			Text: `abc
acb
aaa
bbb
`,
			Auto:       false,
			WorkFlow:   workFlow,
			InputBytes: nil,
		},
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(rsp.GetResult())
	if rsp.GetResult() != `c
c
c
bbb
` {
		t.Fatal("check replace method fail")
	}
	client, err = NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err = client.NewCodec(utils.TimeoutContextSeconds(1),
		&ypb.CodecRequestFlow{
			Text:       "cccccc",
			Auto:       false,
			WorkFlow:   workFlow,
			InputBytes: nil,
		},
	)
	if err != nil {
		panic(err)
	}
	if rsp.GetResult() != "cccccc" {
		t.Fatal("check find method fail")
	}
}

func TestGRPCCodecFlowFuzztag(t *testing.T) {
	workFlow := []*ypb.CodecWork{
		{
			CodecType:  "Base64Encode",
			Script:     "",
			PluginName: "",
			Params: []*ypb.ExecParamItem{
				{
					Key:   "Alphabet",
					Value: "standard",
				},
			},
		},
		{
			CodecType:  "URLEncode",
			Script:     "",
			PluginName: "",
			Params: []*ypb.ExecParamItem{
				{
					Key:   "fullEncode",
					Value: "true",
				},
			},
		},
	}
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	flowName := utils.RandStringBytes(10)
	_, err = client.SaveCodecFlow(utils.TimeoutContextSeconds(1),
		&ypb.CustomizeCodecFlow{
			FlowName: flowName,
			WorkFlow: workFlow,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	data := utils.RandStringBytes(10)
	expected := codec.EncodeUrlCode(codec.EncodeBase64(data))

	res, err := mutate.FuzzTagExec("{{codecflow(" + flowName + "|" + data + ")}}")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 {
		t.Fatal("fuzztag exec failed")
	}
	require.Equal(t, expected, res[0])
}

func TestGRPCCodecFlow(t *testing.T) {
	workFlow := []*ypb.CodecWork{
		{
			CodecType:  "Base64Decode",
			Script:     "",
			PluginName: "",
			Params: []*ypb.ExecParamItem{
				{
					Key:   "Alphabet",
					Value: "standard",
				},
			},
		},
	}
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	codeData := "\u202etest\x00\n\xff你好"
	expectViewData := "test\n\\xff你好"

	rsp, err := client.NewCodec(utils.TimeoutContextSeconds(1),
		&ypb.CodecRequestFlow{
			Text:       codec.EncodeBase64(codeData),
			Auto:       false,
			WorkFlow:   workFlow,
			InputBytes: nil,
		},
	)
	if err != nil || rsp == nil {
		t.Fatal(err)
	}

	require.Equal(t, codeData, string(rsp.GetRawResult()), "rawRes decode error")
	require.Equal(t, true, rsp.GetIsFalseAppearance(), "IsFalseAppearance check error")
	require.Equal(t, expectViewData, rsp.GetResult(), "result check error")

}
