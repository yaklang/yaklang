package yakgrpc

import (
	"context"
	"fmt"
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
