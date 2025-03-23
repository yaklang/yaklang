package yakgrpc

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestAITask(t *testing.T) {
	if utils.InGithubActions() {
		return
	}

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx := utils.TimeoutContextSeconds(60)
	stream, err := client.StartAITask(ctx)
	if err != nil {
		t.Fatal(err)
	}

	temp := consts.TempFileFast("1+1")
	spew.Dump(temp)

	stream.Send(&ypb.AIInputEvent{
		IsStart: true,
		Params: &ypb.AIStartParams{
			UserQuery:                      "打开" + temp + "计算里面的表达式",
			EnableSystemFileSystemOperator: true,
			UseDefaultAIConfig:             true,
		},
	})

	for {
		event, err := stream.Recv()
		if err != nil {
			break
		}
		if event.IsStream {
			continue
		}
		fmt.Println(event.String())
	}
}
