package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
)

func TestYakitLog(t *testing.T) {
	code := `
yakit.AutoInitYakit()
yakit.Info(codec.EncodeBase64("Hello Yak"))
`
	var client, err = NewLocalClient()
	stream, err := client.Exec(context.Background(), &ypb.ExecRequest{
		Script:          code,
		NoDividedEngine: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	flag := "SGVsbG8gWWFr"

	out := ""
	for {
		res, err := stream.Recv()
		if err != nil {
			break
		}
		spew.Dump(string(res.Message))
		out += string(res.Message)
	}
	if strings.Contains(out, flag) {
		t.Log("success")
	} else {
		t.Fatal("failed")
	}
}
