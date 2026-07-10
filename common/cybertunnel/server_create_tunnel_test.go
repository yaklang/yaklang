package cybertunnel

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"google.golang.org/grpc/metadata"
)

func TestMUSTPASS_DispatchTunnelInputSkipsClosedReaderAndKeepsTunnelAlive(t *testing.T) {
	desc := &tunnelDesc{
		Network:     "tcp",
		Connections: new(sync.Map),
	}

	closedLocal, closedPeer := net.Pipe()
	defer closedPeer.Close()
	closedReader := make(chan *tpb.TunnelInput)
	close(closedReader)
	desc.Connections.Store("closed-remote", &connectionDesc{
		Connection: closedLocal,
		RemoteAddr: "closed-remote",
		Reader:     closedReader,
	})

	if dispatchTunnelInputToTCPConnection(context.Background(), desc, &tpb.TunnelInput{
		ToRemoteAddr: "closed-remote",
		Data:         []byte("late packet"),
	}) {
		t.Fatal("closed connection should not be handled")
	}
	if _, ok := desc.Connections.Load("closed-remote"); ok {
		t.Fatal("closed connection should be removed after a late packet")
	}

	openLocal, openPeer := net.Pipe()
	defer openLocal.Close()
	defer openPeer.Close()
	openReader := make(chan *tpb.TunnelInput, 1)
	desc.Connections.Store("open-remote", &connectionDesc{
		Connection: openLocal,
		RemoteAddr: "open-remote",
		Reader:     openReader,
	})

	if !dispatchTunnelInputToTCPConnection(context.Background(), desc, &tpb.TunnelInput{
		ToRemoteAddr: "open-remote",
		Data:         []byte("next packet"),
	}) {
		t.Fatal("open connection should be handled after a closed connection is skipped")
	}

	select {
	case got := <-openReader:
		if string(got.GetData()) != "next packet" {
			t.Fatalf("unexpected forwarded data: %q", got.GetData())
		}
	case <-time.After(time.Second):
		t.Fatal("open connection did not receive forwarded packet")
	}
}

type failingCreateTunnelClient struct {
	ctx context.Context
}

func (f *failingCreateTunnelClient) Send(*tpb.TunnelInput) error {
	return nil
}

func (f *failingCreateTunnelClient) Recv() (*tpb.TunnelOutput, error) {
	return nil, errors.New("recv failed")
}

func (f *failingCreateTunnelClient) Header() (metadata.MD, error) {
	return nil, nil
}

func (f *failingCreateTunnelClient) Trailer() metadata.MD {
	return nil
}

func (f *failingCreateTunnelClient) CloseSend() error {
	return nil
}

func (f *failingCreateTunnelClient) Context() context.Context {
	return f.ctx
}

func (f *failingCreateTunnelClient) SendMsg(any) error {
	return nil
}

func (f *failingCreateTunnelClient) RecvMsg(any) error {
	return nil
}

func TestMUSTPASS_HoldingCreateTunnelClientReturnsWhenRecvLoopFails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HoldingCreateTunnelClient(&failingCreateTunnelClient{ctx: ctx}, "127.0.0.1", 1, 2, "test-tunnel")
	}()

	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("HoldingCreateTunnelClient should return when the recv loop exits")
	}
}
