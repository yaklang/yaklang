package pcaputil

import (
	"context"
	"fmt"
	"github.com/google/gopacket/layers"
)

type trafficFlow struct {
	// ClientConn
	ClientConn *trafficConnection
	ServerConn *trafficConnection
	Hash       string
	Index      uint64

	ctx    context.Context
	cancel context.CancelFunc

	pool *trafficPool
}

func (t *trafficFlow) String() string {
	return fmt.Sprintf("stream[%3d]: %v <-> %v", t.Index, t.ClientConn.localAddr, t.ServerConn.localAddr)
}

func (t *trafficFlow) Feed(packet *layers.TCP) {
	if t != nil {
		if t.pool != nil {
			t.pool.flowCache.Set(t.Hash, t)
		}
	}
	if !packet.ACK && !packet.FIN && !packet.SYN && len(packet.Payload) <= 0 && !packet.RST {
		return
	}

	if t.ClientConn.localPort == int(packet.SrcPort) {
		t.ClientConn.Feed(packet)
	} else {
		t.ServerConn.Feed(packet)
	}
}
