package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// L4 controls only: RFC 8659 section 4.1 wire layout, not claimed real traffic.
func caaDNSControl(rdata []byte) []byte {
	w := []byte{0x12, 0x34, 0x81, 0x80, 0, 1, 0, 1, 0, 0, 0, 0, 0, 1, 1, 0, 1}
	w = append(w, 0xc0, 12, 1, 1, 0, 1, 0, 0, 0, 60)
	w = binary.BigEndian.AppendUint16(w, uint16(len(rdata)))
	return append(w, rdata...)
}

func TestDNSCAASharedCodec(t *testing.T) {
	require.Equal(t, "CAA", dnsTypeName(257))
	for _, tc := range []struct {
		flags byte
		tag   string
		value []byte
	}{
		{0, "issue", []byte("example.net")}, {128, "issuewild", []byte(";")},
		{129, "X9a", []byte{0, 255}}, {0, "x", nil},
		{0, string(bytes.Repeat([]byte{'x'}, 255)), []byte("maximum tag length")},
	} {
		t.Run(fmt.Sprintf("%d/%d", tc.flags, len(tc.tag)), func(t *testing.T) {
			data := append([]byte{tc.flags, byte(len(tc.tag))}, []byte(tc.tag)...)
			data = append(data, tc.value...)
			w := caaDNSControl(data)
			info, err := DecodeDNSMessage(w, 32)
			require.NoError(t, err)
			row := info["Answers"].([]map[string]any)[0]
			require.Equal(t, tc.flags, row["CAA Flags"])
			require.Equal(t, tc.flags&128 != 0, row["Issuer Critical"])
			require.Equal(t, tc.tag, row["Tag"])
			require.Equal(t, data, row["RData"])
			require.Equal(t, tc.value, append([]byte(nil), row["Value"].([]byte)...))
			require.NotContains(t, row, "RData Completeness")
			for n := range w {
				_, err := DecodeDNSMessage(w[:n], 32)
				require.Error(t, err)
			}
			for i := range w {
				w[i] = 0xff
			}
			require.Equal(t, data, row["RData"], "raw data aliases the input")
			require.Equal(t, tc.value, append([]byte(nil), row["Value"].([]byte)...), "value aliases input")
		})
	}
	for _, data := range [][]byte{nil, {0}, {0, 1}, {0, 0, 'x'}, {0, 2, 'x'}, {0, 1, '-'}, {0, 1, 255}} {
		_, err := DecodeDNSMessage(caaDNSControl(data), 32)
		require.Error(t, err)
	}
	// Do not borrow the next RR's bytes to repair a short CAA tag.
	w := caaDNSControl([]byte{0, 2, 'x'})
	binary.BigEndian.PutUint16(w[6:], 2)
	w = append(w, caaDNSControl([]byte{0, 1, 'x'})[17:]...)
	_, err := DecodeDNSMessage(w, 32)
	require.Error(t, err)
}

func TestDNSCAAExistingUDPAndTCPPaths(t *testing.T) {
	w := caaDNSControl(append([]byte{128, 5}, []byte("issueexample.net")...))
	assertEvent := func(t *testing.T, e *ProtocolEvent) {
		t.Helper()
		require.Equal(t, "dns", e.Protocol)
		require.Empty(t, e.Error)
		info := e.Session["DNS"].(map[string]any)
		row := info["Answers"].([]map[string]any)[0]
		require.Equal(t, "issue", row["Tag"])
		require.Equal(t, true, row["Issuer Critical"])
		require.Equal(t, []byte("example.net"), row["Value"])
		_, err := e.Decode()
		require.NoError(t, err)
	}
	for _, deferred := range []bool{false, true} {
		for _, workers := range []int{1, 2, 4} {
			t.Run(fmt.Sprintf("udp/workers%d/deferred%v", workers, deferred), func(t *testing.T) {
				pcap := t20UDPCapture(t, net.IPv4(192, 0, 2, 53), net.IPv4(192, 0, 2, 1), 53, 40000, w)
				events, stats, err := binReplay(t, pcap, workers, WithProtocolDeferred(deferred))
				require.NoError(t, err)
				require.Len(t, events, 1)
				assertEvent(t, events[0])
				require.Zero(t, stats.Malformed)
				require.Zero(t, stats.BufferedBytes)
			})
		}
		for _, chunk := range []int{1, 7, 4096} {
			t.Run(fmt.Sprintf("tcp/chunk%d/deferred%v", chunk, deferred), func(t *testing.T) {
				s, err := NewProtocolSession(DefaultParserBudget())
				require.NoError(t, err)
				defer s.Close("test")
				s.(*captureSession).f.ports = [2]uint16{40000, 53}
				s.(*captureSession).f.a.config.Deferred = deferred
				framed := append(binary.BigEndian.AppendUint16(nil, uint16(len(w))), w...)
				// Coalesced messages and arbitrary chunking exercise the existing framer.
				wire := append(bytes.Clone(framed), framed...)
				var events []*ProtocolEvent
				for off := 0; off < len(wire); off += chunk {
					events = append(events, s.Feed(1, time.Unix(1, 0), wire[off:min(off+chunk, len(wire))]).Events...)
				}
				require.Len(t, events, 2)
				for _, e := range events {
					assertEvent(t, e)
				}
			})
		}
	}
}

