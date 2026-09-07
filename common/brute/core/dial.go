package core

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"
)

// dialError 与旧 bruteutils.dialError 对应，便于上层按错误类别分类。
type dialError struct{ reason string }

func (e *dialError) Error() string { return e.reason }

// IsDialError 判断错误是否来自拨号阶段。
func IsDialError(err error) bool {
	_, ok := err.(*dialError)
	return ok
}

// DefaultTimeout 是单次探测的默认总超时。
const DefaultTimeout = 10 * time.Second

// tlsConfig 本模块默认跳过证书校验（业务定位是弱口令检测，证书有效性不参与判定），
// 但 TLS 是否启用由 TLSPolicy 显式控制。
func tlsConfig() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- 弱口令探测场景刻意跳过证书校验
}

// Dialer 按显式 TLS 策略建立连接。
//
// 与旧实现（TLS 失败静默回退明文）不同：
//   - TLSStrict: TLS 握手失败返回错误，绝不发送明文凭证；
//   - TLSOpportunistic: TLS 失败回退明文，但通过 transport 返回值如实暴露，
//     不再是静默降级；
//   - PlaintextAllowed: 直接明文。
func Dialer(ctx context.Context, address string, policy TLSPolicy, timeout time.Duration) (conn net.Conn, transport Transport, err error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var d net.Dialer
	rawConn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, TransportUnknown, &dialError{reason: fmt.Sprintf("dial tcp %s: %v", address, err)}
	}

	if policy == PlaintextAllowed {
		return rawConn, TransportPlainTCP, nil
	}

	// 给 TLS 握手设置基于 ctx 的总期限，取消时握手立即中断。
	tlsConn := tls.Client(rawConn, tlsConfig())
	handshakeCtx, handshakeCancel := context.WithTimeout(ctx, timeout)
	defer handshakeCancel()
	if err := tlsConn.HandshakeContext(handshakeCtx); err != nil {
		// TLS 失败必须关闭底层连接，避免句柄泄漏。
		_ = rawConn.Close()
		if policy == TLSStrict {
			return nil, TransportUnknown, &dialError{reason: fmt.Sprintf("tls handshake with %s failed: %v", address, err)}
		}
		// TLSOpportunistic：新建明文连接重试（原连接已被 TLS 握手污染）。
		plainConn, dialErr := d.DialContext(ctx, "tcp", address)
		if dialErr != nil {
			return nil, TransportUnknown, &dialError{reason: fmt.Sprintf("fallback dial tcp %s: %v", address, dialErr)}
		}
		return plainConn, TransportPlainTCP, nil
	}
	return tlsConn, TransportTLS, nil
}

// SetDeadline 根据 ctx 与超时设置连接读写期限；探针在每次 I/O 前调用，
// 保证取消/超时能让阻塞的读写立即返回（连接随之被探针关闭）。
func SetDeadline(conn net.Conn, ctx context.Context, timeout time.Duration) {
	if conn == nil {
		return
	}
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
}

// WatchConn 监视 ctx：取消时把连接 deadline 设为过去，立即中断阻塞的读写。
// 返回的 unwatch 在探针 I/O 完成后必须调用，避免 goroutine 泄漏。
// 这不是长期 timer goroutine：随单次探测存活，探测结束即回收。
func WatchConn(ctx context.Context, conn net.Conn) (unwatch func()) {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			// deadline 设为过去会让所有阻塞 I/O 立即返回超时错误。
			_ = conn.SetDeadline(time.Now().Add(-time.Second))
		case <-done:
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() { close(done) })
	}
}

// SafeClose 忽略关闭错误，统一关闭路径。
func SafeClose(conn net.Conn) {
	if conn != nil {
		_ = conn.Close()
	}
}
