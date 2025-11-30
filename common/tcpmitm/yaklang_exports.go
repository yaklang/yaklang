package tcpmitm

import (
	"context"
	"net"
	"reflect"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// Exports provides yaklang bindings for tcpmitm functionality
// Style follows mitm package conventions: lowercase option functions, uppercase entry points
var Exports = map[string]interface{}{
	// Main entry point
	"Start": _start,

	// Option functions (lowercase, following mitm style)
	"context":            _withContext,
	"dialer":             _withDialer,
	"timeGapThreshold":   _withTimeGapThreshold,
	"maxBufferSize":      _withMaxBufferSize,
	"hijackTCPFrame":     _hijackTCPFrame,
	"hijackTCPConn":      _hijackTCPConn,
	"protocolAwareSplit": _withProtocolAwareSplit,
}

// _start creates and starts a TCPMitm from a connection channel.
// This is the main entry point for tcpmitm in yaklang.
// Usage:
// ```
//
//	mitm, err = tcpmitm.Start(connChan, tcpmitm.hijackTCPFrame(func(flow, frame) {
//	    println(flow.String())
//	    frame.Forward()
//	}))
//
// err = mitm.Run()
// ```
func _start(ch interface{}, opts ...Option) (*TCPMitm, error) {
	if ch == nil {
		return nil, utils.Error("connection channel cannot be nil")
	}

	// Check if it's already chan net.Conn
	if netConnChan, ok := ch.(chan net.Conn); ok {
		return LoadConnectionChannel(netConnChan, opts...)
	}

	// Handle yaklang chan any type using reflection
	chValue := reflect.ValueOf(ch)
	if chValue.Kind() != reflect.Chan {
		return nil, utils.Errorf("expected a channel, got %T", ch)
	}

	// Create a bridge channel
	bridgeChan := make(chan net.Conn, 1024)

	// Start goroutine to bridge the channels
	go func() {
		for {
			recv, ok := chValue.Recv()
			if !ok {
				log.Info("tcpmitm: source channel closed")
				close(bridgeChan)
				return
			}

			if recv.IsNil() {
				continue
			}

			if conn, ok := recv.Interface().(net.Conn); ok {
				bridgeChan <- conn
			} else {
				log.Warnf("tcpmitm: received non-net.Conn value from channel: %T", recv.Interface())
			}
		}
	}()

	return LoadConnectionChannel(bridgeChan, opts...)
}

// _withContext sets a custom context for the TCPMitm instance.
// Usage: tcpmitm.Start(ch, tcpmitm.context(ctx))
func _withContext(ctx context.Context) Option {
	return WithContext(ctx)
}

// _withDialer sets a custom dialer for connecting to real servers.
// Usage: tcpmitm.Start(ch, tcpmitm.dialer(func(addr) { return net.Dial("tcp", addr) }))
func _withDialer(dialer func(addr string) (net.Conn, error)) Option {
	return WithDialer(dialer)
}

// _withTimeGapThreshold sets the time gap threshold for frame splitting.
// Common values: 50ms, 100ms, 200ms, 300ms
// Usage: tcpmitm.Start(ch, tcpmitm.timeGapThreshold(100 * time.Millisecond))
func _withTimeGapThreshold(d time.Duration) Option {
	return WithTimeGapThreshold(d)
}

// _withMaxBufferSize sets the maximum buffer size before forcing a frame split.
// Default is 8KB.
// Usage: tcpmitm.Start(ch, tcpmitm.maxBufferSize(16 * 1024))
func _withMaxBufferSize(size int) Option {
	return WithMaxBufferSize(size)
}

// _withProtocolAwareSplit enables protocol-aware frame splitting.
// Usage: tcpmitm.Start(ch, tcpmitm.protocolAwareSplit(true))
func _withProtocolAwareSplit(enable bool) Option {
	return WithProtocolAwareSplit(enable)
}

// _hijackTCPFrame sets the callback for frame-level hijacking.
// The callback receives flow info and frame for each data segment.
// Usage:
// ```
//
//	tcpmitm.Start(ch, tcpmitm.hijackTCPFrame(func(flow, frame) {
//	    data = frame.GetRawBytes()
//	    if frame.GetDirection() == 0 { // client -> server
//	        println("Client sent:", len(data), "bytes")
//	    }
//	    frame.Forward() // or frame.Drop() to block
//	}))
//
// ```
func _hijackTCPFrame(callback FrameHijackCallback) Option {
	return func(m *TCPMitm) {
		m.SetHijackTCPFrame(callback)
	}
}

// _hijackTCPConn sets the callback for connection-level hijacking.
// This callback is invoked when a new connection is established.
// Usage:
// ```
//
//	tcpmitm.Start(ch, tcpmitm.hijackTCPConn(func(conn, operator) {
//	    println("New connection:", operator.GetFlow().String())
//	    // operator.Hold() - take control of connection
//	    // operator.CloseHijackedConn() - close connection
//	}))
//
// ```
func _hijackTCPConn(callback ConnHijackCallback) Option {
	return func(m *TCPMitm) {
		m.SetHijackTCPConn(callback)
	}
}
