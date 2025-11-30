package tcpmitm

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/stretchr/testify/require"
)

// mockConn implements net.Conn for testing purposes.
type mockConn struct {
	reader     io.Reader
	writer     io.Writer
	localAddr  net.Addr
	remoteAddr net.Addr
	closed     bool
	mu         sync.Mutex
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	return m.reader.Read(b)
}

func (m *mockConn) Write(b []byte) (n int, err error) {
	return m.writer.Write(b)
}

func (m *mockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockConn) LocalAddr() net.Addr {
	return m.localAddr
}

func (m *mockConn) RemoteAddr() net.Addr {
	return m.remoteAddr
}

func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

type mockAddr struct {
	network string
	addr    string
}

func (a *mockAddr) Network() string { return a.network }
func (a *mockAddr) String() string  { return a.addr }

func TestConnectionFlow(t *testing.T) {
	flow := NewConnectionFlow(nil, "192.168.1.1", 12345, "10.0.0.1", 80)

	require.Equal(t, "192.168.1.1", flow.GetClientIP())
	require.Equal(t, 12345, flow.GetClientPort())
	require.Equal(t, "10.0.0.1", flow.GetServerIP())
	require.Equal(t, 80, flow.GetServerPort())
	require.Equal(t, "10.0.0.1:80", flow.GetServerAddr())
	require.Equal(t, "192.168.1.1:12345", flow.GetClientAddr())
	require.Equal(t, "192.168.1.1:12345 -> 10.0.0.1:80", flow.String())
}

func TestFrame(t *testing.T) {
	t.Run("basic operations", func(t *testing.T) {
		data := []byte("hello world")
		frame := NewFrame(data, DirectionClientToServer, nil)

		require.Equal(t, data, frame.GetRawBytes())
		require.Equal(t, DirectionClientToServer, frame.GetDirection())
		require.Equal(t, len(data), frame.Size())
		require.False(t, frame.IsDropped())
		require.False(t, frame.IsModified())
		require.False(t, frame.IsUnknownProtocol())
	})

	t.Run("modify data", func(t *testing.T) {
		data := []byte("hello")
		frame := NewFrame(data, DirectionClientToServer, nil)

		newData := []byte("world")
		frame.SetRawBytes(newData)

		require.Equal(t, newData, frame.GetRawBytes())
		require.True(t, frame.IsModified())
	})

	t.Run("drop frame", func(t *testing.T) {
		frame := NewFrame([]byte("test"), DirectionClientToServer, nil)
		frame.Drop()
		require.True(t, frame.IsDropped())
		require.False(t, frame.shouldForward())
	})

	t.Run("inject data", func(t *testing.T) {
		frame := NewFrame([]byte("test"), DirectionClientToServer, nil)
		frame.Inject([]byte("inject1"))
		frame.Inject([]byte("inject2"))

		queue := frame.getInjectQueue()
		require.Len(t, queue, 2)
		require.Equal(t, []byte("inject1"), queue[0])
		require.Equal(t, []byte("inject2"), queue[1])
	})

	t.Run("unknown protocol flag", func(t *testing.T) {
		frame := NewFrame([]byte{0x00, 0x01, 0x02}, DirectionClientToServer, nil)
		require.False(t, frame.IsUnknownProtocol())

		frame.SetUnknownProtocol(true)
		require.True(t, frame.IsUnknownProtocol())
	})
}

func TestConnOperator(t *testing.T) {
	flow := NewConnectionFlow(nil, "192.168.1.1", 12345, "10.0.0.1", 80)

	t.Run("close connection", func(t *testing.T) {
		conn := &mockConn{
			localAddr:  &mockAddr{"tcp", "10.0.0.1:80"},
			remoteAddr: &mockAddr{"tcp", "192.168.1.1:12345"},
		}
		op := NewConnOperator(conn, flow)

		require.False(t, op.IsClosed())
		err := op.CloseHijackedConn()
		require.NoError(t, err)
		require.True(t, op.IsClosed())
		require.True(t, conn.closed)
	})

	t.Run("hold connection", func(t *testing.T) {
		conn := &mockConn{
			localAddr:  &mockAddr{"tcp", "10.0.0.1:80"},
			remoteAddr: &mockAddr{"tcp", "192.168.1.1:12345"},
		}
		op := NewConnOperator(conn, flow)

		require.False(t, op.IsHeld())
		op.Hold()
		require.True(t, op.IsHeld())
	})
}

