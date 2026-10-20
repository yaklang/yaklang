package fp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

var protocolDNSQuery = []byte{0x12, 0x34, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 'a', 0, 0, 1, 0, 1}
var protocolDNSResponse = []byte{0x12, 0x34, 0x81, 0x80, 0, 1, 0, 1, 0, 0, 0, 0, 1, 'a', 0, 0, 1, 0, 1, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 127, 0, 0, 1}

func protocolMessageFields(t *testing.T, values map[string]any, protocol string) []map[string]any {
	t.Helper()
	messages, ok := values[protocol].([]any)
	require.True(t, ok, "protocol=%s output=%#v", protocol, values)
	require.NotEmpty(t, messages)
	var fields []map[string]any
	for _, value := range messages {
		message, ok := value.(map[string]any)
		require.True(t, ok)
		require.NotEmpty(t, message["profile"])
		require.NotEmpty(t, message["status"])
		field, ok := message["fields"].(map[string]any)
		require.True(t, ok)
		fields = append(fields, field)
	}
	return fields
}

func TestParseProtocolFieldsResponseData(t *testing.T) {
	cases := []struct {
		name, protocol, key string
		wire                []byte
		transport           TransportProto
		port                int
		want                any
	}{
		{"ssh", "ssh", "Identification", []byte("SSH-2.0-OpenSSH_9.6\r\n"), TCP, 22, "SSH-2.0-OpenSSH_9.6"},
		{"ftp off port", "ftp", "Code", []byte("220 FTP service ready\r\n"), TCP, 18021, "220"},
		{"http response only", "http", "Status Code", []byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello"), TCP, 18080, 200},
		{"dns", "dns", "Response", protocolDNSResponse, UDP, 53, true},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			wire := bytes.Clone(item.wire)
			got := ParseProtocolFields(wire, item.transport, item.port)
			fields := protocolMessageFields(t, got, item.protocol)
			require.EqualValues(t, item.want, fields[0][item.key])
			clear(wire)
			require.EqualValues(t, item.want, fields[0][item.key], "fields must own the input")
			_, err := json.Marshal(got)
			require.NoError(t, err)
		})
	}
}

func TestParseProtocolFieldsFramesReplies(t *testing.T) {
	got := ParseProtocolFields([]byte("220 FTP service ready\r\n221 Goodbye\r\n"), TCP, 21)
	fields := protocolMessageFields(t, got, "ftp")
	require.Len(t, fields, 2)
	require.Equal(t, "220", fields[0]["Code"])
	require.Equal(t, "221", fields[1]["Code"])
}

func TestProtocolFieldsPreserveProbeContext(t *testing.T) {
	request := []byte("*1\r\n$4\r\nPING\r\n")
	got := parseProtocolFields(context.Background(), request, [][]byte{[]byte("+PONG\r\n")}, TCP, 6379, false)
	fields := protocolMessageFields(t, got, "redis")
	require.Len(t, fields, 1, "request messages must not enter response fields")
	require.Equal(t, "PING", fields[0]["Command"])
	require.Equal(t, true, fields[0]["Matched Request"])
	require.Equal(t, "PONG", fields[0]["Value"].(map[string]any)["Value"])
}

func TestProtocolFieldsDoNotInventMessageBoundaries(t *testing.T) {
	request := []byte("GET / HTTP/1.0\r\n\r\n")
	response := []byte("HTTP/1.0 200 OK\r\n\r\nhello")
	for _, eof := range []bool{false, true} {
		got := parseProtocolFields(context.Background(), request, [][]byte{response}, TCP, 80, eof)
		messages, ok := got["http"].([]any)
		require.True(t, ok)
		if eof {
			require.Equal(t, "decoded", messages[0].(map[string]any)["status"])
		} else {
			require.Equal(t, "headers", messages[0].(map[string]any)["completeness"])
		}
	}
	require.Empty(t, ParseProtocolFields([]byte("SSH-2.0-truncated"), TCP, 22))
	require.Empty(t, ParseProtocolFields(protocolDNSResponse[:20], UDP, 53))
	require.Empty(t, ParseProtocolFields([]byte("not HTTP, despite port 80\r\n"), TCP, 80))
}

