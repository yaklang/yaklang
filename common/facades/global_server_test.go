package facades

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"testing"
	"time"
)

// TestGlobalServer check ctx is valid
func TestGlobalServer(t *testing.T) {
	t.SkipNow()

	ctx, cancel := context.WithCancel(context.Background())
	port := utils.GetRandomAvailableTCPPort()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	server := NewFacadeServer("127.0.0.1", port)
	go func() { server.Serve() }()
	RegisterFacadeServer(ctx, "aaa", server)
	err := utils.WaitConnect(addr, 3)
	if err != nil {
		t.Fatal(err)
	}
	server = GetFacadeServer("aaa")
	assert.NotEqual(t, server, nil)
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("connect to server error: %v", err)
	}
	conn.Close()

	cancel()
	time.Sleep(time.Millisecond * 500)
	server = GetFacadeServer("aaa")
	if server != nil {
		t.Fatal("expect nil server")
	}
	conn, err = netx.DialTimeout(time.Second, "127.0.0.1:8067")
	if conn != nil {
		t.Fatal("expect nil connect")
	}
}
