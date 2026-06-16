package harness

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Client wraps the ypb.YakClient for eval harness usage.
type Client struct {
	conn   *grpc.ClientConn
	client ypb.YakClient
	addr   string
}

// NewClient connects to a yaklang gRPC server at the given address.
func NewClient(addr string) (*Client, error) {
	conn, err := grpc.Dial(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)),
	)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	return &Client{
		conn:   conn,
		client: ypb.NewYakClient(conn),
		addr:   addr,
	}, nil
}

// NewLocalClient connects to a local yaklang gRPC server.
func NewLocalClient() (*Client, error) {
	return NewClient("127.0.0.1:8087")
}

// Raw returns the underlying ypb.YakClient for direct gRPC calls.
func (c *Client) Raw() ypb.YakClient {
	return c.client
}

// Conn returns the underlying gRPC connection.
func (c *Client) Conn() *grpc.ClientConn {
	return c.conn
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// CheckHealth verifies the gRPC server is reachable by listing available tools.
func (c *Client) CheckHealth(ctx context.Context) error {
	_, err := c.client.GetAIToolList(ctx, &ypb.GetAIToolListRequest{})
	return err
}

// IsPortOpen checks if a port is listening.
func IsPortOpen(addr string) bool {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
