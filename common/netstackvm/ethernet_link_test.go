package netstackvm

import (
	"context"
	"io"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/icmp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
)

type frameDevice struct {
	rx, tx chan []byte
	done   chan struct{}
	once   *sync.Once
	mu     sync.Mutex
	sent   [][]byte
}

func (d *frameDevice) Read(b []byte) (int, error) {
	select {
	case p := <-d.rx:
		if len(p) > len(b) {
			return 0, io.ErrShortBuffer
		}
		return copy(b, p), nil
	case <-d.done:
		return 0, io.EOF
	}
}
func (d *frameDevice) Write(b []byte) (int, error) {
	raw := append([]byte(nil), b...)
	d.mu.Lock()
	d.sent = append(d.sent, raw)
	d.mu.Unlock()
	select {
	case d.tx <- raw:
		return len(b), nil
	case <-d.done:
		return 0, io.ErrClosedPipe
	}
}
func (d *frameDevice) Close() error { d.once.Do(func() { close(d.done) }); return nil }
func TestEthernetBridgeARPAndTCP(t *testing.T) {
	ab, ba := make(chan []byte, 128), make(chan []byte, 128)
	done := make(chan struct{})
	once := new(sync.Once)
	da, db := &frameDevice{rx: ba, tx: ab, done: done, once: once}, &frameDevice{rx: ab, tx: ba, done: done, once: once}
	macA, macB := net.HardwareAddr{2, 0, 0, 0, 1, 1}, net.HardwareAddr{2, 0, 0, 0, 1, 2}
	ea, e := NewEthernetLink(context.Background(), da, macA, 1500)
	require.NoError(t, e)
	eb, e := NewEthernetLink(context.Background(), db, macB, 1500)
	require.NoError(t, e)
	a := testNetworkVM(t, NetworkVMConfig{Mode: NetworkBridged, Address: netip.MustParsePrefix("10.77.0.1/24"), Link: ea})
	b := testNetworkVM(t, NetworkVMConfig{Mode: NetworkBridged, Address: netip.MustParsePrefix("10.77.0.2/24"), Link: eb})
	ln, e := b.ListenTCP("10.77.0.2:8080")
	require.NoError(t, e)
	defer ln.Close()
	go func() {
		c, e := ln.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		io.Copy(c, c)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, e := a.DialContext(ctx, "tcp", "10.77.0.2:8080")
	require.NoError(t, e)
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(time.Second))
	_, e = conn.Write([]byte("bridge"))
	require.NoError(t, e)
	buf := make([]byte, 6)
	_, e = io.ReadFull(conn, buf)
	require.NoError(t, e)
	require.Equal(t, "bridge", string(buf))
	reply, err := icmp.NewClient(a.Stack()).Ping(ctx, "10.77.0.2", time.Second)
	require.NoError(t, err)
	require.NotNil(t, reply)
	require.EqualValues(t, header.ICMPv4EchoReply, reply.MessageType)
	da.mu.Lock()
	frames := append([][]byte(nil), da.sent...)
	da.mu.Unlock()
	arp, tcp := false, false
	for _, frame := range frames {
		eth := header.Ethernet(frame)
		require.Equal(t, string(macA), string(eth.SourceAddress()))
		if eth.Type() == header.ARPProtocolNumber {
			arp = true
			require.Equal(t, []byte(macA), []byte(header.ARP(frame[14:]).HardwareAddressSender()))
		}
		if eth.Type() == header.IPv4ProtocolNumber {
			tcp = true
			require.Equal(t, string(macB), string(eth.DestinationAddress()))
		}
	}
	require.True(t, arp, "bridge must resolve a peer through ARP")
	require.True(t, tcp)
	require.NoError(t, a.Close())
	require.NoError(t, b.Close())
}
