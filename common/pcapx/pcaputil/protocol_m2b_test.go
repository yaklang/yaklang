package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"sort"
	"testing"
)

func m2bFixture(t testing.TB, name string) []byte {
	t.Helper()
	b, e := os.ReadFile("testdata/protocol-sessions/" + name)
	require.NoError(t, e)
	return b
}
func m2bReplay(t testing.TB, name string, deferred bool, workers int, secrets TLSSecretProvider) []*ProtocolEvent {
	t.Helper()
	var events []*ProtocolEvent
	opts := []CaptureOption{WithProtocolDeferred(deferred), WithTCPReassemblyWorkers(workers), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })}
	if secrets != nil {
		opts = append(opts, WithTLSSecrets(secrets))
	}
	require.NoError(t, ReplayPcap(bytes.NewReader(m2bFixture(t, name)), opts...))
	return events
}
func m2bRows(events []*ProtocolEvent) []string {
	var rows []string
	for _, e := range events {
		if e.Protocol == "mysql" && e.Session["Profile"] == "prepared" {
			if v, ok := e.Session["Values"]; ok {
				b, _ := json.Marshal(v)
				rows = append(rows, string(b))
			}
		}
	}
	sort.Strings(rows)
	return rows
}
func TestFirstBatchT16(t *testing.T) {
	var baseline []string
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("native-workers%d-deferred%v", workers, deferred), func(t *testing.T) {
				events := m2bReplay(t, "m2b-database-native.pcap", deferred, workers, nil)
				prepared, execute, close, reset := 0, 0, 0, 0
				clients := map[uint64]bool{}
				for _, e := range events {
					require.Contains(t, []string{"decoded", "deferred"}, e.Status, e.Error)
					if e.Protocol != "mysql" {
						continue
					}
					switch e.Session["Command"] {
					case "COM_STMT_PREPARE":
						prepared++
						clients[e.FlowID] = true
					case "COM_STMT_EXECUTE":
						execute++
					case "COM_STMT_CLOSE":
						close++
						require.Equal(t, false, e.Session["Response Expected"])
					case "COM_STMT_RESET":
						reset++
					}
					if e.Session["Profile"] == "prepared" {
						_, err := e.Decode()
						require.NoError(t, err)
						if e.Direction == 1 {
							require.NotZero(t, e.ResponseTo)
						}
					}
				}
				require.Equal(t, 2, prepared)
				require.Equal(t, 4, execute)
				require.Equal(t, 2, close)
				require.GreaterOrEqual(t, reset, 2)
				require.Len(t, clients, 2)
				rows := m2bRows(events)
				require.Len(t, rows, 4)
				if baseline == nil {
					baseline = rows
				} else {
					require.Equal(t, baseline, rows)
				}
				require.Contains(t, fmt.Sprint(rows), `"Value":42`)
				require.Contains(t, fmt.Sprint(rows), `"Value":-7`)
				require.Contains(t, fmt.Sprint(rows), `"Value":17`)
				require.Contains(t, fmt.Sprint(rows), `"Value":-9`)
				require.Contains(t, fmt.Sprint(rows), `"NULL":true`)
			})
		}
	}
	t.Run("fragmented-native-packets", func(t *testing.T) {
		events := m2bReplay(t, "m2b-database-native.pcap", false, 1, nil)
		flows := map[uint64][]sessionStep{}
		for _, e := range events {
			if e.Protocol == "mysql" {
				flows[e.FlowID] = append(flows[e.FlowID], sessionStep{e.Direction, e.Raw})
			}
		}
		for _, steps := range flows {
			for _, chunk := range []int{1, 2, 3, 7, 31} {
				ev, _ := sessionTestFlow(t, "mysql", steps, chunk, true)
				assertSessionEvents(t, ev, "mysql", true)
				require.Len(t, m2bRows(ev), 2)
			}
		}
	})
}
func TestFirstBatchT17(t *testing.T) {
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("native-workers%d-deferred%v", workers, deferred), func(t *testing.T) {
				events := m2bReplay(t, "m2b-database-native.pcap", deferred, workers, nil)
				counts := map[string]int{}
				associated, failed, ready := 0, 0, 0
				for _, e := range events {
					if e.Protocol != "postgresql" {
						continue
					}
					require.Contains(t, []string{"decoded", "deferred"}, e.Status, e.Error)
					name, _ := e.Session["Message Name"].(string)
					counts[name]++
					if name == "DataRow" && e.Session["Operation"] == "Execute" {
						require.NotEmpty(t, e.Session["Statement"])
						require.NotZero(t, e.ResponseTo)
						require.Equal(t, true, e.Session["Row Metadata Applied"])
						associated++
					}
					if name == "ErrorResponse" {
						require.Equal(t, true, e.Session["Operation Failed"])
						failed++
					}
					if name == "ReadyForQuery" && e.Session["Session State Validated"] == true {
						ready++
					}
				}
				require.GreaterOrEqual(t, associated, 5)
				require.Equal(t, 2, failed)
				require.Greater(t, ready, failed)
				for _, n := range []string{"Parse", "Bind", "Execute", "Sync", "CopyInResponse", "CopyOutResponse", "CopyData", "CopyDone", "CopyFail", "NoticeResponse"} {
					require.Positive(t, counts[n], n)
				}
			})
		}
	}
	t.Run("fragmented", func(t *testing.T) {
		var steps []sessionStep
		for _, e := range m2bReplay(t, "m2b-database-native.pcap", false, 1, nil) {
			if e.Protocol == "postgresql" {
				steps = append(steps, sessionStep{e.Direction, e.Raw})
			}
		}
		for _, n := range []int{1, 2, 7, 31} {
			ev, _ := sessionTestFlow(t, "postgresql", steps, n, true)
			assertSessionEvents(t, ev, "postgresql", true)
		}
	})
}
func testFirstBatchM2T12(t *testing.T) {
	keys, err := ParseTLSKeyLog(string(m2bFixture(t, "m2b-quic.keys")))
	require.NoError(t, err)
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("native-workers%d-deferred%v", workers, deferred), func(t *testing.T) {
				events := m2bReplay(t, "m2b-quic-native.pcap", deferred, workers, keys)
				flows := map[uint64]string{}
				phases := map[uint64]bool{}
				initial, handshake, app := 0, 0, 0
				for _, e := range events {
					require.Contains(t, []string{"quic", "http3", "doq"}, e.Protocol)
					require.Contains(t, []string{"decoded", "deferred"}, e.Status, e.Error)
					require.Equal(t, true, e.Session["Authentication Verified"])
					require.NotEqual(t, true, e.Session["Plaintext Input"])
					switch e.Session["Packet Number Space"] {
					case "initial":
						initial++
					case "handshake":
						handshake++
					case "application":
						app++
					}
					if alpn, _ := e.Session["ALPN"].(string); alpn != "" {
						if old := flows[e.FlowID]; old != "" {
							require.Equal(t, old, alpn)
						}
						flows[e.FlowID] = alpn
					}
					if e.Session["Key Phase"] == 1 {
						phases[e.FlowID] = true
					}
				}
				require.Len(t, flows, 2)
				require.Len(t, phases, 2)
				require.Positive(t, initial)
				require.Positive(t, handshake)
				require.Positive(t, app)
			})
		}
	}
	t.Run("no-key-opaque", func(t *testing.T) {
		encrypted := 0
		for _, e := range m2bReplay(t, "m2b-quic-native.pcap", false, 1, nil) {
			require.NotEqual(t, true, e.Session["HTTP3"])
			require.NotEqual(t, true, e.Session["DoQ"])
			if e.Session["Encrypted"] == true && e.Status == "context-required" {
				encrypted++
			}
		}
		require.Positive(t, encrypted)
	})
	t.Run("ordered-once", func(t *testing.T) {
		var a quicAssembler
		b, _, done, err := a.feed(3, []byte("def"), true, 64, 4)
		require.NoError(t, err)
		require.Empty(t, b)
		require.False(t, done)
		b, off, done, err := a.feed(0, []byte("abc"), false, 64, 4)
		require.NoError(t, err)
		require.Equal(t, uint64(0), off)
		require.Equal(t, []byte("abcdef"), b)
		require.True(t, done)
		b, _, done, err = a.feed(3, []byte("def"), true, 64, 4)
		require.NoError(t, err)
		require.Empty(t, b)
		require.False(t, done)
		_, _, _, err = a.feed(3, []byte("bad"), false, 64, 4)
		require.Error(t, err)
		_, _, _, err = a.feed(64, []byte("x"), false, 64, 4)
		require.Error(t, err)
	})
}
func TestFirstBatchT13(t *testing.T) {
	keys, err := ParseTLSKeyLog(string(m2bFixture(t, "m2b-quic.keys")))
	require.NoError(t, err)
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("native-workers%d-deferred%v", workers, deferred), func(t *testing.T) {
				requests, responses, trailers, dns := 0, 0, 0, 0
				for _, e := range m2bReplay(t, "m2b-quic-native.pcap", deferred, workers, keys) {
					require.Contains(t, []string{"decoded", "deferred"}, e.Status, e.Error)
					frames, _ := e.Session["Frames"].([]map[string]any)
					for _, fr := range frames {
						hf, _ := fr["HTTP3 Frames"].([]map[string]any)
						for _, v := range hf {
							switch v["Header Kind"] {
							case "request":
								requests++
							case "response":
								responses++
								require.Equal(t, true, v["Associated Request"])
							case "trailers":
								trailers++
							}
						}
					}
					if e.Session["DoQ"] == true {
						dns++
						v := e.Session["DoQ Message"].(map[string]any)
						require.Equal(t, uint16(0), v["Transaction ID"])
						require.NotNil(t, v["DNS"])
						if e.Direction == 1 {
							require.Equal(t, "matched", v["Association Status"])
						}
					}
				}
				require.Equal(t, 2, requests)
				require.Equal(t, 2, responses)
				require.Equal(t, 2, trailers)
				require.Equal(t, 4, dns)
			})
		}
	}
	t.Run("blocked-resumes", func(t *testing.T) {
		q := &binQUIC{native: true, byteLimit: 4096, alpn: "h3", eventID: 42}
		feed := func(dir int, id, off uint64, data []byte, fin bool) map[string]any {
			info := map[string]any{}
			err := q.feedHTTP3(dir, id, off, data, fin, 64, map[string]any{}, info)
			require.NoError(t, err)
			return info
		}
		feed(1, 3, 0, h3ControlSETTINGS(1, 220, 7, 1), false)
		info := feed(0, 0, 0, h3Frame(1, rfcHex("03811011")), true)
		require.Equal(t, true, info["QPACK Blocked"])
		info = feed(0, 6, 0, h3EncoderStream(rfcHex("3fbd01c00f7777772e6578616d706c652e636f6dc10c2f73616d706c652f70617468")), false)
		resumed := info["QPACK Resumed Streams"].([]map[string]any)
		require.Len(t, resumed, 1)
		require.Equal(t, uint64(42), resumed[0]["Blocked PDU ID"])
		require.False(t, q.streams[0].h3.blocked[0])
		require.Equal(t, 1, q.streams[0].h3.headers[0])
	})
}

func TestM2FixtureManifest(t *testing.T) {
	var manifest struct {
		Files []struct{ Path, SHA256 string }
	}
	require.NoError(t, json.Unmarshal(m2bFixture(t, "m2b-manifest.json"), &manifest))
	require.Len(t, manifest.Files, 8)
	for _, f := range manifest.Files {
		sum := sha256.Sum256(m2bFixture(t, f.Path))
		require.Equal(t, f.SHA256, hex.EncodeToString(sum[:]), f.Path)
	}
}
