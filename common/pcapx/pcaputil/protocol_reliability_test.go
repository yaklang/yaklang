package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
	"time"
)

func reliabilityDNS(labels ...string) []byte {
	w := []byte{0x12, 0x34, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0}
	for _, s := range labels {
		w = append(w, byte(len(s)))
		w = append(w, s...)
	}
	return append(w, 0, 0, 1, 0, 1)
}
func TestDNSCanonicalLabels(t *testing.T) {
	key := func(w []byte) string {
		info, err := DecodeDNSMessage(w, 4096)
		require.NoError(t, err)
		return dnsQuestionKey(info)
	}
	require.NotEqual(t, key(reliabilityDNS("a.b")), key(reliabilityDNS("a", "b")))
	require.Equal(t, key(reliabilityDNS("EXample")), key(reliabilityDNS("example")))
	require.NotEqual(t, key(reliabilityDNS("Ä")), key(reliabilityDNS("ä")))
	info, err := DecodeDNSMessage(reliabilityDNS("a.b", `c\d`), 4096)
	require.NoError(t, err)
	require.Equal(t, `a\.b.c\\d`, info["Questions"].([]map[string]any)[0]["Name"])
	w := append([]byte{3, 'A', 'b', 'C', 0}, 0xc0, 0)
	_, _, a, err := dnsParseNameWire(w, 0)
	require.NoError(t, err)
	_, _, b, err := dnsParseNameWire(w, 5)
	require.NoError(t, err)
	require.Equal(t, a, b)
}
func TestDNSCarrierAssociation(t *testing.T) {
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	a := c.binParser
	defer func() { a.closeUDPSessions(); require.Zero(t, a.buffered.Load()) }()
	emit := func(proto, transport, src, dst string, flow uint64, response bool, domain CaptureDomain) *ProtocolEvent {
		w := reliabilityDNS("test")
		if response {
			w[2] = 0x81
		}
		e := &ProtocolEvent{Protocol: proto, Transport: transport, Source: src, Destination: dst, FlowID: flow, Timestamp: time.Unix(1, 0), Domain: domain}
		require.NoError(t, a.dnsEvent(e, w))
		return e
	}
	d := CaptureDomain{}
	q := emit("dns", "tcp", "a:10", "b:53", 1, false, d)
	require.Zero(t, emit("dns", "tcp", "b:53", "a:10", 2, true, d).ResponseTo)
	require.Equal(t, q.ID, emit("dns", "tcp", "b:53", "a:10", 1, true, d).ResponseTo)
	emit("dns", "tcp", "a:10", "b:53", 3, false, d)
	a.closeDNSFlow(3)
	require.Zero(t, emit("dns", "tcp", "b:53", "a:10", 3, true, d).ResponseTo)
	q = emit("dns", "udp", "a:10", "b:53", 0, false, d)
	require.Zero(t, emit("dns", "udp", "c:53", "a:10", 0, true, d).ResponseTo)
	require.Equal(t, q.ID, emit("dns", "udp", "b:53", "a:10", 0, true, d).ResponseTo)
	for _, group := range []string{"224.0.0.252:5355", "[ff02::1:3]:5355"} {
		requester := "192.0.2.1:1234"
		r1, r2 := "192.0.2.2:5355", "192.0.2.3:5355"
		if group[0] == '[' {
			requester = "[fe80::1]:1234"
			r1 = "[fe80::2]:5355"
			r2 = "[fe80::3]:5355"
		}
		q = emit("llmnr", "udp", requester, group, 0, false, d)
		require.Zero(t, emit("llmnr", "udp", r1, requester, 0, true, CaptureDomain{Section: 1}).ResponseTo)
		one := emit("llmnr", "udp", r1, requester, 0, true, d)
		require.Equal(t, q.ID, one.ResponseTo)
		two := emit("llmnr", "udp", r2, requester, 0, true, d)
		require.Equal(t, q.ID, two.ResponseTo)
		require.Equal(t, 2, two.Session["Responder Count"])
		again := emit("llmnr", "udp", r1, requester, 0, true, d)
		require.Equal(t, true, again.Session["Retransmission"])
	}
}
func reliabilityRESP(args ...string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, arg := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(arg), arg)
	}
	return b.Bytes()
}
func TestRedisReplyModeSequence(t *testing.T) {
	cmd := func(args ...string) sessionStep { return sessionStep{0, reliabilityRESP(args...)} }
	res := func(s string) sessionStep { return sessionStep{1, []byte(s)} }
	cases := []struct {
		name      string
		steps     []sessionStep
		matches   map[int]int
		ambiguous int
	}{
		{"two-boundaries", []sessionStep{cmd("CLIENT", "REPLY", "OFF"), cmd("CLIENT", "REPLY", "ON"), cmd("CLIENT", "REPLY", "OFF"), cmd("CLIENT", "REPLY", "ON"), res("+OK\r\n"), res("-NOPERM denied\r\n")}, map[int]int{4: 1}, 5},
		{"off-on", []sessionStep{cmd("CLIENT", "REPLY", "OFF"), cmd("GET", "ignored"), cmd("CLIENT", "REPLY", "ON"), cmd("GET", "key"), res("+OK\r\n"), res("$1\r\nv\r\n")}, map[int]int{4: 2, 5: 3}, -1},
		{"skip", []sessionStep{cmd("CLIENT", "REPLY", "SKIP"), cmd("GET", "ignored"), cmd("GET", "key"), res("$1\r\nv\r\n")}, map[int]int{3: 2}, -1},
		{"repeat-skip", []sessionStep{cmd("CLIENT", "REPLY", "SKIP"), cmd("CLIENT", "REPLY", "SKIP"), cmd("GET", "ignored"), cmd("PING"), res("+PONG\r\n")}, map[int]int{4: 3}, -1},
		{"on-overrides-skip", []sessionStep{cmd("CLIENT", "REPLY", "SKIP"), cmd("CLIENT", "REPLY", "ON"), res("+OK\r\n")}, map[int]int{2: 1}, -1},
		{"prior-pipeline", []sessionStep{cmd("GET", "old"), cmd("CLIENT", "REPLY", "OFF"), cmd("GET", "ignored"), cmd("CLIENT", "REPLY", "ON"), res("-ERR old command\r\n"), res("+OK\r\n")}, map[int]int{4: 0, 5: 3}, -1},
		{"denied", []sessionStep{cmd("CLIENT", "REPLY", "OFF"), cmd("GET", "key"), res("-NOPERM denied\r\n"), res("$1\r\nv\r\n")}, nil, 2},
		{"next-error-ambiguous", []sessionStep{cmd("CLIENT", "REPLY", "SKIP"), cmd("GET", "ignored"), cmd("GET", "key"), res("-WRONGTYPE denied\r\n")}, nil, 3},
		{"push", []sessionStep{cmd("CLIENT", "REPLY", "SKIP"), cmd("GET", "ignored"), cmd("GET", "key"), res(">2\r\n+invalidate\r\n*0\r\n"), res("$1\r\nv\r\n")}, map[int]int{4: 2}, -1},
	}
	for _, tc := range cases {
		for _, chunk := range []int{0, 1, 7} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%d/%t", tc.name, chunk, deferred), func(t *testing.T) {
					ev, _ := sessionTestFlow(t, "redis", tc.steps, chunk, deferred)
					require.Len(t, ev, len(tc.steps))
					for _, e := range ev {
						require.Empty(t, e.Error)
					}
					for r, q := range tc.matches {
						require.Equal(t, ev[q].ID, ev[r].ResponseTo)
					}
					if tc.ambiguous >= 0 {
						require.Equal(t, "ambiguous-reply-mode", ev[tc.ambiguous].Session["Correlation Status"])
						for _, e := range ev[tc.ambiguous:] {
							require.Zero(t, e.ResponseTo)
						}
					}
				})
			}
		}
	}
}
func reliabilityHello(client, retry bool, group uint16, cookie []byte) []byte {
	b := make([]byte, 35)
	b[0], b[1] = 3, 3
	b[2] = 1
	if retry {
		copy(b[2:34], tlsRetryRandom[:])
	}
	if client {
		b = append(b, 0, 2, 0x13, 1, 1, 0)
	} else {
		b = append(b, 0x13, 1, 0)
	}
	var ext []byte
	add := func(k uint16, v []byte) {
		ext = binary.BigEndian.AppendUint16(ext, k)
		ext = binary.BigEndian.AppendUint16(ext, uint16(len(v)))
		ext = append(ext, v...)
	}
	if client {
		add(43, []byte{2, 3, 4})
		add(10, []byte{0, 4, 0, 29, 0, 23})
		add(51, []byte{0, 5, byte(group >> 8), byte(group), 0, 1, 1})
	} else {
		add(43, []byte{3, 4})
		if group != 0 {
			v := []byte{byte(group >> 8), byte(group)}
			if !retry {
				v = append(v, 0, 1, 1)
			}
			add(51, v)
		}
	}
	if cookie != nil {
		add(44, append([]byte{0, byte(len(cookie))}, cookie...))
	}
	b = binary.BigEndian.AppendUint16(b, uint16(len(ext)))
	return append(b, ext...)
}
func reliabilityHS(typ byte, b []byte) []byte {
	return append([]byte{typ, byte(len(b) >> 16), byte(len(b) >> 8), byte(len(b))}, b...)
}
func TestTLSHelloRetrySequence(t *testing.T) {
	for _, group := range []uint16{0, 23} {
		t.Run(fmt.Sprint(group), func(t *testing.T) {
			state := newBinTLS()
			cookie := []byte("cookie")
			ch1 := reliabilityHello(true, false, 29, nil)
			_, err := state.handshakeMessage(0, reliabilityHS(1, ch1), false)
			require.NoError(t, err)
			hrr := reliabilityHS(2, reliabilityHello(false, true, group, cookie))
			m, err := state.handshakeMessage(1, hrr, false)
			require.NoError(t, err)
			require.Equal(t, true, m["Hello Retry Request"])
			g := group
			if g == 0 {
				g = 29
			}
			ch2 := reliabilityHS(1, reliabilityHello(true, false, g, cookie))
			m, err = state.handshakeMessage(0, ch2, false)
			require.NoError(t, err)
			require.Equal(t, true, m["Retry ClientHello"])
			_, err = state.handshakeMessage(1, reliabilityHS(2, reliabilityHello(false, false, g, nil)), false)
			require.NoError(t, err)
			_, err = state.handshakeMessage(0, ch2, false)
			require.Error(t, err)
		})
	}
	for _, which := range []string{"repeat-hrr", "random", "cookie", "share", "early-final", "suite"} {
		t.Run(which, func(t *testing.T) {
			state := newBinTLS()
			_, err := state.handshakeMessage(0, reliabilityHS(1, reliabilityHello(true, false, 29, nil)), false)
			require.NoError(t, err)
			hrr := reliabilityHS(2, reliabilityHello(false, true, 23, []byte("cookie")))
			_, err = state.handshakeMessage(1, hrr, false)
			require.NoError(t, err)
			b := reliabilityHello(true, false, 23, []byte("cookie"))
			typ, dir := byte(1), 0
			switch which {
			case "repeat-hrr":
				b = hrr[4:]
				typ, dir = 2, 1
			case "random":
				b[2]++
			case "cookie":
				b[len(b)-1]++
			case "share":
				b = reliabilityHello(true, false, 29, []byte("cookie"))
			case "early-final":
				b = reliabilityHello(false, false, 23, nil)
				typ, dir = 2, 1
			case "suite":
				b[38]++
			}
			_, err = state.handshakeMessage(dir, reliabilityHS(typ, b), false)
			require.Error(t, err)
		})
	}
}