func TestProtocolDetector(t *testing.T) {
	pd := NewProtocolDetector()

	testCases := []struct {
		name     string
		data     []byte
		expected Protocol
	}{
		{"TLS", []byte{0x16, 0x03, 0x01, 0x00}, ProtocolTLS},
		{"SSH", []byte("SSH-2.0-OpenSSH"), ProtocolSSH},
		{"HTTP GET", []byte("GET / HTTP/1.1\r\n"), ProtocolHTTP},
		{"HTTP POST", []byte("POST /api HTTP/1.1\r\n"), ProtocolHTTP},
		{"HTTP Response", []byte("HTTP/1.1 200 OK\r\n"), ProtocolHTTP},
		{"Redis RESP Array", []byte("*2\r\n$4\r\nPING\r\n"), ProtocolRedis},
		{"Unknown", []byte{0x00, 0x01, 0x02}, ProtocolUnknown},
		{"Empty", []byte{}, ProtocolUnknown},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := pd.Detect(tc.data)
			require.Equal(t, tc.expected, result, "expected %s, got %s", tc.expected, result)
		})
	}
}

// TestProtocolDetector_Comprehensive tests all supported protocols with realistic mock data
func TestProtocolDetector_Comprehensive(t *testing.T) {
	pd := NewProtocolDetector()

	t.Run("TLS/SSL Detection", func(t *testing.T) {
		testCases := []struct {
			name string
			data []byte
		}{
			// TLS 1.0 Client Hello
			{"TLS 1.0 Client Hello", []byte{0x16, 0x03, 0x01, 0x00, 0x05, 0x01, 0x00, 0x00, 0x01, 0x00}},
			// TLS 1.1 Client Hello
			{"TLS 1.1 Client Hello", []byte{0x16, 0x03, 0x02, 0x00, 0x05, 0x01, 0x00, 0x00, 0x01, 0x00}},
			// TLS 1.2 Client Hello
			{"TLS 1.2 Client Hello", []byte{0x16, 0x03, 0x03, 0x00, 0x05, 0x01, 0x00, 0x00, 0x01, 0x00}},
			// SSL 3.0 (legacy)
			{"SSL 3.0", []byte{0x16, 0x03, 0x00, 0x00, 0x05}},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := pd.Detect(tc.data)
				require.Equal(t, ProtocolTLS, result, "expected TLS for %s", tc.name)
				require.True(t, pd.IsEncrypted(result), "TLS should be marked as encrypted")
			})
		}
	})

	t.Run("SSH Detection", func(t *testing.T) {
		testCases := []struct {
			name string
			data []byte
		}{
			{"SSH-2.0 OpenSSH", []byte("SSH-2.0-OpenSSH_8.0\r\n")},
			{"SSH-2.0 Dropbear", []byte("SSH-2.0-dropbear_2019.78\r\n")},
			{"SSH-1.99 (compatible)", []byte("SSH-1.99-OpenSSH_7.4\r\n")},
			{"SSH minimal", []byte("SSH-2.0")},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := pd.Detect(tc.data)
				require.Equal(t, ProtocolSSH, result, "expected SSH for %s", tc.name)
				require.True(t, pd.IsEncrypted(result), "SSH should be marked as encrypted")
			})
		}
	})

	t.Run("HTTP Request Detection", func(t *testing.T) {
		testCases := []struct {
			name string
			data []byte
		}{
			{"GET request", []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")},
			{"POST request", []byte("POST /api/v1/users HTTP/1.1\r\nContent-Type: application/json\r\n\r\n")},
			{"PUT request", []byte("PUT /resource HTTP/1.1\r\n")},
			{"DELETE request", []byte("DELETE /item/123 HTTP/1.1\r\n")},
			{"HEAD request", []byte("HEAD / HTTP/1.1\r\n")},
			{"OPTIONS request", []byte("OPTIONS * HTTP/1.1\r\n")},
			{"PATCH request", []byte("PATCH /user/1 HTTP/1.1\r\n")},
			{"CONNECT request", []byte("CONNECT proxy.example.com:443 HTTP/1.1\r\n")},
			{"TRACE request", []byte("TRACE / HTTP/1.1\r\n")},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := pd.Detect(tc.data)
				require.Equal(t, ProtocolHTTP, result, "expected HTTP for %s", tc.name)
				require.False(t, pd.IsEncrypted(result), "HTTP should not be marked as encrypted")
			})
		}
	})

	t.Run("HTTP Response Detection", func(t *testing.T) {
		testCases := []struct {
			name string
			data []byte
		}{
			{"200 OK", []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n")},
			{"404 Not Found", []byte("HTTP/1.1 404 Not Found\r\n")},
			{"500 Internal Server Error", []byte("HTTP/1.1 500 Internal Server Error\r\n")},
			{"301 Redirect", []byte("HTTP/1.1 301 Moved Permanently\r\n")},
			{"HTTP/1.0 response", []byte("HTTP/1.0 200 OK\r\n")},
			{"HTTP/2.0 style", []byte("HTTP/2 200\r\n")},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := pd.Detect(tc.data)
				require.Equal(t, ProtocolHTTP, result, "expected HTTP for %s", tc.name)
			})
		}
	})

	t.Run("Redis RESP Detection", func(t *testing.T) {
		testCases := []struct {
			name string
			data []byte
		}{
			{"Array (PING)", []byte("*1\r\n$4\r\nPING\r\n")},
			{"Array (SET)", []byte("*3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$5\r\nvalue\r\n")},
			{"Array (GET)", []byte("*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n")},
			{"Simple string", []byte("+OK\r\n")},
			{"Error", []byte("-ERR unknown command\r\n")},
			{"Integer", []byte(":1000\r\n")},
			{"Bulk string", []byte("$5\r\nhello\r\n")},
			{"Null bulk string", []byte("$-1\r\n")},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := pd.Detect(tc.data)
				require.Equal(t, ProtocolRedis, result, "expected Redis for %s", tc.name)
			})
		}
	})

	t.Run("MySQL Detection", func(t *testing.T) {
		// MySQL greeting packet: packet length (3 bytes) + sequence (1 byte) + protocol version (0x0a)
		mysqlGreeting := []byte{
			0x4a, 0x00, 0x00, 0x00, // packet length + sequence
			0x0a,                   // protocol version 10
			0x35, 0x2e, 0x37, 0x2e, // version string "5.7."
		}
		result := pd.Detect(mysqlGreeting)
		require.Equal(t, ProtocolMySQL, result, "expected MySQL")
	})

	t.Run("PostgreSQL Detection", func(t *testing.T) {
		// PostgreSQL SSL Request: length (4 bytes) = 8, SSL request code
		pgSSLRequest := []byte{
			0x00, 0x00, 0x00, 0x08, // Length = 8
			0x04, 0xd2, 0x16, 0x2f, // SSL request code (80877103)
		}
		result := pd.Detect(pgSSLRequest)
		require.Equal(t, ProtocolPostgreSQL, result, "expected PostgreSQL")
	})

	t.Run("MongoDB Detection", func(t *testing.T) {
		// MongoDB OP_MSG (opcode 2013 = 0x07DD in little-endian)
		mongoOpMsg := []byte{
			0x00, 0x00, 0x00, 0x20, // message length
			0x01, 0x00, 0x00, 0x00, // request ID
			0x00, 0x00, 0x00, 0x00, // response to
			0xdd, 0x07, 0x00, 0x00, // opcode = 2013 (OP_MSG)
		}
		result := pd.Detect(mongoOpMsg)
		require.Equal(t, ProtocolMongoDB, result, "expected MongoDB OP_MSG")

		// MongoDB OP_QUERY (opcode 2004 = 0x07D4 in little-endian)
		mongoOpQuery := []byte{
			0x00, 0x00, 0x00, 0x20, // message length
			0x01, 0x00, 0x00, 0x00, // request ID
			0x00, 0x00, 0x00, 0x00, // response to
			0xd4, 0x07, 0x00, 0x00, // opcode = 2004 (OP_QUERY)
		}
		result = pd.Detect(mongoOpQuery)
		require.Equal(t, ProtocolMongoDB, result, "expected MongoDB OP_QUERY")
	})

	t.Run("SMTP Detection", func(t *testing.T) {
		testCases := []struct {
			name string
			data []byte
		}{
			{"Server greeting", []byte("220 mail.example.com ESMTP Postfix\r\n")},
			{"EHLO command", []byte("EHLO client.example.com\r\n")},
			{"HELO command", []byte("HELO client.example.com\r\n")},
			{"MAIL FROM", []byte("MAIL FROM:<sender@example.com>\r\n")},
			{"RCPT TO", []byte("RCPT TO:<recipient@example.com>\r\n")},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := pd.Detect(tc.data)
				require.Equal(t, ProtocolSMTP, result, "expected SMTP for %s", tc.name)
			})
		}
	})

	t.Run("FTP Detection", func(t *testing.T) {
		testCases := []struct {
			name string
			data []byte
		}{
			// Note: "220 " prefix is ambiguous between FTP and SMTP
			// FTP-specific commands that are unambiguous
			{"USER command", []byte("USER anonymous\r\n")},
			{"PASS command", []byte("PASS guest@\r\n")},
			{"QUIT command", []byte("QUIT\r\n")},
			{"TYPE command", []byte("TYPE I\r\n")},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := pd.Detect(tc.data)
				require.Equal(t, ProtocolFTP, result, "expected FTP for %s", tc.name)
			})
		}
	})

	t.Run("Ambiguous 220 Response", func(t *testing.T) {
		// 220 is used by both SMTP and FTP, detection picks SMTP first
		// This is expected behavior for ambiguous protocols
		result := pd.Detect([]byte("220 Server ready.\r\n"))
		// Either SMTP or FTP is acceptable for ambiguous 220 response
		require.True(t, result == ProtocolSMTP || result == ProtocolFTP,
			"220 response should be detected as SMTP or FTP, got %s", result)
	})

	t.Run("Unknown Protocol Detection", func(t *testing.T) {
		testCases := []struct {
			name string
			data []byte
		}{
			{"Empty data", []byte{}},
			{"Random binary", []byte{0xDE, 0xAD, 0xBE, 0xEF}},
			{"Null bytes", []byte{0x00, 0x00, 0x00, 0x00}},
			{"Short data", []byte{0x01}},
			{"Non-protocol text", []byte("Hello World")},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := pd.Detect(tc.data)
				require.Equal(t, ProtocolUnknown, result, "expected Unknown for %s", tc.name)
			})
		}
	})
}

