package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func syslog5424Message() []byte {
	return []byte("<165>1 2003-08-24T05:14:15.000003-07:00 192.0.2.1 myproc 8710 ID47 [meta@32473 key=\"a\\\"b\" slash=\"c\\\\d\" close=\"e\\]f\" repeat=\"one\" repeat=\"two\"] \xef\xbb\xbf%% It's time\nnext")
}

func syslogRFC3164Message() []byte {
	return []byte("<34>Oct 11 22:14:15 mymachine su[42]: test log")
}

func syslogOctetFrame(message []byte) []byte {
	return append([]byte(fmt.Sprintf("%d ", len(message))), message...)
}

func TestProtocolSessionSyslogTCPProbeAndFragmentation(t *testing.T) {
	cases := []struct {
		name, profile string
		version       any
		wire          []byte
	}{
		{name: "rfc3164-lf", profile: "syslog-rfc3164-bounded", wire: append(append([]byte(nil), syslogRFC3164Message()...), '\n')},
		{name: "rfc5424-octet-count", profile: "syslog-rfc5424-v1", version: 1, wire: syslogOctetFrame(syslog5424Message())},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			session, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			probe := session.Probe(tc.wire)
			require.Equal(t, ProbeAccept, probe.Verdict)
			require.Equal(t, "syslog", probe.Protocol)

			var events []*ProtocolEvent
			for i, b := range tc.wire {
				result := session.Feed(0, time.Unix(1, int64(i)), []byte{b})
				if result.Err != nil {
					require.Equal(t, ErrNeedMore, result.Err.Kind, "byte %d: %v", i, result.Err)
				}
				events = append(events, result.Events...)
			}
			require.Len(t, events, 1)
			require.Equal(t, "syslog", events[0].Protocol)
			require.Equal(t, tc.profile, events[0].Profile)
			fields, err := events[0].GetFields()
			require.NoError(t, err)
			require.Equal(t, tc.version, fields["Version"])
			if tc.name == "rfc3164-lf" {
				require.Equal(t, 34, fields["PRI"])
				require.Contains(t, fields["Message Display"], "su[42]: test log")
			}
		})
	}
}