func TestReliabilityNativeReplay(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, workers := range []int{1, 4} {
			t.Run(fmt.Sprintf("%t/%d", deferred, workers), func(t *testing.T) {
				keys, err := os.ReadFile("testdata/protocol-sessions/reliability/tls-hrr.keys")
				require.NoError(t, err)
				secrets, err := ParseTLSKeyLog(string(keys))
				require.NoError(t, err)
				for _, hasKey := range []bool{false, true} {
					cap, err := os.ReadFile("testdata/protocol-sessions/reliability/tls-hrr.pcapng")
					require.NoError(t, err)
					opts := []CaptureOption{WithProtocolDeferred(deferred)}
					if hasKey {
						opts = append(opts, WithTLSSecrets(secrets))
					}
					ev, stats, err := binReplay(t, cap, workers, opts...)
					require.NoError(t, err)
					require.Zero(t, stats.BufferedBytes)
					hrr, ch2, authenticated := 0, 0, 0
					bodyFound := false
					for _, e := range ev {
						require.Empty(t, e.Error, "%s %s", e.Protocol, e.Summary)
						if e.Session["Authentication Verified"] == true {
							authenticated++
						}
						if rows, ok := e.Session["Handshake Messages"].([]map[string]any); ok {
							for _, row := range rows {
								if row["Hello Retry Request"] == true {
									hrr++
								}
								if row["Retry ClientHello"] == true {
									ch2++
								}
							}
						}
						if e.SourceBytes.Kind == "decrypted" && bytes.Contains(e.Raw, []byte("hello-retry-authenticated")) {
							bodyFound = true
						}
					}
					require.Equal(t, 1, hrr)
					require.Equal(t, 1, ch2)
					require.Equal(t, hasKey, bodyFound)
					if hasKey {
						require.Positive(t, authenticated)
					} else {
						require.Zero(t, authenticated)
					}
				}
				cap, err := os.ReadFile("testdata/protocol-sessions/reliability/redis-reply.pcapng")
				require.NoError(t, err)
				ev, stats, err := binReplay(t, cap, workers, WithProtocolDeferred(deferred))
				require.NoError(t, err)
				require.Zero(t, stats.BufferedBytes)
				byID := map[uint64]*ProtocolEvent{}
				matched, ambiguous := 0, 0
				for _, e := range ev {
					require.Empty(t, e.Error)
					byID[e.ID] = e
					if e.ResponseTo != 0 {
						matched++
						q := byID[e.ResponseTo]
						require.NotNil(t, q)
						require.NotEqual(t, false, q.Session["Reply Expected"])
						if bytes.Equal(e.Raw, []byte("+PONG\r\n")) {
							require.Equal(t, "PING", q.Session["Command"])
						}
						if bytes.Equal(e.Raw, []byte("$-1\r\n")) {
							require.Equal(t, "GET", q.Session["Command"])
						}
					}
					if e.Session["Correlation Status"] == "ambiguous-reply-mode" {
						ambiguous++
						require.Zero(t, e.ResponseTo)
					}
				}
				// The isolated ACL SETUSER setup flow has no supported role evidence.
				require.Equal(t, 9, matched)
				require.Equal(t, 1, ambiguous)
			})
		}
	}
}

