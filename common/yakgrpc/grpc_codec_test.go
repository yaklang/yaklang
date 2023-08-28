package yakgrpc

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
)

func TestGRPCMUSTPASS_CODEC_AUTODECODE(t *testing.T) {
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

func TestGRPCNewCodec(t *testing.T) {
	workFlow := []*ypb.CodecWork{
		{
			CodecType:  "base64",
			Script:     "",
			PluginName: "",
			Params:     nil,
		},
		{
			CodecType:  "base64-decode",
			Script:     "",
			PluginName: "",
			Params:     nil,
		},
		{
			CodecType: "custom-script",
			Script: `
handle = func(origin) {
    return origin + "test"
}
`,
			PluginName: "",
			Params:     nil,
		},
	}
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := client.NewCodec(utils.TimeoutContextSeconds(1),
		&ypb.CodecRequestFlow{
			Text:       "test",
			Auto:       false,
			WorkFlow:   workFlow,
			InputBytes: nil,
		},
	)
	if err != nil {
		panic(err)
	}
	if !(rsp.GetResult() == "testtest") {
		t.Fatal("workflow codec fail")
	}
}
