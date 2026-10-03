package yakgrpc

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// Old RPCs must reject immediately, without waiting for start parameters or
// constructing an AI runtime. Both errors travel over the existing protobuf RPCs.
func TestDeprecatedAIExecutionRPCs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	listener := bufconn.Listen(1024 * 1024)
	defer listener.Close()
	server := grpc.NewServer()
	ypb.RegisterYakServer(server, &Server{})
	defer server.Stop()
	go server.Serve(listener)
	conn, err := grpc.DialContext(ctx, "passthrough:///deprecated-ai", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	client := ypb.NewYakClient(conn)

	for _, test := range []struct {
		name string
		recv func() error
	}{
		{"StartAITask", func() error {
			stream, err := client.StartAITask(ctx)
			if err != nil {
				return err
			}
			_, err = stream.Recv()
			return err
		}},
		{"StartAITriage", func() error {
			stream, err := client.StartAITriage(ctx)
			if err != nil {
				return err
			}
			_, err = stream.Recv()
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.recv()
			require.Equal(t, codes.Unimplemented, grpcstatus.Code(err))
			require.Equal(t, test.name+" is deprecated; use StartAIReAct.", grpcstatus.Convert(err).Message())
		})
	}
}
