package netstackvm_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/netstackvm"
)

func newChannelPair(t *testing.T) (client, server *netstackvm.NetStackVirtualMachineEntry) {
	t.Helper()
	var err error
	client, err = netstackvm.NewChannelNetStackVirtualMachineEntry("10.0.0.1")
	require.NoError(t, err)
	server, err = netstackvm.NewChannelNetStackVirtualMachineEntry("10.0.0.2")
	require.NoError(t, err)
	t.Cleanup(func() {
		client.GetStack().Destroy()
		server.GetStack().Destroy()
	})
	return client, server
}

func sameFlow(t *testing.T, a, b netstackvm.TCPSegment) {
	t.Helper()
	require.Truef(t, a.LocalIP.Equal(b.LocalIP), "local ip %s vs %s", a, b)
	require.Equalf(t, a.LocalPort, b.LocalPort, "local port %s vs %s", a, b)
	require.Truef(t, a.RemoteIP.Equal(b.RemoteIP), "remote ip %s vs %s", a, b)
	require.Equalf(t, a.RemotePort, b.RemotePort, "remote port %s vs %s", a, b)
}

func TestTCPProbeHandshakePayloadAndClose(t *testing.T) {
	client, server := newChannelPair(t)
	const port = 18443
	target := "10.0.0.2:18443"
	ln, err := server.ListenTCP(target)
	require.NoError(t, err)
	defer ln.Close()

	type acceptResult struct {
		c   net.Conn
		err error
	}
	accepted := make(chan acceptResult, 1)
	go func() {
		c, acceptErr := ln.Accept()
		accepted <- acceptResult{c: c, err: acceptErr}
	}()

	probe, err := client.StartTCPProbe(context.Background(), target, server)
	require.NoError(t, err)
	defer probe.Close()

	payload := []byte("probe-payload")
	_, err = probe.Write(payload)
	require.Error(t, err, "payload must not be writable before ProbeACK")
	require.False(t, probe.Established())
	require.False(t, probe.Closed())

	syn, err := probe.ProbeSYN()
	require.NoError(t, err)
	t.Logf("SYN %s", syn)
	synAck, err := probe.ReceiveSYNACK()
	require.NoError(t, err)
	t.Logf("SYN-ACK %s", synAck)

	require.True(t, syn.SYN)
	require.False(t, syn.ACK)
	require.False(t, syn.FIN)
	require.False(t, syn.RST)
	require.Equal(t, netstackvm.TCPSegmentSent, syn.Direction)
	require.Zero(t, syn.PayloadLen)
	require.NotZero(t, syn.LocalPort)
	require.True(t, syn.LocalIP.Equal(client.GetMainNICIPv4Address()))
	require.True(t, syn.RemoteIP.Equal(server.GetMainNICIPv4Address()))
	require.Equal(t, uint16(port), syn.RemotePort)
	require.Equal(t, syn.SYN, syn.Flags&0x02 != 0)
	require.Equal(t, syn.ACK, syn.Flags&0x10 != 0)

	require.True(t, synAck.SYN)
	require.True(t, synAck.ACK)
	require.False(t, synAck.FIN)
	require.False(t, synAck.RST)
	require.Equal(t, netstackvm.TCPSegmentReceived, synAck.Direction)
	require.Zero(t, synAck.PayloadLen)
	require.Equal(t, syn.Seq+1, synAck.Ack)
	sameFlow(t, syn, synAck)
	require.False(t, probe.Established())

	select {
	case res := <-accepted:
		t.Fatalf("listener accepted before ProbeACK: conn=%v err=%v", res.c, res.err)
	case <-time.After(200 * time.Millisecond):
	}
	_, err = probe.Conn()
	require.Error(t, err)

	hsAck, err := probe.ProbeACK()
	require.NoError(t, err)
	t.Logf("ACK %s", hsAck)
	require.True(t, hsAck.ACK)
	require.False(t, hsAck.SYN)
	require.False(t, hsAck.FIN)
	require.False(t, hsAck.RST)
	require.Equal(t, netstackvm.TCPSegmentSent, hsAck.Direction)
	require.Zero(t, hsAck.PayloadLen)
	require.Equal(t, syn.Seq+1, hsAck.Seq)
	require.Equal(t, synAck.Seq+1, hsAck.Ack)
	sameFlow(t, syn, hsAck)
	require.True(t, probe.Established())
	require.False(t, probe.Closed())

	var serverConn net.Conn
	select {
	case res := <-accepted:
		require.NoError(t, res.err)
		require.NotNil(t, res.c)
		serverConn = res.c
	case <-time.After(3 * time.Second):
		t.Fatal("listener did not accept after ProbeACK")
	}
	defer serverConn.Close()

	conn, err := probe.Conn()
	require.NoError(t, err)
	_, err = conn.Write(payload)
	require.NoError(t, err)

	serverConn.SetDeadline(time.Now().Add(3 * time.Second))
	got := make([]byte, len(payload))
	_, err = io.ReadFull(serverConn, got)
	require.NoError(t, err)
	require.Equal(t, payload, got)

	_, err = serverConn.Write(got)
	require.NoError(t, err)
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	echo := make([]byte, len(payload))
	_, err = io.ReadFull(conn, echo)
	require.NoError(t, err)
	require.Equal(t, payload, echo)

	fin, err := probe.SendFIN()
	require.NoError(t, err)
	t.Logf("FIN %s", fin)
	require.True(t, fin.FIN)
	require.True(t, fin.ACK)
	require.False(t, fin.SYN)
	require.False(t, fin.RST)
	require.Equal(t, netstackvm.TCPSegmentSent, fin.Direction)
	require.Zero(t, fin.PayloadLen)
	require.Equal(t, hsAck.Seq+uint32(len(payload)), fin.Seq)
	require.Equal(t, synAck.Seq+1+uint32(len(payload)), fin.Ack)
	sameFlow(t, syn, fin)
	require.False(t, probe.Closed())

	serverConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	scratch := make([]byte, 8)
	var readErr error
	for i := 0; i < 4; i++ {
		_, readErr = serverConn.Read(scratch)
		if readErr != nil {
			break
		}
	}
	require.Error(t, readErr)
	require.NoError(t, serverConn.Close())

	peerAck, peerFin, err := probe.ReceivePeerClose()
	require.NoError(t, err)
	t.Logf("peer ACK %s", peerAck)
	t.Logf("peer FIN %s", peerFin)
	require.True(t, peerAck.ACK)
	require.Equal(t, fin.Seq+1, peerAck.Ack)
	require.Equal(t, netstackvm.TCPSegmentReceived, peerAck.Direction)
	require.True(t, peerFin.FIN)
	require.False(t, peerFin.RST)
	require.Equal(t, netstackvm.TCPSegmentReceived, peerFin.Direction)
	require.Equal(t, synAck.Seq+1+uint32(len(payload)), peerFin.Seq)
	sameFlow(t, syn, peerAck)
	sameFlow(t, syn, peerFin)
	require.False(t, probe.Closed())

	final, err := probe.SendFinalACK()
	require.NoError(t, err)
	t.Logf("final ACK %s", final)
	require.True(t, final.ACK)
	require.False(t, final.FIN)
	require.False(t, final.SYN)
	require.False(t, final.RST)
	require.Equal(t, netstackvm.TCPSegmentSent, final.Direction)
	require.Equal(t, fin.Seq+1, final.Seq)
	require.Equal(t, peerFin.Seq+1, final.Ack)
	sameFlow(t, syn, final)
	require.True(t, probe.Established())
	require.True(t, probe.Closed())

	_, err = conn.Write([]byte("after-close"))
	require.Error(t, err)
	_, err = probe.Write([]byte("after-close"))
	require.Error(t, err)
	_, err = probe.Conn()
	require.Error(t, err)
}

