package netstackvm

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestProbeUDPInjectsStackDatagramAndAcceptsReply(t *testing.T) {
	var mu sync.Mutex
	var frames [][]byte
	h := mockHalfOpen(t, func(_ *HalfOpenSYN, b []byte) error {
		mu.Lock()
		frames = append(frames, append([]byte(nil), b...))
		mu.Unlock()
		return nil
	})
	payload := []byte{1, 2, 3, 4}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	var got UDPSegment
	var err error
	go func() {
		defer close(done)
		got, err = h.ProbeUDP(ctx, "192.0.2.20:9", payload)
	}()

	deadline := time.Now().Add(time.Second)
	var sent []byte
	for time.Now().Before(deadline) {
		mu.Lock()
		if len(frames) > 0 {
			sent = append([]byte(nil), frames[0]...)
		}
		mu.Unlock()
		if sent != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.NotEmpty(t, sent)
	parsed := gopacket.NewPacket(sent, layers.LayerTypeIPv4, gopacket.Default)
	ip, udpLayer, ok := ipv4UDP(parsed)
	require.True(t, ok)
	require.Equal(t, payload, []byte(udpLayer.Payload))
	require.Equal(t, uint16(9), uint16(udpLayer.DstPort))
	h.observeInbound(h.main.MainNICID(), udpReply(t, ip, udpLayer, []byte("ok")))

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ProbeUDP did not return after the reply")
	}
	require.NoError(t, err)
	require.Equal(t, uint16(9), got.RemotePort)
	require.Equal(t, "192.0.2.20", got.RemoteIP.String())
	require.Equal(t, []byte("ok"), got.Payload)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, frames, 1)
}

func TestProbeUDPSilenceIsInconclusive(t *testing.T) {
	h := mockHalfOpen(t, func(_ *HalfOpenSYN, _ []byte) error { return nil })
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	_, err := h.ProbeUDP(ctx, "192.0.2.20:53", nil)
	require.ErrorIs(t, err, ErrUDPNoResponse)
}

func TestProbeUDPPortUnreachableIsNotOpen(t *testing.T) {
	var once sync.Once
	got := make(chan []byte, 1)
	h := mockHalfOpen(t, func(session *HalfOpenSYN, b []byte) error {
		once.Do(func() {
			copied := append([]byte(nil), b...)
			got <- copied
			parsed := gopacket.NewPacket(copied, layers.LayerTypeIPv4, gopacket.Default)
			ip, udpLayer, ok := ipv4UDP(parsed)
			if ok {
				session.observeInbound(session.main.MainNICID(), icmpPortUnreachable(t, ip, udpLayer))
			}
		})
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := h.ProbeUDP(ctx, "192.0.2.20:53", []byte{9})
	require.ErrorIs(t, err, ErrUDPUnreachable)
}

func udpReply(t *testing.T, ip *layers.IPv4, udpLayer *layers.UDP, payload []byte) gopacket.Packet {
	t.Helper()
	replyIP := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: ip.DstIP, DstIP: ip.SrcIP}
	replyUDP := &layers.UDP{SrcPort: udpLayer.DstPort, DstPort: udpLayer.SrcPort}
	require.NoError(t, replyUDP.SetNetworkLayerForChecksum(replyIP))
	return mustPacket(t, layers.LinkTypeRaw, replyIP, replyUDP, gopacket.Payload(payload))
}

func icmpPortUnreachable(t *testing.T, ip *layers.IPv4, udpLayer *layers.UDP) gopacket.Packet {
	t.Helper()
	quoted := &layers.IPv4{
		Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP,
		SrcIP: ip.SrcIP, DstIP: ip.DstIP,
	}
	quotedUDP := &layers.UDP{SrcPort: udpLayer.SrcPort, DstPort: udpLayer.DstPort}
	require.NoError(t, quotedUDP.SetNetworkLayerForChecksum(quoted))
	inner := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(inner, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, quoted, quotedUDP, gopacket.Payload(udpLayer.Payload)))
	replyIP := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolICMPv4, SrcIP: ip.DstIP, DstIP: ip.SrcIP}
	icmp := &layers.ICMPv4{
		TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeDestinationUnreachable, layers.ICMPv4CodePort),
	}
	return mustPacket(t, layers.LinkTypeRaw, replyIP, icmp, gopacket.Payload(inner.Bytes()))
}
