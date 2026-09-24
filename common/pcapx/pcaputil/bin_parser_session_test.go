package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	binparser "github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"golang.org/x/net/http2/hpack"
)

type sessionStep struct {
	dir  int
	wire []byte
}

func h2TestFrame(typ, flags byte, id uint32, payload []byte) []byte {
	w := make([]byte, 9, len(payload)+9)
	w[0] = byte(len(payload) >> 16)
	w[1] = byte(len(payload) >> 8)
	w[2] = byte(len(payload))
	w[3] = typ
	w[4] = flags
	binary.BigEndian.PutUint32(w[5:], id)
	return append(w, payload...)
}

func h2TestFixtures(t testing.TB) []sessionStep {
	t.Helper()
	var client, server bytes.Buffer
	ce, se := hpack.NewEncoder(&client), hpack.NewEncoder(&server)
	encode := func(b *bytes.Buffer, e *hpack.Encoder, fields ...hpack.HeaderField) []byte {
		b.Reset()
		for _, v := range fields {
			require.NoError(t, e.WriteField(v))
		}
		return bytes.Clone(b.Bytes())
	}
	req := func(method, path string) []byte {
		return encode(&client, ce, hpack.HeaderField{Name: ":method", Value: method}, hpack.HeaderField{Name: ":scheme", Value: "http"}, hpack.HeaderField{Name: ":path", Value: path}, hpack.HeaderField{Name: ":authority", Value: "example.test"}, hpack.HeaderField{Name: "x-session", Value: "shared-dynamic-value"})
	}
	response := func() []byte {
		return encode(&server, se, hpack.HeaderField{Name: ":status", Value: "200"}, hpack.HeaderField{Name: "content-type", Value: "text/plain"}, hpack.HeaderField{Name: "x-session", Value: "server-dynamic-value"})
	}
	r1, r3 := req("GET", "/one"), req("POST", "/two")
	s3, s1 := response(), response()
	return []sessionStep{
		{0, append([]byte(binH2Preface), h2TestFrame(4, 0, 0, nil)...)},
		{1, h2TestFrame(4, 0, 0, nil)}, {0, h2TestFrame(4, 1, 0, nil)}, {1, h2TestFrame(4, 1, 0, nil)},
		{0, append(h2TestFrame(1, 1, 1, r1[:3]), h2TestFrame(9, 4, 1, r1[3:])...)},
		{0, h2TestFrame(1, 4, 3, r3)}, {0, h2TestFrame(0, 1, 3, []byte("body"))},
		{1, h2TestFrame(1, 4, 3, s3)}, {1, h2TestFrame(0, 1, 3, []byte("second"))},
		{1, h2TestFrame(1, 4, 1, s1)}, {1, h2TestFrame(0, 1, 1, []byte("first"))},
		{1, h2TestFrame(7, 0, 0, []byte{0, 0, 0, 3, 0, 0, 0, 0})},
	}
}

func mysqlTestPacket(seq byte, p []byte) []byte {
	return append([]byte{byte(len(p)), byte(len(p) >> 8), byte(len(p) >> 16), seq}, p...)
}

func mysqlTestHandshake(caps uint32) (greeting, response []byte) {
	p := append([]byte{10}, []byte("8.0.36-fixture\x00")...)
	p = append(p, 42, 0, 0, 0)
	p = append(p, []byte("12345678")...)
	p = append(p, 0, byte(caps), byte(caps>>8), 33, 2, 0, byte(caps>>16), byte(caps>>24), 21)
	p = append(p, make([]byte, 10)...)
	p = append(p, []byte("abcdefghijkl\x00mysql_native_password\x00")...)
	greeting = mysqlTestPacket(0, p)
	p = make([]byte, 32)
	binary.LittleEndian.PutUint32(p, caps)
	binary.LittleEndian.PutUint32(p[4:], 1<<20)
	p[8] = 33
	p = append(p, []byte("fixture\x00")...)
	p = append(p, 0)
	p = append(p, []byte("mysql_native_password\x00")...)
	response = mysqlTestPacket(1, p)
	return
}

const mysqlTestCaps uint32 = 1 | 1<<9 | 1<<15 | 1<<19 | 1<<17