// TestProtocolDetector_EncryptedProtocols tests IsEncrypted method
func TestProtocolDetector_EncryptedProtocols(t *testing.T) {
	pd := NewProtocolDetector()

	encryptedProtocols := []Protocol{ProtocolTLS, ProtocolHTTPS, ProtocolSSH}
	for _, p := range encryptedProtocols {
		require.True(t, pd.IsEncrypted(p), "%s should be encrypted", p)
	}

	unencryptedProtocols := []Protocol{
		ProtocolHTTP, ProtocolRedis, ProtocolMySQL,
		ProtocolPostgreSQL, ProtocolMongoDB, ProtocolSMTP,
		ProtocolFTP, ProtocolDNS, ProtocolUnknown,
	}
	for _, p := range unencryptedProtocols {
		require.False(t, pd.IsEncrypted(p), "%s should not be encrypted", p)
	}
}

// TestProtocolDetector_StringRepresentation tests Protocol.String() method
func TestProtocolDetector_StringRepresentation(t *testing.T) {
	testCases := []struct {
		protocol Protocol
		expected string
	}{
		{ProtocolUnknown, "Unknown"},
		{ProtocolHTTP, "HTTP"},
		{ProtocolHTTPS, "HTTPS"},
		{ProtocolTLS, "TLS"},
		{ProtocolSSH, "SSH"},
		{ProtocolRedis, "Redis"},
		{ProtocolMySQL, "MySQL"},
		{ProtocolPostgreSQL, "PostgreSQL"},
		{ProtocolMongoDB, "MongoDB"},
		{ProtocolSMTP, "SMTP"},
		{ProtocolFTP, "FTP"},
		{ProtocolDNS, "DNS"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.protocol.String())
		})
	}
}