func TestTCPProbeSkippedStepDoesNotEstablish(t *testing.T) {
	client, server := newChannelPair(t)
	target := "10.0.0.2:18443"
	ln, err := server.ListenTCP(target)
	require.NoError(t, err)
	defer ln.Close()

	probe, err := client.StartTCPProbe(context.Background(), target, server)
	require.NoError(t, err)
	defer probe.Close()

	_, err = probe.ReceiveSYNACK()
	require.Error(t, err)
	_, err = probe.ProbeACK()
	require.Error(t, err)
	_, err = probe.SendFIN()
	require.Error(t, err)
	_, _, err = probe.ReceivePeerClose()
	require.Error(t, err)
	_, err = probe.SendFinalACK()
	require.Error(t, err)
	_, err = probe.Write([]byte("skipped"))
	require.Error(t, err)
	_, err = probe.Conn()
	require.Error(t, err)
	require.False(t, probe.Established())
	require.False(t, probe.Closed())

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	accepted := make(chan struct{}, 1)
	go func() {
		c, acceptErr := ln.Accept()
		if acceptErr == nil {
			c.Close()
			accepted <- struct{}{}
		}
	}()
	select {
	case <-accepted:
		t.Fatal("skipped steps established a connection")
	case <-ctx.Done():
	}
}

