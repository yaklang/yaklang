package pcaputil

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func carrierTestIPv6(transport byte, body []byte, hop bool, padding int) []byte {
	n := 40 + len(body)
	if hop {
		n += 8
	}
	raw := make([]byte, n+padding)
	raw[0], raw[6], raw[7] = 0x60, transport, 64
	binary.BigEndian.PutUint16(raw[4:6], uint16(n-40))
	copy(raw[8:24], net.ParseIP("2001:db8::1").To16())
	copy(raw[24:40], net.ParseIP("2001:db8::2").To16())
	at := 40
	if hop {
		raw[6], raw[40] = 0, transport
		at += 8
	}
	copy(raw[at:], body)
	return raw
}

func carrierTestUDP(body []byte, declared int) []byte {
	u := make([]byte, 8+len(body))
	binary.BigEndian.PutUint16(u, 40000)
	binary.BigEndian.PutUint16(u[2:4], 53)
	binary.BigEndian.PutUint16(u[4:6], uint16(declared))
	copy(u[8:], body)
	return u
}

func carrierTestIPv4(body []byte) []byte {
	raw := make([]byte, 20+len(body))
	raw[0], raw[8], raw[9] = 0x45, 64, 17
	binary.BigEndian.PutUint16(raw[2:4], uint16(len(raw)))
	copy(raw[12:16], net.IPv4(192, 0, 2, 1).To4())
	copy(raw[16:20], net.IPv4(192, 0, 2, 2).To4())
	copy(raw[20:], body)
	return raw
}

func carrierTestReplay(t *testing.T, raw []byte, wireLength int, observer bool) []*ProtocolEvent {
	t.Helper()
	var capture bytes.Buffer
	w := pcapgo.NewWriter(&capture)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeRaw))
	require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: len(raw), Length: wireLength}, raw))
	var events []*ProtocolEvent
	opts := []CaptureOption{WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true}), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })}
	if observer {
		opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
	}
	require.NoError(t, ReplayPcap(bytes.NewReader(capture.Bytes()), opts...))
	return events
}

func TestIPv6HopByHopReuseAndInvalidEnvelopes(t *testing.T) {
	body := carrierTestUDP([]byte("opaque"), 14)
	var ip layers.IPv6
	var feedback protocolDecodeFeedback
	withHop := carrierTestIPv6(17, body, true, 0)
	require.NoError(t, decodeProtocolIPv6(&ip, withHop, &feedback))
	require.False(t, feedback.truncated)
	require.NotNil(t, ip.HopByHop)
	plain := carrierTestIPv6(17, body, false, 16)
	plain[23], plain[39] = 7, 9
	require.NoError(t, decodeProtocolIPv6(&ip, plain, &feedback))
	require.Nil(t, ip.HopByHop, "reusable decoder cannot retain the previous extension header")
	require.Equal(t, "2001:db8::7", ip.SrcIP.String())
	require.Equal(t, "2001:db8::9", ip.DstIP.String())
	require.Equal(t, body, ip.Payload)
	for _, cut := range []int{1, len(body) - 1} {
		feedback = protocolDecodeFeedback{}
		require.NoError(t, decodeProtocolIPv6(&ip, withHop[:len(withHop)-cut], &feedback))
		require.True(t, feedback.truncated)
	}
	for _, mutate := range []func([]byte){
		func(w []byte) { w[5] = 4 },              // extension cannot borrow bytes outside the IP envelope
		func(w []byte) { w[41] = 8 },             // incomplete extension
		func(w []byte) { w[42], w[43] = 1, 255 }, // impossible TLV
		func(w []byte) { w[0] = 0x40 },
		func(w []byte) { w[4], w[5] = 0, 0 }, // no selected native jumbo profile
	} {
		bad := bytes.Clone(withHop)
		mutate(bad)
		feedback = protocolDecodeFeedback{}
		require.Error(t, decodeProtocolIPv6(&ip, bad, &feedback))
	}
}

func TestIPv6HopByHopUDPFragmentRecoveryAndCarrierRefusal(t *testing.T) {
	query := dnsQuery(72, "fragments.example", 1)[2:]
	for _, malformed := range []bool{false, true} {
		u := carrierTestUDP(query, 8+len(query))
		if malformed {
			binary.BigEndian.PutUint16(u[4:6], 2000)
		}
		fragment := func(off int, more bool, payload []byte) []byte {
			f := make([]byte, 8+len(payload))
			f[0] = 17
			bits := off
			if more {
				bits |= 1
			}
			binary.BigEndian.PutUint16(f[2:4], uint16(bits))
			binary.BigEndian.PutUint32(f[4:8], 123)
			copy(f[8:], payload)
			return carrierTestIPv6(44, f, true, 0)
		}
		first, last := fragment(0, true, u[:16]), fragment(16, false, u[16:])
		conflict := bytes.Clone(first)
		conflict[len(conflict)-1] ^= 1
		for _, tc := range []struct {
			name     string
			frames   [][]byte
			complete bool
		}{
			{"ordered", [][]byte{first, last}, true},
			{"reversed", [][]byte{last, first}, true},
			{"duplicate", [][]byte{first, first, last}, true},
			{"missing-tail", [][]byte{first}, false},
			{"conflicting-overlap", [][]byte{first, conflict, last}, false},
		} {
			for _, observer := range []bool{false, true} {
				t.Run(tc.name+map[bool]string{true: "/bad-length", false: "/valid"}[malformed]+map[bool]string{true: "/observer", false: "/fast"}[observer], func(t *testing.T) {
					var capture bytes.Buffer
					w := pcapgo.NewWriter(&capture)
					require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeRaw))
					for i, raw := range tc.frames {
						require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(int64(i+1), 0), CaptureLength: len(raw), Length: len(raw)}, raw))
					}
					var events []*ProtocolEvent
					opts := []CaptureOption{WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true}), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })}
					if observer {
						opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
					}
					require.NoError(t, ReplayPcap(bytes.NewReader(capture.Bytes()), opts...))
					require.Len(t, events, 1)
					e := events[0]
					if !tc.complete {
						require.Equal(t, "ip", e.Protocol)
						require.Contains(t, []string{"incomplete", "malformed"}, e.Status)
						return
					}
					require.Equal(t, "reassembled", e.SourceBytes.Kind)
					require.Len(t, e.SourceBytes.PacketRefs, 2)
					if malformed {
						require.Empty(t, e.Protocol)
						require.Equal(t, "malformed", e.Status)
						require.Contains(t, e.Error, "2000")
					} else {
						require.Equal(t, "dns", e.Protocol, "status=%s error=%s raw=%x", e.Status, e.Error, e.Raw)
						require.Equal(t, "decoded", e.Status)
					}
				})
			}
		}
	}
}