// TestProtocolDetector_DetectFromFrame tests DetectFromFrame method
func TestProtocolDetector_DetectFromFrame(t *testing.T) {
	pd := NewProtocolDetector()

	testCases := []struct {
		name     string
		data     []byte
		expected Protocol
	}{
		{"HTTP frame", []byte("GET / HTTP/1.1\r\n"), ProtocolHTTP},
		{"TLS frame", []byte{0x16, 0x03, 0x01, 0x00, 0x05}, ProtocolTLS},
		{"SSH frame", []byte("SSH-2.0-OpenSSH\r\n"), ProtocolSSH},
		{"Redis frame", []byte("*1\r\n$4\r\nPING\r\n"), ProtocolRedis},
		{"Unknown frame", []byte{0xDE, 0xAD, 0xBE, 0xEF}, ProtocolUnknown},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			frame := NewFrame(tc.data, DirectionClientToServer, nil)
			result := pd.DetectFromFrame(frame)
			require.Equal(t, tc.expected, result)
		})
	}
}

// TestProtocolDetector_EdgeCases tests edge cases for protocol detection
func TestProtocolDetector_EdgeCases(t *testing.T) {
	pd := NewProtocolDetector()

	t.Run("Partial TLS header", func(t *testing.T) {
		// Only first byte of TLS
		result := pd.Detect([]byte{0x16})
		require.Equal(t, ProtocolUnknown, result, "partial TLS should be unknown")

		// Missing version byte
		result = pd.Detect([]byte{0x16, 0x03})
		require.Equal(t, ProtocolTLS, result, "minimal TLS header should be detected")
	})

	t.Run("Partial HTTP method", func(t *testing.T) {
		// Partial GET
		result := pd.Detect([]byte("GE"))
		require.Equal(t, ProtocolUnknown, result, "partial HTTP method should be unknown")

		// GET without space
		result = pd.Detect([]byte("GET"))
		require.Equal(t, ProtocolUnknown, result, "GET without space should be unknown")
	})

	t.Run("Similar but not Redis", func(t *testing.T) {
		// Starts with * but not valid RESP
		result := pd.Detect([]byte("*hello world"))
		require.Equal(t, ProtocolUnknown, result, "invalid RESP should be unknown")

		// Starts with + but no CRLF
		result = pd.Detect([]byte("+OK"))
		require.Equal(t, ProtocolUnknown, result, "Redis without CRLF should be unknown")
	})

	t.Run("Large data detection", func(t *testing.T) {
		// HTTP request with large body
		largeHTTP := append([]byte("POST / HTTP/1.1\r\n\r\n"), bytes.Repeat([]byte("x"), 10000)...)
		result := pd.Detect(largeHTTP)
		require.Equal(t, ProtocolHTTP, result, "large HTTP should still be detected")
	})
}

