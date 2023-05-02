package cybertunnel

import (
	"context"
	"google.golang.org/grpc"
	"net"
	"testing"
	"time"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/utils"
)

func testServer() (tpb.TunnelClient, tpb.TunnelServer) {
	s, err := NewTunnelServer()
	if err != nil {
		panic(err)
	}

	trans := grpc.NewServer()
	tpb.RegisterTunnelServer(trans, s)

	port := utils.GetRandomAvailableTCPPort()
	lis, err := net.Listen("tcp", utils.HostPort("127.0.0.1", port))
	go func() {
		err := trans.Serve(lis)
		if err != nil {
			panic(err)
		}
	}()
	time.Sleep(time.Second)

	conn, err := grpc.Dial(
		utils.HostPort("127.0.0.1", port),
		grpc.WithInsecure(),
		grpc.WithNoProxy(),
	)
	if err != nil {
		panic(err)
	}
	client := tpb.NewTunnelClient(conn)
	return client, s
}

func TestMirrorLocalPortToRemote(t *testing.T) {
	client, server := testServer()
	_ = server
	stream, err := client.CreateTunnel(context.Background())
	if err != nil {
		return
	}

	mPort := utils.GetRandomAvailableTCPPort()
	stream.Send(&tpb.TunnelInput{
		Mirrors: []*tpb.Mirror{
			{
				Id:      "abc",
				Port:    int32(mPort),
				Network: "tcp",
			},
		},
	})
}