func mysqlTestResult(seq byte, deprecated bool, status uint16) []byte {
	col := []byte{3, 'd', 'e', 'f', 0, 0, 0, 1, 'x', 0, 12, 33, 0, 36, 0, 0, 0, 0xfd, 0, 0, 0, 0, 0}
	w := mysqlTestPacket(seq, []byte{1})
	seq++
	w = append(w, mysqlTestPacket(seq, col)...)
	seq++
	if !deprecated {
		w = append(w, mysqlTestPacket(seq, []byte{0xfe, 0, 0, 2, 0})...)
		seq++
	}
	w = append(w, mysqlTestPacket(seq, []byte{2, '4', '2'})...)
	seq++
	end := []byte{0xfe, 0, 0, byte(status), byte(status >> 8)}
	if deprecated {
		end = append(end, 0, 0)
	}
	return append(w, mysqlTestPacket(seq, end)...)
}

func mysqlTestFixtures(deprecated bool, tracked ...bool) []sessionStep {
	caps := mysqlTestCaps
	if len(tracked) > 0 && tracked[0] {
		caps |= 1 << 23
	}
	if deprecated {
		caps |= 1 << 24
	}
	g, r := mysqlTestHandshake(caps)
	result := mysqlTestResult(1, deprecated, 10)
	next := byte(6)
	if deprecated {
		next = 5
	}
	result = append(result, mysqlTestResult(next, deprecated, 2)...)
	return []sessionStep{
		{1, g}, {0, r},
		{1, mysqlTestPacket(2, append([]byte{0xfe}, []byte("caching_sha2_password\x0012345678\x00")...))},
		{0, mysqlTestPacket(3, bytes.Repeat([]byte{0x42}, 32))}, {1, mysqlTestPacket(4, []byte{1, 3})}, {1, mysqlTestPacket(5, []byte{0, 0, 0, 2, 0, 0, 0})},
		{0, mysqlTestPacket(0, append([]byte{3}, []byte("SELECT 42; SELECT 42")...))}, {1, result},
		{0, mysqlTestPacket(0, append([]byte{3}, []byte("bad sql")...))}, {1, mysqlTestPacket(1, []byte("\xff\x28\x04#42000syntax error"))},
		{0, mysqlTestPacket(0, []byte{14})}, {1, mysqlTestPacket(1, []byte{0, 0, 0, 2, 0, 0, 0})}, {0, mysqlTestPacket(0, []byte{1})},
	}
}

func sessionTestPCAP(t testing.TB, steps []sessionStep, port layers.TCPPort, chunk int, ipv6, ng bool) []byte {
	t.Helper()
	seq := [2]uint32{1, 1}
	tcp := []tcpStep{{syn: true, window: 65535}, {syn: true, synack: true, reverse: true, ack: 1, window: 65535}, {seq: 1, ack: 1, window: 65535}}
	for _, s := range steps {
		for w := s.wire; len(w) > 0; {
			n := len(w)
			if chunk > 0 {
				n = min(n, chunk)
			}
			tcp = append(tcp, tcpStep{seq: seq[s.dir], ack: seq[1-s.dir], window: 65535, reverse: s.dir == 1, data: string(w[:n])})
			seq[s.dir] += uint32(n)
			w = w[n:]
		}
	}
	tcp = append(tcp, tcpStep{seq: seq[0], ack: seq[1], window: 65535, fin: true}, tcpStep{seq: seq[1], ack: seq[0] + 1, window: 65535, fin: true, reverse: true}, tcpStep{seq: seq[0] + 1, ack: seq[1] + 1, window: 65535})
	return binTestPcap(t, tcp, port, ipv6, ng)
}

