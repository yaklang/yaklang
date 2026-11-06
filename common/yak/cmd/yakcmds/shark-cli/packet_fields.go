package sharkcli

import (
	"crypto/sha256"
	"fmt"
	"net"
	"strconv"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func (p *capturedPacket) packetFieldSnapshot(row packetSummary) pcaputil.DisplayFieldSnapshot {
	e := &pcaputil.ProtocolEvent{Source: row.Source, Destination: row.Destination, Transport: row.Transport, TCPFlowID: row.TCPFlowID}
	e.SourceBytes = pcaputil.ByteSource{Kind: "captured", SHA256: fmt.Sprintf("%x", sha256.Sum256(p.data)), PacketRefs: []pcaputil.PacketReference{p.reference}}
	if row.Transport == "tcp" {
		e.Source = net.JoinHostPort(row.Source, strconv.Itoa(int(row.SourcePort)))
		e.Destination = net.JoinHostPort(row.Destination, strconv.Itoa(int(row.DestinationPort)))
	}
	if p.physicalRecord || p.number != 0 {
		e.Frame = &pcaputil.DisplayFrameObservation{Number: p.number, Time: p.ci.Timestamp, CapturedLength: uint32(len(p.data))}
	}
	s := e.DisplayFieldValues()
	// A packet observation has no synthetic PDU identity/status. Retain only
	// registered physical event fields, before the potentially partial PDU union.
	values := s.Values[:0]
	for _, value := range s.Values {
		switch value.Name {
		case "ip.src", "ip.dst", "ip.addr", "tcp.srcport", "tcp.dstport", "tcp.port", "tcp.stream", "tcp.flow.id", "source", "destination", "frame.number", "frame.time_epoch", "frame.len":
			values = append(values, value)
		}
	}
	s.Values = values
	return s
}
