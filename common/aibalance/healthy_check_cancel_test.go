// healthy_check_cancel_test.go - 健康检查可取消回归测试
//
// 覆盖抗坍塌修复: 健康检查 client 注入可取消 ctx, 上游卡死时 ctx 到点能取消底层
// HTTP 请求, 使 Chat goroutine 及时返回而不泄漏.
//
// 关键词: 健康检查 ctx 取消回归, 上游卡死不泄漏, GetAIClientForHealthCheck

package aibalance

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHealthCheckClientCancelOnHang 上游 accept 后永不响应, 注入 800ms ctx 的
// 健康检查 client.Chat 应在 ctx 到点后及时返回错误, 而不是一直挂着 (杜绝泄漏).
func TestHealthCheckClientCancelOnHang(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	// 卡死上游: 接受连接但永不回包.
	var held []net.Conn
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			held = append(held, conn) // 持有, 不读不写不关
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	p := &Provider{
		TypeName:    "openai",
		ModelName:   "health-test",
		DomainOrURL: fmt.Sprintf("127.0.0.1:%d", addr.Port),
		NoHTTPS:     true,
		APIKey:      "sk-health-test",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()

	client, err := p.GetAIClientForHealthCheck(ctx, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, client)

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, chatErr := client.Chat("ping")
		done <- chatErr
	}()

	select {
	case chatErr := <-done:
		elapsed := time.Since(start)
		assert.Error(t, chatErr, "Chat against a hanging upstream should error out via ctx cancel")
		assert.Less(t, elapsed, 8*time.Second,
			"ctx cancel should make Chat return promptly, not hang; took %v", elapsed)
	case <-time.After(10 * time.Second):
		t.Fatal("Chat did not return after ctx deadline; underlying HTTP not cancelled (goroutine leak)")
	}

	for _, c := range held {
		_ = c.Close()
	}
}