func TestStreamSplitter_TimeGap(t *testing.T) {
	// Create a pipe for testing
	reader, writer := io.Pipe()

	var output bytes.Buffer
	config := &SplitterConfig{
		Strategy:         SplitByTimeGap,
		TimeGapThreshold: 10 * time.Millisecond,
		MaxBufferSize:    DefaultMaxBufferSize,
		ReadBufferSize:   1024,
	}

	splitter := NewStreamSplitter(reader, &output, DirectionClientToServer, config)
	splitter.Start()

	// Write data with gap
	go func() {
		writer.Write([]byte("hello"))
		time.Sleep(50 * time.Millisecond)
		writer.Write([]byte("world"))
		time.Sleep(50 * time.Millisecond)
		writer.Close()
	}()

	// Collect frames
	var frames []*Frame
	for frame := range splitter.Frames() {
		frames = append(frames, frame)
	}

	// Should have at least 2 frames due to time gap
	require.GreaterOrEqual(t, len(frames), 2, "expected at least 2 frames")
}

func TestStreamSplitter_MaxBufferSize(t *testing.T) {
	// Create a pipe for testing
	reader, writer := io.Pipe()

	var output bytes.Buffer
	config := &SplitterConfig{
		Strategy:         SplitByTimeGap,
		TimeGapThreshold: 1 * time.Second, // Long timeout
		MaxBufferSize:    100,             // Small buffer to trigger split
		ReadBufferSize:   50,
	}

	splitter := NewStreamSplitter(reader, &output, DirectionClientToServer, config)
	splitter.Start()

	// Write more data than max buffer size
	go func() {
		// Write 250 bytes (should trigger 2 splits at 100 bytes each)
		writer.Write(bytes.Repeat([]byte("x"), 250))
		time.Sleep(50 * time.Millisecond)
		writer.Close()
	}()

	// Collect frames
	var frames []*Frame
	for frame := range splitter.Frames() {
		frames = append(frames, frame)
	}

	// Should have at least 2 frames due to buffer overflow
	require.GreaterOrEqual(t, len(frames), 2, "expected at least 2 frames due to buffer overflow")

	// First frames should be exactly max buffer size
	if len(frames) >= 2 {
		require.Equal(t, 100, frames[0].Size(), "first frame should be exactly max buffer size")
		require.True(t, frames[0].IsUnknownProtocol(), "force-split frame should be marked as unknown protocol")
	}
}

func TestSplitterConfig_Defaults(t *testing.T) {
	config := DefaultSplitterConfig()

	require.Equal(t, SplitByTimeGap, config.Strategy)
	require.Equal(t, DefaultTimeGapThreshold, config.TimeGapThreshold)
	require.Equal(t, DefaultMaxBufferSize, config.MaxBufferSize)
	require.Equal(t, DefaultReadBufferSize, config.ReadBufferSize)
}