func TestUDPCarrierRefusalCannotBeReplacedByPublicCaches(t *testing.T) {
	raw := carrierTestIPv4(carrierTestUDP(dnsQuery(7, "carrier.example", 1)[2:], 2000))
	events := carrierTestReplay(t, raw, len(raw), false)
	require.Len(t, events, 1)
	e := events[0]
	want := e.Error
	e.Protocol, e.Status, e.Completeness = "dns", "decoded", "message"
	e.Fields = map[string]any{"Caller": true}
	e.Structured = map[string]any{"fields": e.Fields}
	e.Session = e.Fields
	f, err := e.GetFields()
	require.Nil(t, f)
	require.EqualError(t, err, want)
	require.Equal(t, "limited", e.DisplayFieldValues().Completeness)
}

func FuzzNativeIPv6CarrierEnvelope(f *testing.F) {
	for _, hop := range []bool{false, true} {
		f.Add(carrierTestIPv6(17, carrierTestUDP([]byte("opaque"), 14), hop, 16))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		var ip layers.IPv6
		var feedback protocolDecodeFeedback
		if err := decodeProtocolIPv6(&ip, raw, &feedback); err != nil {
			return
		}
		require.Equal(t, uint8(6), ip.Version)
		require.NotZero(t, ip.Length)
		header := 40
		if ip.HopByHop != nil {
			header += ip.HopByHop.ActualLength
		}
		end := min(40+int(ip.Length), len(raw))
		require.LessOrEqual(t, header, end)
		require.Equal(t, raw[header:end], ip.Payload)
		require.Equal(t, 40+int(ip.Length) > len(raw), feedback.truncated)
	})
}

func TestIPv6HopByHopNativeCarrierBoundaries(t *testing.T) {
	for _, transport := range []byte{6, 17} {
		body := carrierTestUDP([]byte("opaque"), 14)
		if transport == 6 {
			body = make([]byte, 23)
			body[12] = 0x50
			copy(body[20:], "tcp")
		}
		for _, padding := range []int{0, 16} {
			raw := carrierTestIPv6(transport, body, true, padding)
			p := gopacket.NewPacket(raw, protocolPacketDecoder{decoder: layers.LayerTypeIPv6}, gopacket.Default)
			require.Nil(t, p.ErrorLayer())
			require.False(t, p.Metadata().Truncated, "HBH bytes are included in IPv6 Payload Length")
			require.Equal(t, body, p.NetworkLayer().LayerPayload(), "link padding is not transport payload")
			require.Equal(t, body, p.Layer(layers.LayerTypeIPv6HopByHop).LayerPayload())
			require.NotNil(t, p.TransportLayer())
		}
	}
}

func TestUDPCarrierMalformedVersusCaptureTruncated(t *testing.T) {
	query := dnsQuery(71, "carrier.example", 1)[2:]
	for _, v6 := range []bool{false, true} {
		makeIP := func(udp []byte) []byte {
			if v6 {
				return carrierTestIPv6(17, udp, true, 0)
			}
			return carrierTestIPv4(udp)
		}
		for _, tc := range []struct {
			name          string
			declared, cut int
			status        string
		}{
			{"complete", 8 + len(query), 0, "decoded"},
			{"larger-than-IP", 2000, 0, "malformed"},
			{"short-UDP-length", 7, 0, "malformed"},
			{"capture-truncated", 8 + len(query), 3, "incomplete"},
		} {
			for _, observer := range []bool{false, true} {
				t.Run(tc.name+map[bool]string{true: "/IPv6", false: "/IPv4"}[v6]+map[bool]string{true: "/observer", false: "/fast"}[observer], func(t *testing.T) {
					raw := makeIP(carrierTestUDP(query, tc.declared))
					events := carrierTestReplay(t, raw[:len(raw)-tc.cut], len(raw), observer)
					require.Len(t, events, 1)
					e := events[0]
					require.Equal(t, tc.status, e.Status)
					if tc.status == "decoded" {
						require.Equal(t, "dns", e.Protocol)
					} else {
						require.Empty(t, e.Protocol, "bad carrier cannot confirm an application")
						require.NotEmpty(t, e.Error)
					}
				})
			}
		}
	}
}