func TestTCPProbeMissingSYNACK(t *testing.T) {
	client, server := newChannelPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	probe, err := client.StartTCPProbe(ctx, "10.0.0.2:18443", server)
	require.NoError(t, err)
	defer probe.Close()

	syn, err := probe.ProbeSYN()
	require.NoError(t, err)
	require.True(t, syn.SYN)
	require.False(t, syn.ACK)
	t.Logf("SYN %s", syn)

	_, err = probe.ReceiveSYNACK()
	require.Error(t, err)
	_, err = probe.ProbeACK()
	require.Error(t, err)
	_, err = probe.Conn()
	require.Error(t, err)
	require.False(t, probe.Established())
	require.False(t, probe.Closed())
}

func TestTCPProbeCloseBeforeHandshake(t *testing.T) {
	client, server := newChannelPair(t)
	target := "10.0.0.2:18443"
	ln, err := server.ListenTCP(target)
	require.NoError(t, err)
	defer ln.Close()

	probe, err := client.StartTCPProbe(context.Background(), target, server)
	require.NoError(t, err)
	defer probe.Close()

	_, err = probe.SendFIN()
	require.Error(t, err)
	_, _, err = probe.ReceivePeerClose()
	require.Error(t, err)
	_, err = probe.SendFinalACK()
	require.Error(t, err)
	require.False(t, probe.Established())
	require.False(t, probe.Closed())

	syn, err := probe.ProbeSYN()
	require.NoError(t, err)
	require.True(t, syn.SYN)
	_, err = probe.SendFIN()
	require.Error(t, err)
	_, err = probe.Conn()
	require.Error(t, err)
	require.False(t, probe.Established())
	require.False(t, probe.Closed())

	time.Sleep(200 * time.Millisecond)
	accepted := make(chan struct{}, 1)
	go func() {
		c, acceptErr := ln.Accept()
		if acceptErr == nil {
			buf := make([]byte, 8)
			c.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			n, _ := c.Read(buf)
			if n > 0 {
				accepted <- struct{}{}
			}
			c.Close()
		}
	}()
	select {
	case <-accepted:
		t.Fatal("payload flowed before the handshake finished")
	case <-time.After(400 * time.Millisecond):
	}
}

func TestDialTCPStillWorksOnChannelStack(t *testing.T) {
	client, server := newChannelPair(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, netstackvm.BridgeChannelNetStacks(ctx, client, server))

	target := "10.0.0.2:18444"
	ln, err := server.ListenTCP(target)
	require.NoError(t, err)
	defer ln.Close()

	payload := []byte("dial-tcp")
	got := make(chan []byte, 1)
	go func() {
		c, acceptErr := ln.Accept()
		if acceptErr != nil {
			got <- nil
			return
		}
		defer c.Close()
		buf := make([]byte, len(payload))
		c.SetDeadline(time.Now().Add(3 * time.Second))
		_, readErr := io.ReadFull(c, buf)
		if readErr != nil {
			got <- nil
			return
		}
		got <- append([]byte(nil), buf...)
	}()

	conn, err := client.DialTCP(3*time.Second, target)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.Write(payload)
	require.NoError(t, err)

	select {
	case body := <-got:
		require.True(t, bytes.Equal(payload, body))
	case <-time.After(4 * time.Second):
		t.Fatal("DialTCP payload was not delivered")
	}
}

func TestHostPCAPDeviceOptional(t *testing.T) {
	if os.Getenv("NETSTACK_TRY_NIC") == "" {
		t.Skip("set NETSTACK_TRY_NIC=1 to try a host pcap device")
	}
	ifaces, err := net.Interfaces()
	require.NoError(t, err)
	var last error
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		vm, openErr := netstackvm.NewNetStackVirtualMachineEntry(netstackvm.WithPcapDevice(ifc.Name))
		if openErr != nil {
			last = openErr
			t.Logf("%s: %v", ifc.Name, openErr)
			continue
		}
		t.Logf("opened %s", ifc.Name)
		vm.Close()
		return
	}
	t.Fatalf("no pcap device opened: %v", last)
}