func TestSplitterConfig_TimeGapVariants(t *testing.T) {
	// Test different time gap configurations
	testCases := []time.Duration{
		TimeGap50ms,
		TimeGap100ms,
		TimeGap200ms,
		TimeGap300ms,
	}

	for _, gap := range testCases {
		config := NewSplitterConfig(gap)
		require.Equal(t, gap, config.TimeGapThreshold)
	}
}

func TestTCPMitm_Basic(t *testing.T) {
	connChan := make(chan net.Conn, 10)

	mitm, err := LoadConnectionChannel(connChan)
	require.NoError(t, err)
	require.NotNil(t, mitm)

	// Test setting callbacks
	var frameCount int
	var connCount int
	var mu sync.Mutex

	mitm.SetHijackTCPFrame(func(flow *ConnectionFlow, frame *Frame) {
		mu.Lock()
		frameCount++
		mu.Unlock()
	})

	mitm.SetHijackTCPConn(func(conn net.Conn, operator *ConnOperator) {
		mu.Lock()
		connCount++
		mu.Unlock()
		// Hold the connection to prevent further processing
		operator.Hold()
	})

	// Start MITM in background
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go func() {
		<-ctx.Done()
		close(connChan)
	}()

	// Run should complete when channel is closed
	err = mitm.Run()
	require.NoError(t, err)
}

func TestTCPMitm_Options(t *testing.T) {
	connChan := make(chan net.Conn, 10)

	mitm, err := LoadConnectionChannel(connChan,
		WithTimeGapThreshold(200*time.Millisecond),
		WithMaxBufferSize(16*1024),
		WithReadBufferSize(8*1024),
		WithSplitStrategy(SplitByTimeGap),
		WithProtocolAwareSplit(true),
	)
	require.NoError(t, err)

	require.Equal(t, 200*time.Millisecond, mitm.splitterConfig.TimeGapThreshold)
	require.Equal(t, 16*1024, mitm.splitterConfig.MaxBufferSize)
	require.Equal(t, 8*1024, mitm.splitterConfig.ReadBufferSize)
	require.Equal(t, SplitByTimeGap, mitm.splitterConfig.Strategy)
	require.True(t, mitm.splitterConfig.EnableProtocolAwareSplit)

	close(connChan)
}

func TestFrameDirection(t *testing.T) {
	require.Equal(t, "client->server", DirectionClientToServer.String())
	require.Equal(t, "server->client", DirectionServerToClient.String())
}

func TestSplitStrategy(t *testing.T) {
	// Test strategy aliases
	require.Equal(t, SplitByTimeGap, TimeGap)
	require.Equal(t, SplitByDirection, Direction)
	require.Equal(t, SplitBySize, FixedSize)
	require.Equal(t, SplitNone, NoSplit)
}

// ==================== Protocol Detection Tests ====================

func TestFrame_ProtocolDetection_TLS(t *testing.T) {
	// TLS Client Hello prefix
	tlsClientHello, _ := codec.DecodeHex("160301")
	frame := NewFrame(tlsClientHello, DirectionClientToServer, nil)

	// Use protocol detector directly (avoiding bin-parser for now)
	detector := NewProtocolDetector()
	protocol := detector.DetectFromFrame(frame)
	require.Equal(t, ProtocolTLS, protocol)
}

func TestFrame_ProtocolDetection_UnknownProtocol(t *testing.T) {
	// Random binary data
	randomData := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}
	frame := NewFrame(randomData, DirectionClientToServer, nil)

	detector := NewProtocolDetector()
	protocol := detector.DetectFromFrame(frame)
	require.Equal(t, ProtocolUnknown, protocol)

	// Mark as unknown protocol
	frame.SetUnknownProtocol(true)
	require.True(t, frame.IsUnknownProtocol())
}

func TestFrame_ProtocolDetection_HTTP(t *testing.T) {
	// HTTP request
	httpRequest := []byte("GET / HTTP/1.1\r\n")
	frame := NewFrame(httpRequest, DirectionClientToServer, nil)

	detector := NewProtocolDetector()
	protocol := detector.DetectFromFrame(frame)
	require.Equal(t, ProtocolHTTP, protocol)
}

func TestFrame_ProtocolDetection_SSH(t *testing.T) {
	sshData := []byte("SSH-2.0-OpenSSH_8.0")
	frame := NewFrame(sshData, DirectionClientToServer, nil)

	detector := NewProtocolDetector()
	protocol := detector.DetectFromFrame(frame)
	require.Equal(t, ProtocolSSH, protocol)

	// SSH is encrypted, so we can't parse it but we know it's SSH
	require.True(t, detector.IsEncrypted(protocol))
}