func TestProtocolFieldsLimitsAndCancellation(t *testing.T) {
	for _, input := range [][]byte{nil, {}, make([]byte, maxProtocolResponseBytes+1)} {
		got := ParseProtocolFields(input, TCP, 22)
		require.NotNil(t, got)
		require.Empty(t, got)
	}
	require.Empty(t, ParseProtocolFields([]byte("SSH-2.0-fixture\r\n"), TransportProto("other"), 22))
	require.Empty(t, ParseProtocolFields([]byte("SSH-2.0-fixture\r\n"), TCP, 65536))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Empty(t, parseProtocolFields(ctx, nil, [][]byte{[]byte("SSH-2.0-fixture\r\n")}, TCP, 22, false))
	got := ParseProtocolFields([]byte(strings.Repeat("220 FTP ready\r\n", maxProtocolMessages+50)), TCP, 21)
	require.Len(t, got["ftp"], maxProtocolMessages)
}

func TestProtocolFieldsMergeAndJSON(t *testing.T) {
	result := &MatchResult{Target: "127.0.0.1", Port: 22}
	result.Tidy()
	require.NotNil(t, result.ProtocolFields)
	first := ParseProtocolFields([]byte("SSH-2.0-first\r\n"), TCP, 22)
	second := ParseProtocolFields([]byte("SSH-2.0-second\r\n"), TCP, 22)
	result.mergeProtocolFields(first)
	result.Merge(&MatchResult{Target: result.Target, Port: result.Port, ProtocolFields: second})
	require.Len(t, result.ProtocolFields["ssh"], 2)
	require.Len(t, first["ssh"], 1)
	result.Merge(&MatchResult{Target: "127.0.0.2", Port: result.Port, ProtocolFields: second})
	require.Len(t, result.ProtocolFields["ssh"], 2)
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	var decoded MatchResult
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Len(t, decoded.ProtocolFields["ssh"], 2)
	require.Equal(t, "SSH-2.0-first", protocolMessageFields(t, decoded.ProtocolFields, "ssh")[0]["Identification"])
}

func TestProtocolFieldsConcurrentIsolation(t *testing.T) {
	var wg sync.WaitGroup
	outputs := make(chan map[string]any, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outputs <- ParseProtocolFields([]byte("SSH-2.0-fixture\r\n"), TCP, 22)
		}()
	}
	wg.Wait()
	close(outputs)
	for output := range outputs {
		fields := protocolMessageFields(t, output, "ssh")
		require.Equal(t, "SSH-2.0-fixture", fields[0]["Identification"])
		fields[0]["Identification"] = "caller changed"
	}
}

func TestServiceScanProtocolFieldsWithoutFingerprintMatch(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_, _ = conn.Write([]byte("SSH-2.0-OwnedFixture\r\n"))
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); <-done })
	port := listener.Addr().(*net.TCPAddr).Port
	rules, err := ParseNmapServiceProbeToRuleMap([]byte("Probe TCP NULL q||\nmatch fixture m|^will-not-match$|\n"))
	require.NoError(t, err)
	var onOpen map[string]any
	matcher, err := NewDefaultFingerprintMatcher(NewConfig(
		WithNmapRule(rules), WithDisableWebFingerprint(true), WithActiveMode(false), WithProbeTimeout(200*time.Millisecond),
		WithOnPortOpenCallback(func(result *MatchResult) { onOpen = result.ProtocolFields }),
	))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := matcher.MatchWithContext(ctx, "127.0.0.1", port)
	require.NoError(t, err)
	require.Equal(t, OPEN, result.State)
	require.Empty(t, result.Fingerprint.Raw, "Raw remains the matched nmap rule, not response bytes")
	require.Equal(t, "SSH-2.0-OwnedFixture", protocolMessageFields(t, result.ProtocolFields, "ssh")[0]["Identification"])
	require.Contains(t, onOpen, "ssh")
}

func TestServiceScanProtocolFieldsActiveTCP(t *testing.T) {
	request := []byte("*1\r\n$4\r\nPING\r\n")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.SetDeadline(time.Now().Add(time.Second))
			data := make([]byte, len(request))
			if _, err := io.ReadFull(conn, data); err == nil && bytes.Equal(data, request) {
				_, _ = conn.Write([]byte("+PONG\r\n"))
			}
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); <-done })
	port := listener.Addr().(*net.TCPAddr).Port
	rules, err := ParseNmapServiceProbeToRuleMap([]byte(fmt.Sprintf("Probe TCP RedisFixture q|*1\\r\\n$4\\r\\nPING\\r\\n|\nrarity 1\nports %d\nmatch redis m|^\\+PONG|\n", port)))
	require.NoError(t, err)
	matcher, err := NewDefaultFingerprintMatcher(NewConfig(
		WithNmapRule(rules), WithDisableWebFingerprint(true), WithProbesMax(1), WithProbeTimeout(200*time.Millisecond),
	))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := matcher.MatchWithContext(ctx, "127.0.0.1", port)
	require.NoError(t, err)
	require.Equal(t, OPEN, result.State)
	require.Contains(t, result.Fingerprint.Raw, "match redis")
	fields := protocolMessageFields(t, result.ProtocolFields, "redis")
	require.Len(t, fields, 1)
	require.Equal(t, "PING", fields[0]["Command"])
	require.Equal(t, true, fields[0]["Matched Request"])
}