func sessionTestFlow(t testing.TB, protocol string, steps []sessionStep, chunk int, deferred bool) ([]*ProtocolEvent, *binFlow) {
	t.Helper()
	var events []*ProtocolEvent
	c := NewDefaultConfig()
	require.NoError(t, WithBinParserConfig(BinParserConfig{Deferred: deferred, OnEvent: func(e *ProtocolEvent) { events = append(events, e) }})(c))
	require.NoError(t, c.prepareBinParser())
	f := &binFlow{a: c.binParser, id: 1, endpoints: [2]string{"192.0.2.1:12345", "192.0.2.2:3306"}, ports: [2]uint16{12345, 3306}}
	for _, s := range steps {
		for w := s.wire; len(w) > 0; {
			n := len(w)
			if chunk > 0 {
				n = min(n, chunk)
			}
			f.feed(s.dir, w[:n], time.Unix(1, 0))
			w = w[n:]
		}
	}
	f.close(TrafficFlowCloseReason_FIN)
	require.Zero(t, f.a.stats().BufferedBytes)
	require.Nil(t, f.a.err.Load())
	return events, f
}

func assertSessionEvents(t testing.TB, events []*ProtocolEvent, protocol string, deferred bool) {
	t.Helper()
	for _, e := range events {
		status := "decoded"
		if deferred {
			status = "deferred"
		}
		require.Equal(t, status, e.Status, "%s: %s %s", e.Entry, e.Error, e.Summary)
		require.Equal(t, protocol, e.Protocol)
		require.NotEmpty(t, e.Session)
		decoded, err := e.Decode()
		require.NoError(t, err)
		require.NotEmpty(t, decoded["fields"])
		require.Equal(t, e.Session, decoded["session"])
	}
}

func TestLiveProtocolSegmentsAndDeferred(t *testing.T) {
	for _, protocol := range []string{"http2", "mysql", "mysql-deprecated"} {
		t.Run(protocol, func(t *testing.T) {
			steps := h2TestFixtures(t)
			want := 12
			name := protocol
			if protocol != "http2" {
				steps = mysqlTestFixtures(protocol == "mysql-deprecated")
				want = 14
				name = "mysql"
			}
			for _, chunk := range []int{0, 1, 2, 3, 4, 7, 23, 24, 31, 64} {
				for _, deferred := range []bool{false, true} {
					t.Run(fmt.Sprintf("chunk=%d/deferred=%v", chunk, deferred), func(t *testing.T) {
						events, _ := sessionTestFlow(t, name, steps, chunk, deferred)
						assertSessionEvents(t, events, name, deferred)
						require.Len(t, events, want)
						if name == "http2" {
							require.Equal(t, "request", events[4].Session["Header Kind"])
							require.Equal(t, "response", events[9].Session["Header Kind"])
							require.Equal(t, true, events[10].Session["Exchange Complete"])
						} else {
							require.Equal(t, uint64(1), events[7].Session["Transaction ID"])
							require.Equal(t, true, events[7].Session["More Results"])
							require.Equal(t, false, events[8].Session["More Results"])
							require.Equal(t, true, events[5].Session["Authentication OK Observed"])
						}
					})
				}
			}
		})
	}
}

func TestLiveProtocolOriginalCaptures(t *testing.T) {
	for _, name := range []string{"ndpi/ndpi-http2.pcapng", "ndpi/ndpi-mysql.pcapng"} {
		t.Run(name, func(t *testing.T) {
			wire := binCorpusBytes(t, name)
			wantHash := "8ca9722db9527618db1f1a35841d01e46d0d82b2e3bde022e5d24e28cff0e598"
			wantMessages, wantBytes := 12, 591
			if name == "ndpi/ndpi-mysql.pcapng" {
				wantHash = "11e0988e75b471e25d4c1e3948b9357b77882e5710a492976f2c54d8cf2d98b7"
				wantMessages, wantBytes = 28, 4271
			}
			require.Equal(t, wantHash, fmt.Sprintf("%x", sha256.Sum256(wire)))
			events, stats, err := binReplay(t, wire, 1)
			require.NoError(t, err)
			require.Zero(t, stats.BufferedBytes)
			for _, e := range events {
				t.Logf("%s %s %s %s session=%v", e.Protocol, e.Status, e.Entry, e.Error, e.Session)
			}
			t.Logf("stats=%+v", stats)
			require.Zero(t, stats.Malformed)
			require.Zero(t, stats.ContextRequired)
			require.EqualValues(t, wantMessages, stats.Decoded)
			require.EqualValues(t, wantBytes, stats.InputBytes)
			require.Equal(t, stats.InputBytes, stats.MessageBytes)
			require.Zero(t, stats.Incomplete)
			if name == "ndpi/ndpi-http2.pcapng" {
				var found, request bool
				for _, e := range events {
					if e.Session["Header Kind"] == "request" {
						require.Equal(t, "POST", e.Session["Request Method"])
						request = true
					}
					if e.Session["Header Kind"] == "response" {
						for _, h := range e.Session["Headers"].([]map[string]any) {
							if h["Name"] == ":status" {
								require.Equal(t, "201", h["Value"])
								found = true
							}
						}
					}
				}
				require.True(t, found)
				require.True(t, request)
			} else {
				query := findSessionField(events[3].Fields, "Query Bytes")
				require.Equal(t, []byte("select @@version_comment limit 1"), query)
				meta := events[4].Metadata.(map[string]any)
				require.Equal(t, 1, meta["Row Count"])
				rows := meta["Rows"].([][]map[string]any)
				require.Equal(t, []byte("Ubuntu 22.04"), rows[0][0]["Value"])
			}
		})
	}
}