func TestFrame_ProtocolDetection_Redis(t *testing.T) {
	redisData := []byte("*2\r\n$4\r\nPING\r\n")
	frame := NewFrame(redisData, DirectionClientToServer, nil)

	detector := NewProtocolDetector()
	protocol := detector.DetectFromFrame(frame)
	require.Equal(t, ProtocolRedis, protocol)
}

// ==================== Protocol-Aware Frame Splitting Tests ====================

func TestFrameSplitting_RespectTimeGap(t *testing.T) {
	// Test that time gap splitting works with different intervals
	testCases := []struct {
		name     string
		timeGap  time.Duration
		messages []string
		delays   []time.Duration
	}{
		{
			name:     "100ms gap separates messages",
			timeGap:  TimeGap100ms,
			messages: []string{"message1", "message2"},
			delays:   []time.Duration{0, 200 * time.Millisecond},
		},
		{
			name:     "200ms gap separates messages",
			timeGap:  TimeGap200ms,
			messages: []string{"hello", "world"},
			delays:   []time.Duration{0, 300 * time.Millisecond},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reader, writer := io.Pipe()
			var output bytes.Buffer

			config := NewSplitterConfig(tc.timeGap)
			splitter := NewStreamSplitter(reader, &output, DirectionClientToServer, config)
			splitter.Start()

			// Write messages with delays
			go func() {
				for i, msg := range tc.messages {
					if tc.delays[i] > 0 {
						time.Sleep(tc.delays[i])
					}
					writer.Write([]byte(msg))
				}
				time.Sleep(tc.timeGap + 50*time.Millisecond)
				writer.Close()
			}()

			// Collect frames
			var frames []*Frame
			for frame := range splitter.Frames() {
				frames = append(frames, frame)
			}

			// Should have separate frames for each message due to time gap
			require.GreaterOrEqual(t, len(frames), len(tc.messages),
				"expected at least %d frames", len(tc.messages))
		})
	}
}

func TestFrameSplitting_MaxBufferEnforced(t *testing.T) {
	// Test that max buffer size is enforced (default 8KB)
	reader, writer := io.Pipe()
	var output bytes.Buffer

	config := DefaultSplitterConfig()
	config.MaxBufferSize = 1024               // 1KB for testing
	config.TimeGapThreshold = 5 * time.Second // Long timeout to ensure buffer split

	splitter := NewStreamSplitter(reader, &output, DirectionClientToServer, config)
	splitter.Start()

	// Write more than max buffer size
	go func() {
		// Write 3KB of data
		writer.Write(bytes.Repeat([]byte("x"), 3*1024))
		time.Sleep(100 * time.Millisecond)
		writer.Close()
	}()

	// Collect frames
	var frames []*Frame
	for frame := range splitter.Frames() {
		frames = append(frames, frame)
	}

	// Should have at least 3 frames (3KB / 1KB = 3)
	require.GreaterOrEqual(t, len(frames), 3, "expected at least 3 frames due to buffer limit")

	// Check that first frames are exactly max buffer size
	for i := 0; i < len(frames)-1; i++ {
		if frames[i].Size() == config.MaxBufferSize {
			require.True(t, frames[i].IsUnknownProtocol(),
				"force-split frame should be marked as unknown protocol")
		}
	}
}

// ==================== Integration Tests ====================

func TestFullPipeline_UnknownProtocol(t *testing.T) {
	// Simulate unknown protocol data
	unknownData := bytes.Repeat([]byte{0xDE, 0xAD, 0xBE, 0xEF}, 100)

	reader, writer := io.Pipe()
	var output bytes.Buffer

	config := NewSplitterConfig(TimeGap100ms)
	splitter := NewStreamSplitter(reader, &output, DirectionClientToServer, config)
	splitter.Start()

	// Write unknown data
	go func() {
		writer.Write(unknownData)
		time.Sleep(200 * time.Millisecond)
		writer.Close()
	}()

	// Get frame
	frame := <-splitter.Frames()
	require.NotNil(t, frame)

	// Parse - should fail gracefully
	node, err := frame.GetBinParserNode()
	require.Nil(t, node)
	require.Nil(t, err) // No error, just unknown protocol
	require.True(t, frame.IsUnknownProtocol())
	require.Equal(t, ProtocolUnknown, frame.GetDetectedProtocol())

	// Drain remaining frames
	for range splitter.Frames() {
	}
}

