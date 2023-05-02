package yakgrpc

import (
	"context"
	"testing"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

func TestServer_ResetAndInvalidUserData(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	c.ResetAndInvalidUserData(context.Background(), &ypb.ResetAndInvalidUserDataRequest{})
}
