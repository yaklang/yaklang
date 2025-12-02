package netx

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestProxyWithoutPort_ShouldFail 测试不带端口的代理地址应该报错而不是回退到直连
func TestProxyWithoutPort_ShouldFail(t *testing.T) {
	// 测试：不带端口的代理地址应该报错
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 尝试连接一个公网地址，使用一个不带端口的代理
	conn, err := DialContext(ctx, "example.com:80", "socks5://127.0.0.1")

	// 应该返回错误
	require.Error(t, err, "proxy without port should fail")
	require.Nil(t, conn, "connection should be nil when proxy fails")
	require.Contains(t, err.Error(), "proxy", "error message should mention proxy")
}

// TestProxyWithInvalidAddress_ShouldFail 测试无效的代理地址应该报错
func TestProxyWithInvalidAddress_ShouldFail(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 使用一个无法连接的代理地址
	conn, err := DialContext(ctx, "example.com:80", "socks5://127.0.0.1:65535")

	// 应该返回错误
	require.Error(t, err, "invalid proxy should fail")
	require.Nil(t, conn, "connection should be nil when proxy fails")
}

// TestProxyWithoutPort_UsingDialX 测试 DialX 函数也应该正确处理
func TestProxyWithoutPort_UsingDialX(t *testing.T) {
	// 使用 DialX 函数测试
	conn, err := DialX("example.com:80",
		DialX_WithProxy("socks5://127.0.0.1"), // 不带端口
		DialX_WithTimeout(5*time.Second),
	)

	// 应该返回错误
	require.Error(t, err, "proxy without port should fail in DialX")
	require.Nil(t, conn, "connection should be nil when proxy fails")
	require.Contains(t, err.Error(), "proxy", "error message should mention proxy")
}

// TestMultipleProxies_AllFail_ShouldNotFallbackToDirect 测试多个代理都失败时不应该回退到直连
func TestMultipleProxies_AllFail_ShouldNotFallbackToDirect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 使用多个都无效的代理
	conn, err := DialContext(ctx, "example.com:80",
		"socks5://127.0.0.1",       // 没有端口
		"socks5://127.0.0.1:65535", // 无法连接
		"http://192.0.2.1:8080",    // 不存在的地址
	)

	// 应该返回错误，不应该回退到直连
	require.Error(t, err, "all proxies failed should return error")
	require.Nil(t, conn, "connection should be nil when all proxies fail")
	require.Contains(t, err.Error(), "no proxy available", "error should indicate proxy failure")
}

// TestDialX_WithInvalidProxyFromFixProxy 测试通过 FixProxy 处理的无效代理地址
func TestDialX_WithInvalidProxyFromFixProxy(t *testing.T) {
	// 模拟用户输入只有 IP 没有端口的情况
	invalidProxy := FixProxy("127.0.0.1") // 返回 "127.0.0.1"

	conn, err := DialX("example.com:80",
		DialX_WithProxy(invalidProxy),
		DialX_WithTimeout(5*time.Second),
	)

	// 应该返回错误
	require.Error(t, err, "invalid proxy from FixProxy should fail")
	require.Nil(t, conn, "connection should be nil when proxy fails")
}