func FuzzRedisReplyModeSequence(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4})
	f.Add([]byte{1, 1, 0, 3, 2, 4})
	f.Fuzz(func(t *testing.T, actions []byte) {
		if len(actions) > 128 {
			return
		}
		// Redis command semantics provide an independent reply oracle. Every reply
		// carries the ID of the command that must own it, including repeated SKIP.
		var steps []sessionStep
		var owners []int
		off, skip := false, false
		for _, a := range actions {
			args := []string{"PING"}
			mode := a % 4
			if mode == 0 {
				args = []string{"CLIENT", "REPLY", "OFF"}
			}
			if mode == 1 {
				args = []string{"CLIENT", "REPLY", "SKIP"}
			}
			if mode == 2 {
				args = []string{"CLIENT", "REPLY", "ON"}
			}
			suppressed := off || skip
			skip = false
			switch mode {
			case 0:
				off = true
				suppressed = true
			case 1:
				if !off {
					skip = true
				}
				suppressed = true
			case 2:
				off = false
				suppressed = false
			}
			q := len(steps)
			steps = append(steps, sessionStep{0, reliabilityRESP(args...)})
			if !suppressed {
				reply := "+PONG\r\n"
				if mode == 2 {
					reply = "+OK\r\n"
				}
				steps = append(steps, sessionStep{1, []byte(reply)})
				owners = append(owners, q)
			}
		}
		if len(steps) == 0 {
			return
		}
		ev, _ := sessionTestFlow(t, "redis", steps, 1, false)
		require.Len(t, ev, len(steps))
		j := 0
		for i, s := range steps {
			require.Empty(t, ev[i].Error)
			if s.dir == 1 {
				require.Equal(t, ev[owners[j]].ID, ev[i].ResponseTo)
				j++
			}
		}
	})
}

