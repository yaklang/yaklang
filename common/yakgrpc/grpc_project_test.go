package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestGetTemporaryProject(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := client.GetTemporaryProject(context.Background(), &ypb.Empty{})
	if err != nil {
		panic(err)
	}
	_ = rsp
}