func TestServiceScanProtocolFieldsWeb(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "OwnedFixture")
		w.Header().Set("Content-Length", "5")
		_, _ = w.Write([]byte("hello"))
	}))
	t.Cleanup(server.Close)
	host, port, err := utils.ParseStringToHostPort(server.URL)
	require.NoError(t, err)
	matcher, err := NewDefaultFingerprintMatcher(NewConfig(
		WithOnlyEnableWebFingerprint(true), WithProbeTimeout(500*time.Millisecond),
		func(c *Config) { c.DisableDefaultFingerprint = true; c.DisableDefaultIotFingerprint = true },
	))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := matcher.MatchWithContext(ctx, host, port)
	require.NoError(t, err)
	require.Equal(t, OPEN, result.State)
	fields := protocolMessageFields(t, result.ProtocolFields, "http")
	require.NotEmpty(t, fields)
	raw, err := json.Marshal(fields)
	require.NoError(t, err)
	require.Contains(t, string(raw), "OwnedFixture")
	require.Contains(t, string(raw), "hello")
}

func TestServiceScanProtocolFieldsUDPDatagrams(t *testing.T) {
	listener, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan struct{})
	go func() {
		defer close(done)
		buffer := make([]byte, 1024)
		for {
			_, address, err := listener.ReadFrom(buffer)
			if err != nil {
				return
			}
			_, _ = listener.WriteTo(protocolDNSResponse, address)
			_, _ = listener.WriteTo(protocolDNSResponse, address)
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); <-done })
	var payload strings.Builder
	for _, b := range protocolDNSQuery {
		fmt.Fprintf(&payload, `\x%02x`, b)
	}
	port := listener.LocalAddr().(*net.UDPAddr).Port
	rules, err := ParseNmapServiceProbeToRuleMap([]byte(fmt.Sprintf("Probe UDP DNSFixture q|%s|\nrarity 1\nports %d\nmatch domain m|^will-not-match$|\n", payload.String(), port)))
	require.NoError(t, err)
	matcher, err := NewDefaultFingerprintMatcher(NewConfig(
		WithNmapRule(rules), WithTransportProtos(UDP), WithDisableWebFingerprint(true), WithProbeTimeout(150*time.Millisecond), WithProbesMax(1),
	))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := matcher.MatchWithContext(ctx, "127.0.0.1", utils.EncodeUDPPort(port))
	require.NoError(t, err)
	require.Equal(t, OPEN, result.State)
	fields := protocolMessageFields(t, result.ProtocolFields, "dns")
	require.Len(t, fields, 2, "concatenated UDP replies must be parsed as separate datagrams")
	for _, field := range fields {
		require.EqualValues(t, 0x1234, field["ID"])
	}
}

func TestProtocolResponseConnDoesNotInventEOF(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	wire := []byte("HTTP/1.0 200 OK\r\n\r\nhello-and-truncated")
	go func() { _, _ = server.Write(wire); _ = server.Close() }()
	conn := &protocolResponseConn{Conn: client}
	banner, err := io.ReadAll(conn)
	require.NoError(t, err)
	request := []byte("GET / HTTP/1.0\r\n\r\n")
	full := conn.parse(context.Background(), request, banner, TCP, 80)
	require.Equal(t, "decoded", full["http"].([]any)[0].(map[string]any)["status"])
	prefix := conn.parse(context.Background(), request, banner[:len(banner)-4], TCP, 80)
	require.Equal(t, "headers", prefix["http"].([]any)[0].(map[string]any)["completeness"])
}

func BenchmarkParseProtocolFieldsSSH(b *testing.B) {
	wire := []byte("SSH-2.0-OpenSSH_9.6\r\n")
	ParseProtocolFields(wire, TCP, 22)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseProtocolFields(wire, TCP, 22)
	}
}