func TestFirstBatchT19(t *testing.T) {
	t.Run("rfc5424-structured-data-bom-timestamp", func(t *testing.T) {
		fields, err := parseSyslogMessage(syslog5424Message(), 64)
		require.NoError(t, err)
		require.Equal(t, 165, fields["PRI"])
		require.Equal(t, 20, fields["Facility"])
		require.Equal(t, 5, fields["Severity"])
		require.Equal(t, "notice", fields["Severity Name"])
		require.Equal(t, 1, fields["Version"])
		require.Equal(t, "2003-08-24T05:14:15.000003-07:00", fields["Declared Timestamp"])
		require.Equal(t, "2003-08-24T12:14:15.000003Z", fields["Timestamp UTC"])
		require.Equal(t, "declared-offset", fields["Time Confidence"])
		require.Equal(t, "192.0.2.1", fields["Hostname"])
		require.Equal(t, "myproc", fields["App Name"])
		require.Equal(t, "8710", fields["ProcID"])
		require.Equal(t, "ID47", fields["MsgID"])
		sd := fields["Structured Data"].([]map[string]any)
		require.Len(t, sd, 1)
		require.Equal(t, "meta@32473", sd[0]["ID"])
		require.Equal(t, []string{"one", "two"}, sd[0]["Parameters By Name"].(map[string][]string)["repeat"])
		require.Equal(t, "a\"b", sd[0]["Parameters By Name"].(map[string][]string)["key"][0])
		require.Equal(t, "c\\d", sd[0]["Parameters By Name"].(map[string][]string)["slash"][0])
		require.Equal(t, "e]f", sd[0]["Parameters By Name"].(map[string][]string)["close"][0])
		require.Equal(t, "utf-8-bom", fields["Message Encoding"])
		require.Equal(t, []byte("\xef\xbb\xbf%% It's time\nnext"), fields["Message Bytes"])
		require.Contains(t, fields["Message Display"], `\n`)
	})

	t.Run("bounded-rfc3164-time-confidence", func(t *testing.T) {
		fields, err := parseSyslogMessage(syslogRFC3164Message(), 64)
		require.NoError(t, err)
		require.Equal(t, "Oct 11 22:14:15", fields["Declared Timestamp"])
		require.Equal(t, "partial-no-year-or-zone", fields["Time Confidence"])
		require.Nil(t, fields["Timestamp UTC"])
		require.Equal(t, "mymachine", fields["Hostname"])
		require.Equal(t, `"su[42]: test log"`, fields["Message Display"])
	})

	t.Run("rfc5424-invalid-and-budget-boundaries", func(t *testing.T) {
		for _, raw := range [][]byte{
			[]byte("<192>1 2003-08-24T05:14:15Z host app proc id - msg"),
			[]byte("<34>1 2003-08-24T05:14:15Z host app proc id [bad x=\"unterminated]"),
			[]byte("<34>1 2003-08-24T05:14:15Z host app proc id [x a=\"bad\\q\"]"),
			[]byte("<34>1 2003-08-24T05:14:15Z host app proc id [x a=\"\\xff\"]"),
		} {
			_, err := parseSyslogMessage(raw, 64)
			require.Error(t, err)
		}
		many := []byte("<34>1 2003-08-24T05:14:15Z host app proc id [x a=\"1\" b=\"2\"]")
		_, err := parseSyslogMessage(many, 1)
		require.ErrorContains(t, err, "budget")
		aggregate := []byte("<34>1 2003-08-24T05:14:15Z host app proc id [x a=\"1\"][y b=\"2\"]")
		_, err = parseSyslogMessage(aggregate, 3)
		require.ErrorContains(t, err, "budget", "the collection limit covers SD elements and parameters across the full message")
		_, err = parseSyslogMessage([]byte("<34>1 2025-13-40T99:99:99Z host app proc id - msg"), 64)
		require.Error(t, err)
	})

	t.Run("native-upstream-udp-capture", func(t *testing.T) {
		capture := binCorpusBytes(t, "ndpi/ndpi-syslog.pcap")
		sum := sha256.Sum256(capture)
		require.Equal(t, "f53a4f5de002f385107fb0077d898107ce041e5e11105b53ea051b57156d1c4b", hex.EncodeToString(sum[:]))
		events, stats, err := binReplay(t, capture, 2)
		require.NoError(t, err)
		require.Zero(t, stats.BufferedBytes)
		var syslogEvents, udpEvents, tcpEvents, decodedEvents, unsupportedVersion int
		for _, e := range events {
			if e.Protocol != "syslog" {
				continue
			}
			syslogEvents++
			switch e.Transport {
			case "udp":
				udpEvents++
			case "tcp":
				tcpEvents++
				require.Equal(t, "tcp-non-transparent-lf", e.Session["Framing"])
				require.Contains(t, string(e.Raw), "iPhone7,2 com.dts.freefireth")
				require.Equal(t, 1, e.Session["Version"])
				require.Equal(t, "iPhone7,2", e.Session["Hostname"])
				require.Equal(t, "com.dts.freefireth", e.Session["App Name"])
				require.Equal(t, "2021-04-18T14:51:48+04:00", e.Session["Declared Timestamp"])
			}
			if e.Status == "context-required" && bytes.Contains(e.Raw, []byte(">854 ")) {
				unsupportedVersion++
				require.Contains(t, e.Error, "UnsupportedVersion")
			} else {
				require.Equal(t, "decoded", e.Status, "unexpected upstream Syslog failure: %s raw=%q", e.Error, e.Raw)
			}
			if e.Transport == "udp" {
				require.Equal(t, "udp-datagram", e.Session["Framing"])
			}
			if e.Status == "decoded" {
				decodedEvents++
				require.Equal(t, "unverified-message-content", e.Session["Declared Sender"])
			}
			require.NotEmpty(t, e.SourceBytes.PacketRefs)
		}
		// The pcap contains 83 UDP datagrams on the well-known Syslog port,
		// but only 29 have a bounded PRI/header signature accepted by this
		// profile. Keep the remaining vendor/plain variants opaque instead
		// of treating the port or Wireshark's port heuristic as truth. One
		// complete RFC 5424 message is carried over TCP on a high port.
		reader, err := pcapgo.NewReader(bytes.NewReader(capture))
		require.NoError(t, err)
		var port514Datagrams int
		for {
			packetBytes, _, readErr := reader.ReadPacketData()
			if readErr != nil {
				break
			}
			packet := gopacket.NewPacket(packetBytes, reader.LinkType(), gopacket.Lazy)
			if layer := packet.Layer(layers.LayerTypeUDP); layer != nil {
				udp := layer.(*layers.UDP)
				if udp.SrcPort == 514 || udp.DstPort == 514 {
					port514Datagrams++
				}
			}
		}
		require.Equal(t, 83, port514Datagrams, "the fixed capture contains 83 UDP datagrams involving port 514")
		require.Equal(t, 30, syslogEvents, "bounded UDP signatures plus one independently valid TCP RFC 5424 message are admitted")
		require.Equal(t, 29, udpEvents)
		require.Equal(t, 1, tcpEvents)
		require.Equal(t, 15, decodedEvents, "14 UDP messages and one TCP message use the supported RFC 5424 v1/RFC 3164 profiles")
		require.Equal(t, 15, unsupportedVersion, "the capture repeats a vendor frame declaring unsupported VERSION 854")
	})
	t.Run("live-rsyslog-udp-tcp-pcap", func(t *testing.T) {
		capture, err := os.ReadFile("testdata/protocol-sessions/first-batch-oracles/net-snmp-rsyslog-interop-v2c.pcap")
		require.NoError(t, err)
		sum := sha256.Sum256(capture)
		require.Equal(t, "10087cb684d98acad8a2da7e95620edd8e9787176a84344d5b5ff69daa0dfb18", hex.EncodeToString(sum[:]))
		oracle, err := os.ReadFile("testdata/protocol-sessions/first-batch-oracles/rsyslog-received.log")
		require.NoError(t, err)
		for _, accepted := range []string{"udp-rfc5424", "udp-rfc3164", "tcp-octet-count-one", "tcp-octet-count-two", "#012continued"} {
			require.Contains(t, string(oracle), accepted, "independent rsyslog 8.2302.0 received the captured payload")
		}
		for _, deferred := range []bool{false, true} {
			for _, workers := range []int{1, 2, 4} {
				t.Run(fmt.Sprintf("deferred=%v/workers=%d", deferred, workers), func(t *testing.T) {
					var events []*ProtocolEvent
					var stats BinParserStats
					err := ReplayPcap(bytes.NewReader(capture), WithProtocolDecodeAs("udp", 1514, "syslog"), WithTCPReassemblyWorkers(workers), WithBinParserConfig(BinParserConfig{Deferred: deferred, OnEvent: func(e *ProtocolEvent) { events = append(events, e) }, OnStats: func(s BinParserStats) { stats = s }}))
					require.NoError(t, err)
					require.Zero(t, stats.BufferedBytes)
					wantStatus := "decoded"
					if deferred {
						wantStatus = "deferred"
					}
					transportCount := map[string]int{}
					bodyCount := map[string]int{}
					for _, event := range events {
						if event.Protocol != "syslog" {
							continue
						}
						transportCount[event.Transport]++
						require.Equal(t, wantStatus, event.Status)
						require.NotEmpty(t, event.SourceBytes.PacketRefs)
						if deferred {
							require.Nil(t, event.semanticFields, "capture-first mode must not predecode Syslog")
							require.Nil(t, event.Session, "stateless Syslog does not need an eager session snapshot")
							require.Nil(t, event.Structured)
						}
						fields, decodeErr := event.GetFields()
						require.NoError(t, decodeErr)
						body, ok := fields["Message Bytes"].([]byte)
						require.True(t, ok)
						bodyCount[string(body)]++
						wantProfile := syslogProfileForWire(event.Raw, syslogDatagram)
						if event.Transport == "tcp" {
							wantProfile = syslogProfileForWire(event.Raw, event.syslogFraming)
						}
						require.Equal(t, wantProfile, event.Profile)
						if event.Transport == "udp" {
							require.Equal(t, "explicit-decode-as", event.Admission)
						}
					}
					require.Equal(t, map[string]int{"udp": 2, "tcp": 2}, transportCount)
					require.Equal(t, 1, bodyCount["udp-rfc5424"])
					require.Equal(t, 1, bodyCount["su[42]: udp-rfc3164"])
					require.Equal(t, 1, bodyCount["tcp-octet-count-one\ncontinued"], "octet counting preserves embedded LF")
					require.Equal(t, 1, bodyCount["su[42]: tcp-octet-count-two"])
				})
			}
		}
	})

	t.Run("udp-decode-as-and-no-plain-text-false-positive", func(t *testing.T) {
		valid := syslog5424Message()
		var events []*ProtocolEvent
		pcap := sessionDatagramPCAP(t, []sessionStep{{dir: 0, wire: valid}}, 4053)
		err := ReplayPcap(bytes.NewReader(pcap), WithProtocolDecodeAs("udp", 4053, "syslog"), WithTCPReassemblyWorkers(1), WithBinParser(func(e *ProtocolEvent) { events = append(events, e) }))
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, "syslog", events[0].Protocol)
		require.Equal(t, "explicit-decode-as", events[0].Admission)
		require.Equal(t, "myproc", events[0].Session["App Name"])

		plain := sessionDatagramPCAP(t, []sessionStep{{dir: 0, wire: []byte("ordinary printable application text\n")}}, 514)
		plainEvents, _, err := binReplay(t, plain, 1)
		require.NoError(t, err)
		for _, e := range plainEvents {
			require.NotEqual(t, "syslog", e.Protocol)
		}
		wrongPRI := sessionDatagramPCAP(t, []sessionStep{{dir: 0, wire: []byte("<999>1 2003-08-24T05:14:15Z host app proc id - msg")}}, 514)
		wrongEvents, _, err := binReplay(t, wrongPRI, 1)
		require.NoError(t, err)
		for _, e := range wrongEvents {
			require.NotEqual(t, "syslog", e.Protocol)
		}
	})

	t.Run("aggregate-collection-and-udp-message-byte-budgets", func(t *testing.T) {
		wire := syslog5424Message()
		_, err := (&binSyslog{}).consume(wire, syslogDatagram, len(wire)-1, 64)
		require.NotNil(t, err)
		require.Equal(t, ErrResourceExceeded, err.(*ProtocolError).Kind)
		for _, deferred := range []bool{false, true} {
			var events []*ProtocolEvent
			var stats BinParserStats
			pcap := sessionDatagramPCAP(t, []sessionStep{{dir: 0, wire: wire}}, 514)
			err := ReplayPcap(bytes.NewReader(pcap), WithBinParserConfig(BinParserConfig{
				MaxMessageBytes: 64,
				Deferred:        deferred,
				OnEvent:         func(e *ProtocolEvent) { events = append(events, e) },
				OnStats:         func(s BinParserStats) { stats = s },
			}))
			require.NoError(t, err)
			require.Len(t, events, 1)
			require.Equal(t, "limited", events[0].Status, "capture ingress must enforce the common UDP byte ceiling before protocol admission")
			require.Empty(t, events[0].Protocol)
			require.EqualValues(t, len(wire), stats.LimitedBytes)
			require.Zero(t, stats.BufferedBytes)
		}
	})

	t.Run("tcp-octet-counting-embedded-newline-full-deferred", func(t *testing.T) {
		m1 := syslog5424Message()
		m2 := []byte("<13>1 2025-01-02T03:04:05Z - - - - - second")
		stream := append(syslogOctetFrame(m1), syslogOctetFrame(m2)...)
		steps := []sessionStep{{dir: 0, wire: stream}}
		for _, deferred := range []bool{false, true} {
			for _, chunk := range []int{0, 1, 7} {
				for _, workers := range []int{1, 2, 4} {
					t.Run(fmt.Sprintf("deferred=%v/chunk=%d/workers=%d", deferred, chunk, workers), func(t *testing.T) {
						var events []*ProtocolEvent
						var stats BinParserStats
						pcap := sessionTestPCAP(t, steps, layers.TCPPort(1514), chunk, false, false)
						err := ReplayPcap(bytes.NewReader(pcap), WithTCPReassemblyWorkers(workers), WithBinParserConfig(BinParserConfig{Deferred: deferred, OnEvent: func(e *ProtocolEvent) { events = append(events, e) }, OnStats: func(s BinParserStats) { stats = s }}))
						require.NoError(t, err)
						require.Len(t, events, 2)
						require.Zero(t, stats.BufferedBytes)
						for eventIndex, e := range events {
							if deferred {
								require.Equal(t, "deferred", e.Status)
								require.Nil(t, e.semanticFields, "capture-first mode must leave Syslog semantics for Decode")
								require.Nil(t, e.Session)
							} else {
								require.Equal(t, "decoded", e.Status)
								require.Equal(t, "tcp-octet-counting", e.Session["Framing"])
								if eventIndex == 0 {
									require.Equal(t, "myproc", e.Session["App Name"])
								}
							}
							require.Equal(t, "syslog", e.Protocol)
							require.Equal(t, "message", e.Completeness)
							decoded, decodeErr := e.GetFields()
							require.NoError(t, decodeErr)
							require.Equal(t, "tcp-octet-counting", decoded["Framing"])
							wantPRI := 165
							if eventIndex == 1 {
								wantPRI = 13
							}
							require.Equal(t, wantPRI, decoded["PRI"])
							if eventIndex == 0 {
								require.Equal(t, "myproc", decoded["App Name"])
							}
						}
						require.EqualValues(t, 2, stats.Messages)
						if deferred {
							require.EqualValues(t, 2, stats.Deferred)
						} else {
							require.EqualValues(t, 2, stats.Decoded)
						}
					})
				}
			}
		}
	})

	t.Run("tcp-non-transparent-lf-and-truncated-octet-frame", func(t *testing.T) {
		legacy := append(bytes.Clone(syslogRFC3164Message()), '\n')
		steps := []sessionStep{{dir: 0, wire: legacy}}
		events, stats, err := binReplay(t, sessionTestPCAP(t, steps, 1514, 1, false, false), 2)
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, "syslog", events[0].Protocol)
		require.Equal(t, "tcp-non-transparent-lf", events[0].Session["Framing"])
		require.Equal(t, "partial-no-year-or-zone", events[0].Session["Time Confidence"])
		require.EqualValues(t, 1, stats.Decoded)

		partial := syslogOctetFrame(syslog5424Message())
		partial = partial[:len(partial)-4]
		events, stats, err = binReplay(t, sessionTestPCAP(t, []sessionStep{{dir: 0, wire: partial}}, 1514, 0, false, false), 1)
		require.NoError(t, err)
		var incomplete int
		for _, e := range events {
			if e.Protocol == "syslog" && e.Status == "incomplete" {
				incomplete++
			}
		}
		require.Equal(t, 1, incomplete)
		require.Zero(t, stats.BufferedBytes)
	})
}

