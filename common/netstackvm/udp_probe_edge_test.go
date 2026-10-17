package netstackvm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"golang.org/x/time/rate"
)

func TestProbeUDPRejectsBadTargetsBeforeAdmission(t *testing.T) {
	h := mockHalfOpen(t, func(*HalfOpenSYN, []byte) error {
		t.Fatal("invalid target reached the writer")
		return nil
	})
	var nilSession *HalfOpenSYN
	_, err := nilSession.ProbeUDP(context.Background(), "192.0.2.20:9", nil)
	require.Error(t, err)

	cases := []string{
		"not-a-target",
		"192.0.2.20",
		"192.0.2.20:0",
		"192.0.2.20:65536",
		"192.0.2.20:abc",
		"224.0.0.1:9",
		"255.255.255.255:9",
		"0.0.0.0:9",
		"[::1]:9",
		"192.0.2.0:9",
		"192.0.2.255:9",
		"127.0.0.1:9",
	}
	for _, target := range cases {
		t.Run(target, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			_, err := h.ProbeUDP(ctx, target, nil)
			require.Error(t, err)
			require.NotErrorIs(t, err, ErrUDPNoResponse)
			require.NotErrorIs(t, err, ErrUDPUnreachable)
		})
	}
	_, err = h.ProbeUDP(context.Background(), "192.0.2.20:9", bytes.Repeat([]byte{1}, header.UDPMaximumSize))
	require.Error(t, err)
	h.mu.Lock()
	require.Empty(t, h.udpFlights)
	h.mu.Unlock()
}

func TestProbeUDPClosedSessionDoesNotSend(t *testing.T) {
	var writes atomic.Int32
	h := mockHalfOpen(t, func(*HalfOpenSYN, []byte) error {
		writes.Add(1)
		return nil
	})
	require.NoError(t, h.Close())
	_, err := h.ProbeUDP(context.Background(), "192.0.2.20:9", []byte{1})
	require.ErrorIs(t, err, errHalfOpenClosed)
	require.EqualValues(t, 0, writes.Load())
}

func TestProbeUDPCloseCancelsWaitAndDoesNotReportOpen(t *testing.T) {
	started := make(chan struct{})
	h := mockHalfOpen(t, func(*HalfOpenSYN, []byte) error {
		close(started)
		return nil
	})
	done := make(chan error, 1)
	go func() {
		_, err := h.ProbeUDP(context.Background(), "192.0.2.20:9", []byte{1})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("probe did not send")
	}
	require.NoError(t, h.Close())
	select {
	case err := <-done:
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrUDPNoResponse)
	case <-time.After(time.Second):
		t.Fatal("Close did not unblock ProbeUDP")
	}
}

func TestProbeUDPIgnoresReplyBeforeInjection(t *testing.T) {
	h := mockHalfOpen(t, func(*HalfOpenSYN, []byte) error { return nil })
	lim := rate.NewLimiter(rate.Every(time.Hour), 1)
	require.True(t, lim.Allow())
	h.limiter = lim
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := h.ProbeUDP(ctx, "192.0.2.20:9", []byte{7})
		done <- err
	}()
	var frame []byte
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && frame == nil {
		h.mu.Lock()
		for _, flight := range h.udpFlights {
			if flight.frame != nil && !flight.sent && !flight.sending {
				frame = append([]byte(nil), flight.frame.data...)
			}
		}
		h.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	require.NotEmpty(t, frame)
	parsed := gopacket.NewPacket(frame, layers.LayerTypeIPv4, gopacket.Default)
	ip, udpLayer, ok := ipv4UDP(parsed)
	require.True(t, ok)
	require.False(t, h.observeInbound(h.main.MainNICID(), udpReply(t, ip, udpLayer, []byte("early"))))
	h.mu.Lock()
	for _, flight := range h.udpFlights {
		require.Zero(t, len(flight.results), "a reply before injection must not be queued")
	}
	h.mu.Unlock()
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
		require.NotErrorIs(t, err, ErrUDPUnreachable)
	case <-time.After(time.Second):
		t.Fatal("canceled probe did not return")
	}
}

