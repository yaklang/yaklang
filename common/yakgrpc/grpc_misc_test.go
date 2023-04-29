package yakgrpc

import (
	"context"
	"yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestServer_ResetAndInvalidUserData(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	c.ResetAndInvalidUserData(context.Background(), &ypb.ResetAndInvalidUserDataRequest{})
}