func findSessionField(v any, name string) any {
	switch x := v.(type) {
	case map[string]any:
		if v, ok := x[name]; ok {
			return v
		}
		for _, v := range x {
			if out := findSessionField(v, name); out != nil {
				return out
			}
		}
	case []any:
		for _, v := range x {
			if out := findSessionField(v, name); out != nil {
				return out
			}
		}
	}
	return nil
}

type liveBoundedReader struct {
	*bytes.Reader
	bits uint64
}

func (r *liveBoundedReader) InputBitLength() uint64 { return r.bits }

func TestLiveProtocolPublicRuleEquivalence(t *testing.T) {
	// Stateful routing must still deliver the exact YAML/ParseBinary contract.
	// This catches an implementation which only works through a native shortcut.
	for _, steps := range [][]sessionStep{h2TestFixtures(t), mysqlTestFixtures(false), mysqlTestFixtures(true)} {
		events, _ := sessionTestFlow(t, "", steps, 0, false)
		for _, e := range events {
			r := &liveBoundedReader{bytes.NewReader(e.Raw), uint64(len(e.Raw)) * 8}
			node, err := binparser.ParseBinary(r, e.Rule, e.Entry)
			require.NoError(t, err)
			require.Zero(t, r.Len())
			require.Equal(t, e.Fields, stream_parser.NodeToMap(node), e.Entry)
			if e.Entry == "MySQLAuthResponseFields" {
				continue
			}
			_, err = binparser.ParseStructured(e.Raw[:len(e.Raw)-1], e.Rule, e.Entry)
			require.Error(t, err, e.Entry)
		}
	}
}

func TestLiveProtocolPCAP(t *testing.T) {
	for _, name := range []string{"http2-multiplex", "mysql-classic", "mysql-deprecated-eof", "mysql-tracked-eof"} {
		t.Run(name, func(t *testing.T) {
			steps, port, protocol := h2TestFixtures(t), layers.TCPPort(18080), "http2"
			if name != "http2-multiplex" {
				steps = mysqlTestFixtures(name != "mysql-classic", name == "mysql-tracked-eof")
				port = 13306
				protocol = "mysql"
			}
			golden := sessionTestPCAP(t, steps, port, 7, false, false)
			path := filepath.Join("testdata", "protocol-sessions", name+".pcap")
			if os.Getenv("YAK_UPDATE_SESSION_PCAP") == "1" {
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
				require.NoError(t, os.WriteFile(path, golden, 0644))
			}
			stored, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, hex.EncodeToString(golden), hex.EncodeToString(stored), "fixture generation drift")
			for _, workers := range []int{1, 2, 4} {
				for _, ipv6 := range []bool{false, true} {
					for _, ng := range []bool{false, true} {
						wire := stored
						if ipv6 || ng {
							wire = sessionTestPCAP(t, steps, port, 7, ipv6, ng)
						}
						events, stats, err := binReplay(t, wire, workers)
						require.NoError(t, err)
						assertSessionEvents(t, events, protocol, false)
						require.Zero(t, stats.BufferedBytes)
						require.Equal(t, uint64(len(events)), stats.Decoded)
					}
				}
			}
		})
	}
}