func TestProbeUDPRejectsUncorrelatedAndCorruptReplies(t *testing.T) {
	h := mockHalfOpen(t, func(session *HalfOpenSYN, b []byte) error {
		parsed := gopacket.NewPacket(b, layers.LayerTypeIPv4, gopacket.Default)
		ip, udpLayer, ok := ipv4UDP(parsed)
		require.True(t, ok)
		require.False(t, session.observeInbound(session.main.MainNICID(), directedUDP(t, ip, udpLayer.SrcPort, layers.UDPPort(udpLayer.DstPort+1), net.ParseIP("192.0.2.20"), []byte("x"))))
		require.False(t, session.observeInbound(session.main.MainNICID(), directedUDP(t, ip, udpLayer.SrcPort, udpLayer.DstPort, net.ParseIP("192.0.2.99"), []byte("x"))))
		require.False(t, session.observeInbound(session.main.MainNICID(), corruptUDPChecksum(t, udpReply(t, ip, udpLayer, []byte("bad")))))
		require.False(t, session.observeInbound(session.main.MainNICID(), fragmentedUDP(t, ip, udpLayer)))
		require.False(t, session.observeInbound(session.main.MainNICID(), icmpUnreachable(t, ip, udpLayer, layers.ICMPv4CodeHost)))
		require.False(t, session.observeInbound(session.main.MainNICID(), udpReply(t, ip, udpLayer, []byte("ok"))))
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, err := h.ProbeUDP(ctx, "192.0.2.20:11", []byte{1, 2})
	require.NoError(t, err)
	require.Equal(t, uint16(11), got.RemotePort)
	require.Equal(t, "192.0.2.20", got.RemoteIP.String())
	require.Equal(t, []byte("ok"), got.Payload)
	h.mu.Lock()
	require.Empty(t, h.udpFlights)
	h.mu.Unlock()
}

func TestProbeUDPBadChecksumIsNotAnOpenPort(t *testing.T) {
	h := mockHalfOpen(t, func(session *HalfOpenSYN, b []byte) error {
		parsed := gopacket.NewPacket(b, layers.LayerTypeIPv4, gopacket.Default)
		ip, udpLayer, ok := ipv4UDP(parsed)
		require.True(t, ok)
		require.False(t, session.observeInbound(session.main.MainNICID(), corruptUDPChecksum(t, udpReply(t, ip, udpLayer, []byte("bad")))))
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	_, err := h.ProbeUDP(ctx, "192.0.2.20:11", []byte{1})
	require.ErrorIs(t, err, ErrUDPNoResponse)
	require.ErrorIs(t, err, ErrProbeChecksum)
	require.NotErrorIs(t, err, ErrUDPUnreachable)
}

func TestProbeUDPZeroChecksumReplyIsAccepted(t *testing.T) {
	h := mockHalfOpen(t, func(session *HalfOpenSYN, b []byte) error {
		parsed := gopacket.NewPacket(b, layers.LayerTypeIPv4, gopacket.Default)
		ip, udpLayer, ok := ipv4UDP(parsed)
		require.True(t, ok)
		reply := udpReply(t, ip, udpLayer, []byte{9})
		raw := append([]byte(nil), reply.Data()...)
		hdr := header.IPv4(raw)
		sumAt := int(hdr.HeaderLength()) + 6
		raw[sumAt] = 0
		raw[sumAt+1] = 0
		require.False(t, session.observeInbound(session.main.MainNICID(), gopacket.NewPacket(raw, layers.LayerTypeIPv4, gopacket.Default)))
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, err := h.ProbeUDP(ctx, "192.0.2.20:11", nil)
	require.NoError(t, err)
	require.Equal(t, []byte{9}, got.Payload)
}

func TestProbeUDPWriteFailureIsNotSilence(t *testing.T) {
	failure := errors.New("pcap write failed")
	h := mockHalfOpen(t, func(*HalfOpenSYN, []byte) error { return failure })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := h.ProbeUDP(ctx, "192.0.2.20:11", []byte{1})
	require.ErrorIs(t, err, ErrUDPSend)
	require.ErrorIs(t, err, failure)
	require.NotErrorIs(t, err, ErrUDPNoResponse)
}

func TestProbeUDPGateIsSingleUseAndPayloadBound(t *testing.T) {
	h := mockHalfOpen(t, func(*HalfOpenSYN, []byte) error { return nil })
	nic := h.main.MainNICID()
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.IPv4(192, 0, 2, 10), DstIP: net.IPv4(192, 0, 2, 20)}
	udpLayer := &layers.UDP{SrcPort: 40000, DstPort: 9}
	require.NoError(t, udpLayer.SetNetworkLayerForChecksum(ip))
	packet := mustPacket(t, layers.LinkTypeRaw, ip, udpLayer, gopacket.Payload([]byte{1}))
	require.False(t, h.allowOutbound(nic, packet), "unregistered UDP must not leave the gate")

	parsedIP, parsedUDP, ok := ipv4UDP(packet)
	require.True(t, ok)
	key := udpPacketKey(nic, parsedIP, parsedUDP, false)
	h.mu.Lock()
	h.udpFlights[key] = &udpFlight{
		ctx:    context.Background(),
		permit: true,
		frame:  &udpFrame{segment: UDPSegment{Payload: []byte{1, 2}}},
	}
	h.mu.Unlock()
	require.False(t, h.allowOutbound(nic, packet), "payload mismatch must not consume the permit")
	h.mu.Lock()
	require.True(t, h.udpFlights[key].permit)
	h.udpFlights[key].frame.segment.Payload = []byte{1}
	h.mu.Unlock()
	require.True(t, h.allowOutbound(nic, packet))
	require.False(t, h.allowOutbound(nic, packet), "a second copy must not be injected")
}

func TestProbeUDPAdmissionStaysInsideSlotLimit(t *testing.T) {
	var writes atomic.Int32
	started := make(chan struct{})
	h := mockHalfOpen(t, func(*HalfOpenSYN, []byte) error {
		writes.Add(1)
		select {
		case <-started:
		default:
			close(started)
		}
		return nil
	})
	h.slots = make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = h.ProbeUDP(ctx, "192.0.2.20:9", []byte{1})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first probe did not reach the writer")
	}
	limited, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stop()
	_, err := h.ProbeUDP(limited, "192.0.2.20:11", []byte{1})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.EqualValues(t, 1, writes.Load())
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("held probe did not release the slot")
	}
}

func TestProbeUDPConcurrentRepliesDoNotCross(t *testing.T) {
	const n = 24
	h := mockHalfOpen(t, func(session *HalfOpenSYN, b []byte) error {
		parsed := gopacket.NewPacket(b, layers.LayerTypeIPv4, gopacket.Default)
		ip, udpLayer, ok := ipv4UDP(parsed)
		if !ok {
			return errors.New("stack frame was not UDP")
		}
		decoy, err := directedUDPErr(ip, udpLayer.SrcPort, udpLayer.DstPort, net.ParseIP("192.0.2.99"), []byte{0xff})
		if err != nil {
			return err
		}
		if session.observeInbound(session.main.MainNICID(), decoy) {
			return errors.New("decoy reply entered the stack")
		}
		reply, err := directedUDPErr(ip, udpLayer.SrcPort, udpLayer.DstPort, ip.DstIP, []byte{byte(udpLayer.DstPort)})
		if err != nil {
			return err
		}
		if session.observeInbound(session.main.MainNICID(), reply) {
			return errors.New("correlated reply entered the stack")
		}
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			got, err := h.ProbeUDP(ctx, fmt.Sprintf("192.0.2.20:%d", port), []byte{byte(port)})
			if err != nil {
				errCh <- fmt.Errorf("port %d: %w", port, err)
				return
			}
			if int(got.RemotePort) != port || got.RemoteIP.String() != "192.0.2.20" || len(got.Payload) != 1 || got.Payload[0] != byte(port) {
				errCh <- fmt.Errorf("port %d crossed to %+v", port, got)
			}
		}(12000 + i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
	h.mu.Lock()
	require.Empty(t, h.udpFlights)
	h.mu.Unlock()
}

func TestProbeUDPCloseRacesWithAdmission(t *testing.T) {
	h := mockHalfOpen(t, func(*HalfOpenSYN, []byte) error { return nil })
	h.slots = make(chan struct{}, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			_, _ = h.ProbeUDP(ctx, fmt.Sprintf("192.0.2.20:%d", port), []byte{1})
		}(13000 + i)
	}
	time.Sleep(5 * time.Millisecond)
	require.NoError(t, h.Close())
	cancel()
	wg.Wait()
}

func directedUDP(t *testing.T, request *layers.IPv4, localPort, remotePort layers.UDPPort, remoteIP net.IP, payload []byte) gopacket.Packet {
	t.Helper()
	pkt, err := directedUDPErr(request, localPort, remotePort, remoteIP, payload)
	require.NoError(t, err)
	return pkt
}

func directedUDPErr(request *layers.IPv4, localPort, remotePort layers.UDPPort, remoteIP net.IP, payload []byte) (gopacket.Packet, error) {
	replyIP := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: remoteIP, DstIP: append(net.IP(nil), request.SrcIP...)}
	replyUDP := &layers.UDP{SrcPort: remotePort, DstPort: localPort}
	if err := replyUDP.SetNetworkLayerForChecksum(replyIP); err != nil {
		return nil, err
	}
	buf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, replyIP, replyUDP, gopacket.Payload(payload)); err != nil {
		return nil, err
	}
	return gopacket.NewPacket(buf.Bytes(), layers.LayerTypeIPv4, gopacket.Default), nil
}

func fragmentedUDP(t *testing.T, request *layers.IPv4, udpLayer *layers.UDP) gopacket.Packet {
	t.Helper()
	replyIP := &layers.IPv4{
		Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP,
		SrcIP: request.DstIP, DstIP: request.SrcIP,
		Flags: layers.IPv4MoreFragments, FragOffset: 1,
	}
	replyUDP := &layers.UDP{SrcPort: udpLayer.DstPort, DstPort: udpLayer.SrcPort}
	require.NoError(t, replyUDP.SetNetworkLayerForChecksum(replyIP))
	return mustPacket(t, layers.LinkTypeRaw, replyIP, replyUDP, gopacket.Payload([]byte("frag")))
}

func corruptUDPChecksum(t *testing.T, pkt gopacket.Packet) gopacket.Packet {
	t.Helper()
	raw := append([]byte(nil), pkt.Data()...)
	hdr := header.IPv4(raw)
	require.True(t, hdr.IsChecksumValid())
	off := int(hdr.HeaderLength()) + 6
	require.Less(t, off+1, len(raw))
	raw[off] ^= 0xff
	return gopacket.NewPacket(raw, layers.LayerTypeIPv4, gopacket.Default)
}

func icmpUnreachable(t *testing.T, ip *layers.IPv4, udpLayer *layers.UDP, code uint8) gopacket.Packet {
	t.Helper()
	quoted := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: ip.SrcIP, DstIP: ip.DstIP}
	quotedUDP := &layers.UDP{SrcPort: udpLayer.SrcPort, DstPort: udpLayer.DstPort}
	require.NoError(t, quotedUDP.SetNetworkLayerForChecksum(quoted))
	inner := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(inner, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, quoted, quotedUDP, gopacket.Payload(udpLayer.Payload)))
	replyIP := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolICMPv4, SrcIP: ip.DstIP, DstIP: ip.SrcIP}
	icmp := &layers.ICMPv4{TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeDestinationUnreachable, code)}
	return mustPacket(t, layers.LinkTypeRaw, replyIP, icmp, gopacket.Payload(inner.Bytes()))
}