func TestReliabilityDNSNativeReplay(t *testing.T) {
	cap, err := os.ReadFile("testdata/protocol-sessions/reliability/dns-lan.pcap")
	require.NoError(t, err)
	for _, workers := range []int{1, 4} {
		ev, stats, err := binReplay(t, cap, workers, WithProtocolDecodeAs("udp", 19553, "dns"))
		require.NoError(t, err)
		require.Zero(t, stats.BufferedBytes)
		byID := map[uint64]*ProtocolEvent{}
		matches := map[string]int{}
		for _, e := range ev {
			require.Empty(t, e.Error)
			byID[e.ID] = e
			if e.ResponseTo != 0 {
				matches[e.Protocol]++
				q := byID[e.ResponseTo]
				require.NotNil(t, q)
				require.Equal(t, dnsQuestionKey(q.Session["DNS"].(map[string]any)), dnsQuestionKey(e.Session["DNS"].(map[string]any)))
			}
		}
		require.Equal(t, map[string]int{"dns": 4, "llmnr": 1}, matches)
	}
}

func TestTLSHelloRetryFragmented(t *testing.T) {
	record := func(w []byte) []byte { return append([]byte{22, 3, 3, byte(len(w) >> 8), byte(len(w))}, w...) }
	ch1 := reliabilityHS(1, reliabilityHello(true, false, 29, nil))
	hrr := reliabilityHS(2, reliabilityHello(false, true, 23, []byte("cookie")))
	ch2 := reliabilityHS(1, reliabilityHello(true, false, 23, []byte("cookie")))
	sh := reliabilityHS(2, reliabilityHello(false, false, 23, nil))
	steps := []sessionStep{{0, record(ch1)}, {1, record(hrr[:9])}, {1, record(hrr[9:])}, {0, record(ch2[:15])}, {0, record(ch2[15:])}, {1, record(sh)}}
	for _, chunk := range []int{0, 1, 7} {
		for _, deferred := range []bool{false, true} {
			ev, _ := sessionTestFlow(t, "tls", steps, chunk, deferred)
			require.Len(t, ev, 6)
			for _, e := range ev {
				require.Empty(t, e.Error)
			}
		}
	}
	// Final ServerHello and another cleartext handshake in the same record must
	// not bypass the record-level epoch boundary.
	steps = steps[:5]
	steps = append(steps, sessionStep{1, record(append(sh, reliabilityHS(20, []byte{1})...))})
	ev, _ := sessionTestFlow(t, "tls", steps, 1, false)
	require.NotEmpty(t, ev[len(ev)-1].Error)
}
func TestRedisReplyModeTransactionBoundary(t *testing.T) {
	ev, _ := sessionTestFlow(t, "redis", []sessionStep{{0, reliabilityRESP("MULTI")}, {0, reliabilityRESP("CLIENT", "REPLY", "OFF")}}, 1, false)
	require.Len(t, ev, 2)
	require.Equal(t, string(ErrContextRequired), ev[1].ExpertCode)
}

func TestLLMNRResponderBudget(t *testing.T) {
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	a := c.binParser
	a.budget.MaxCollectionElements = 2
	q := reliabilityDNS("host")
	e := &ProtocolEvent{Protocol: "llmnr", Transport: "udp", Source: "192.0.2.1:1234", Destination: "224.0.0.252:5355", Timestamp: time.Unix(1, 0)}
	require.NoError(t, a.dnsEvent(e, q))
	q[2] = 0x80
	for i := 2; i < 5; i++ {
		r := &ProtocolEvent{Protocol: "llmnr", Transport: "udp", Source: fmt.Sprintf("192.0.2.%d:5355", i), Destination: e.Source, Timestamp: time.Unix(2, 0)}
		require.NoError(t, a.dnsEvent(r, q))
		if i == 4 {
			require.Zero(t, r.ResponseTo)
			require.Equal(t, "limited", r.Session["Association Status"])
		} else {
			require.Equal(t, e.ID, r.ResponseTo)
		}
	}
	a.closeUDPSessions()
	require.Zero(t, a.buffered.Load())
}