func BenchmarkFirstBatchSyslog(b *testing.B) {
	b.Run("semantic/rfc5424", func(b *testing.B) {
		wire := syslog5424Message()
		b.SetBytes(int64(len(wire)))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := parseSyslogMessage(wire, DefaultParserBudget().MaxCollectionElements); err != nil {
				b.Fatal(err)
			}
		}
	})

	capture, err := os.ReadFile("testdata/protocol-sessions/first-batch-oracles/net-snmp-rsyslog-interop-v2c.pcap")
	if err != nil {
		b.Fatal(err)
	}
	for _, mode := range []struct {
		name     string
		deferred bool
		decode   bool
	}{
		{name: "full"},
		{name: "deferred-capture", deferred: true},
		{name: "deferred-on-demand", deferred: true, decode: true},
	} {
		for _, workers := range []int{1, 2, 4} {
			b.Run(fmt.Sprintf("replay/%s/workers-%d", mode.name, workers), func(b *testing.B) {
				b.SetBytes(int64(len(capture)))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var count atomic.Int64
					var eventMu sync.Mutex
					var eventErr error
					err := ReplayPcap(bytes.NewReader(capture), WithProtocolDecodeAs("udp", 1514, "syslog"), WithTCPReassemblyWorkers(workers), WithBinParserConfig(BinParserConfig{
						Deferred: mode.deferred,
						OnEvent: func(event *ProtocolEvent) {
							if event.Protocol != "syslog" {
								return
							}
							count.Add(1)
							if mode.decode {
								if _, decodeErr := event.GetFields(); decodeErr != nil {
									eventMu.Lock()
									if eventErr == nil {
										eventErr = decodeErr
									}
									eventMu.Unlock()
								}
							}
						},
					}))
					if err != nil {
						b.Fatal(err)
					}
					eventMu.Lock()
					callbackErr := eventErr
					eventMu.Unlock()
					if callbackErr != nil {
						b.Fatal(callbackErr)
					}
					if count.Load() != 4 {
						b.Fatalf("expected 4 Syslog messages, got %d", count.Load())
					}
				}
				b.ReportMetric(4, "events/op")
			})
		}
	}
}

func FuzzSyslogMessageBounded(f *testing.F) {
	f.Add(syslog5424Message())
	f.Add(syslogRFC3164Message())
	f.Add([]byte("<34>1 bad"))
	f.Add([]byte("ordinary text"))
	f.Fuzz(func(t *testing.T, wire []byte) {
		if len(wire) > 1<<20 {
			t.Skip()
		}
		_, _ = parseSyslogMessage(wire, 4096)
	})
}
