package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestServer_SavePayload(t *testing.T) {
	test := assert.New(t)

	client, err := NewLocalClient()
	if err != nil {
		test.FailNow(err.Error())
	}

	s, err := client.SavePayload(context.Background(), &ypb.SavePayloadRequest{
		Group:   "test",
		Content: "asdfasdf\nasdfasdfas\nasdfasdf111121313q4t14",
	})
	if err != nil {
		test.FailNow(err.Error())
	}

	_ = s

	rsp, err := client.QueryPayload(context.Background(), &ypb.QueryPayloadRequest{
		Group: "test",
	})
	if err != nil {
		test.FailNow(err.Error())
		return
	}
	if len(rsp.Data) != 3 {
		test.FailNow("error to test pyaload")
	}
	spew.Dump(rsp.Data)

	rsp1, err := client.GetAllPayloadGroup(context.Background(), &ypb.Empty{})
	if err != nil {
		test.FailNow(err.Error())
		return
	}
	spew.Dump(rsp1)
	if len(rsp1.Groups) <= 0 {
		test.FailNow("no results")
	}

	//client.DeletePayloadByGroup(context.Background(), &ypb.DeletePayloadByGroupRequest{Group: "test"})
}
