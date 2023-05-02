package yakgrpc

import (
	"context"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"net"
	"testing"
	"time"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

func NewLocalClient() (ypb.YakClient, error) {
	port := utils.GetRandomAvailableTCPPort()
	addr := utils.HostPort("127.0.0.1", port)
	grpcTrans := grpc.NewServer(
		grpc.MaxRecvMsgSize(100*1024*1024),
		grpc.MaxSendMsgSize(100*1024*1024),
	)
	s, err := NewServer()
	if err != nil {
		log.Errorf("build yakit server failed: %s", err)
		return nil, err
	}
	ypb.RegisterYakServer(grpcTrans, s)
	var lis net.Listener
	lis, err = net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	go func() {
		err = grpcTrans.Serve(lis)
		if err != nil {
			log.Error(err)
		}
	}()

	time.Sleep(1 * time.Second)

	conn, err := grpc.Dial(addr, grpc.WithInsecure(), grpc.WithDefaultCallOptions(
		grpc.MaxCallRecvMsgSize(100*1024*1045),
		grpc.MaxCallRecvMsgSize(100*1024*1045),
	))
	if err != nil {
		return nil, err
	}
	return ypb.NewYakClient(conn), nil
}

func TestNewServer(t *testing.T) {
	test := assert.New(t)

	client, err := NewLocalClient()
	if err != nil {
		test.FailNow(err.Error())
		return
	}

	stream, err := client.ExecBatchYakScript(context.Background(), &ypb.ExecBatchYakScriptRequest{
		Target:              "localhost",
		Keyword:             "struts",
		Limit:               10,
		TotalTimeoutSeconds: 10,
		Type:                "nuclei",
		Concurrent:          4,
	})
	if err != nil {
		test.FailNow(err.Error())
		return
	}

	stream.Recv()
	stream.Recv()
	stream.Recv()
}