func TestFrameCallbackWithParsing(t *testing.T) {
	// Test frame callback with protocol detection
	httpRequest := "GET / HTTP/1.1\r\n"

	var parsedFrames []*Frame
	var mu sync.Mutex

	callback := func(flow *ConnectionFlow, frame *Frame) {
		// Detect protocol
		detector := NewProtocolDetector()
		protocol := detector.DetectFromFrame(frame)

		mu.Lock()
		defer mu.Unlock()

		if protocol != ProtocolUnknown {
			t.Logf("Detected protocol from %s: %s",
				flow.GetClientAddr(), protocol)
			parsedFrames = append(parsedFrames, frame)
		}
	}

	// Simulate frame processing
	flow := NewConnectionFlow(nil, "192.168.1.1", 12345, "10.0.0.1", 80)
	frame := NewFrame([]byte(httpRequest), DirectionClientToServer, nil)

	callback(flow, frame)

	require.Len(t, parsedFrames, 1)

	// Verify protocol detection using detector
	detector := NewProtocolDetector()
	protocol := detector.DetectFromFrame(parsedFrames[0])
	require.Equal(t, ProtocolHTTP, protocol)
}

func TestProtocolToBinParserRule(t *testing.T) {
	testCases := []struct {
		protocol Protocol
		hasRule  bool
	}{
		{ProtocolHTTP, true},
		{ProtocolTLS, true},
		{ProtocolHTTPS, true},
		{ProtocolDNS, true},
		{ProtocolSSH, false},        // No SSH parser (encrypted)
		{ProtocolRedis, false},      // No Redis parser yet
		{ProtocolMySQL, false},      // No MySQL parser yet
		{ProtocolPostgreSQL, false}, // No PostgreSQL parser yet
		{ProtocolMongoDB, false},    // No MongoDB parser yet
		{ProtocolSMTP, false},       // No SMTP parser yet
		{ProtocolFTP, false},        // No FTP parser yet
		{ProtocolUnknown, false},
	}

	for _, tc := range testCases {
		rule := protocolToBinParserRule(tc.protocol)
		if tc.hasRule {
			require.NotEmpty(t, rule, "protocol %s should have a rule", tc.protocol)
		} else {
			require.Empty(t, rule, "protocol %s should not have a rule", tc.protocol)
		}
	}
}

func TestDefaultMaxBufferSize(t *testing.T) {
	// Verify default max buffer size is 8KB
	require.Equal(t, 8*1024, DefaultMaxBufferSize)
}

func TestStreamSplitter_SplitBySize(t *testing.T) {
	reader, writer := io.Pipe()
	var output bytes.Buffer

	config := &SplitterConfig{
		Strategy:       SplitBySize,
		MaxFrameSize:   100,
		ReadBufferSize: 50,
	}

	splitter := NewStreamSplitter(reader, &output, DirectionClientToServer, config)
	splitter.Start()

	// Write 350 bytes
	go func() {
		writer.Write(bytes.Repeat([]byte("a"), 350))
		time.Sleep(50 * time.Millisecond)
		writer.Close()
	}()

	// Collect frames
	var frames []*Frame
	for frame := range splitter.Frames() {
		frames = append(frames, frame)
	}

	// Should have 4 frames: 3 full (100 bytes each) + 1 partial (50 bytes)
	require.Equal(t, 4, len(frames), "expected 4 frames")

	// First 3 frames should be exactly 100 bytes
	for i := 0; i < 3; i++ {
		require.Equal(t, 100, frames[i].Size(), "frame %d should be 100 bytes", i)
	}

	// Last frame should be 50 bytes
	require.Equal(t, 50, frames[3].Size(), "last frame should be 50 bytes")
}

func TestStreamSplitter_Transparent(t *testing.T) {
	reader, writer := io.Pipe()
	var output bytes.Buffer

	config := &SplitterConfig{
		Strategy:       SplitNone,
		ReadBufferSize: 100,
	}

	splitter := NewStreamSplitter(reader, &output, DirectionClientToServer, config)
	splitter.Start()

	// Write data in chunks
	go func() {
		writer.Write([]byte("chunk1"))
		time.Sleep(10 * time.Millisecond)
		writer.Write([]byte("chunk2"))
		time.Sleep(10 * time.Millisecond)
		writer.Close()
	}()

	// Collect frames - each write should produce a frame
	var frames []*Frame
	for frame := range splitter.Frames() {
		frames = append(frames, frame)
	}

	// Should have at least 2 frames (one per write)
	require.GreaterOrEqual(t, len(frames), 2)
}