func TestDNSEmptyTXTRejectedAcrossEntryPaths(t *testing.T) {
	w := caaDNSControl(nil)
	binary.BigEndian.PutUint16(w[13:], 16) // question type
	binary.BigEndian.PutUint16(w[19:], 16) // answer type
	_, err := DecodeDNSMessage(w, 32)
	require.ErrorContains(t, err, "DNS TXT")
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprintf("udp/deferred%v", deferred), func(t *testing.T) {
			pcap := t20UDPCapture(t, net.IPv4(192, 0, 2, 53), net.IPv4(192, 0, 2, 1), 53, 40000, w)
			events, stats, err := binReplay(t, pcap, 1, WithProtocolDeferred(deferred))
			require.NoError(t, err)
			require.Len(t, events, 1)
			require.Equal(t, "malformed", events[0].Status)
			require.Contains(t, events[0].Error, "DNS TXT")
			require.EqualValues(t, 1, stats.Malformed)
		})
		t.Run(fmt.Sprintf("tcp/deferred%v", deferred), func(t *testing.T) {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			defer s.Close("test")
			s.(*captureSession).f.ports = [2]uint16{40000, 53}
			s.(*captureSession).f.a.config.Deferred = deferred
			wire := append(binary.BigEndian.AppendUint16(nil, uint16(len(w))), w...)
			var events []*ProtocolEvent
			for _, b := range wire {
				events = append(events, s.Feed(1, time.Unix(1, 0), []byte{b}).Events...)
			}
			require.Len(t, events, 1)
			require.Equal(t, "malformed", events[0].Status)
			require.Contains(t, events[0].Error, "DNS TXT")
		})
	}
}

func TestDNSTXTCollectionBoundarySharedCodec(t *testing.T) {
	for _, count := range []int{4096, 4097} {
		w := caaDNSControl(make([]byte, count))
		binary.BigEndian.PutUint16(w[13:], 16)
		binary.BigEndian.PutUint16(w[19:], 16)
		for _, limit := range []int{0, 8192} {
			info, err := DecodeDNSMessage(w, limit)
			if count > 4096 {
				require.Error(t, err)
				continue
			}
			require.NoError(t, err)
			require.Len(t, info["Answers"].([]map[string]any)[0]["Text"], count)
		}
	}
	// Two records fit a budget of two, but three TXT strings must not.
	w := caaDNSControl(make([]byte, 3))
	binary.BigEndian.PutUint16(w[13:], 16)
	binary.BigEndian.PutUint16(w[19:], 16)
	_, err := DecodeDNSMessage(w, 2)
	require.ErrorContains(t, err, "DNS TXT count")
}

func TestDNSTXTLimitOutcomeAcrossTransports(t *testing.T) {
	for _, count := range []int{4096, 4097} {
		w := caaDNSControl(make([]byte, count))
		binary.BigEndian.PutUint16(w[13:], 16)
		binary.BigEndian.PutUint16(w[19:], 16)
		for _, deferred := range []bool{false, true} {
			check := func(t *testing.T, events []*ProtocolEvent, stats ProtocolStats) {
				t.Helper()
				require.Len(t, events, 1)
				e := events[0]
				if count == 4096 {
					require.Contains(t, []string{"decoded", "deferred"}, e.Status)
					require.Empty(t, e.Error)
					require.Len(t, e.Session["DNS"].(map[string]any)["Answers"].([]map[string]any)[0]["Text"], count)
				} else {
					require.Equal(t, "limited", e.Status)
					require.Equal(t, string(ErrResourceExceeded), e.ExpertCode)
					require.Contains(t, e.Error, "DNS TXT count")
					require.Zero(t, stats.Malformed)
					require.Greater(t, stats.LimitedBytes, uint64(0))
				}
			}
			for _, workers := range []int{1, 2, 4} {
				t.Run(fmt.Sprintf("udp/count%d/deferred%v/workers%d", count, deferred, workers), func(t *testing.T) {
					pcap := t20UDPCapture(t, net.IPv4(192, 0, 2, 53), net.IPv4(192, 0, 2, 1), 53, 40000, w)
					events, stats, err := binReplay(t, pcap, workers, WithProtocolDeferred(deferred))
					require.NoError(t, err)
					check(t, events, stats)
					require.Zero(t, stats.BufferedBytes)
				})
			}
			for _, chunk := range []int{1, 7, 65536} {
				t.Run(fmt.Sprintf("tcp/count%d/deferred%v/chunk%d", count, deferred, chunk), func(t *testing.T) {
					s, err := NewProtocolSession(DefaultParserBudget())
					require.NoError(t, err)
					flow := s.(*captureSession).f
					flow.ports = [2]uint16{40000, 53}
					flow.a.config.Deferred = deferred
					framed := append(binary.BigEndian.AppendUint16(nil, uint16(len(w))), w...)
					var events []*ProtocolEvent
					for off := 0; off < len(framed); off += chunk {
						events = append(events, s.Feed(1, time.Unix(1, 0), framed[off:min(off+chunk, len(framed))]).Events...)
					}
					check(t, events, s.Stats())
					s.Close("EOF")
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}
