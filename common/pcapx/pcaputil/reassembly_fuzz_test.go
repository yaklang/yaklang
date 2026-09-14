package pcaputil

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

func FuzzTCPReassemblyOverlap(f *testing.F) {
	f.Add([]byte{255, 255, 255, 240, 0, 100, 80, 90, 8, 50, 1, 255})
	f.Add([]byte{0, 0, 0, 0, 99, 30, 20, 40, 10, 70})
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) < 4 || len(input) > 512 {
			return
		}
		base := binary.BigEndian.Uint32(input)
		source := make([]byte, 256)
		for i := range source {
			source[i] = byte(i) ^ input[i%len(input)]
		}
		steps := []tcpStep{{seq: base - 1, syn: true}}
		for i := 4; i+1 < len(input); i += 2 {
			start := int(input[i])
			end := start + 1 + int(input[i+1])%(256-start)
			steps = append(steps, tcpStep{seq: base + uint32(start), data: string(source[start:end])})
		}
		steps = append(steps, tcpStep{seq: base, data: string(source)}, tcpStep{seq: base + 256, fin: true})
		got, err := adversarialReplay(t, steps, 1, true, input[0]&1 != 0)
		if err != nil || got != string(source) {
			t.Fatalf("reference stream mismatch: err=%v, bytes=%d", err, len(got))
		}
	})
}

func FuzzTCPMalformedPacket(f *testing.F) {
	f.Add(adversarialWire(1, 0x11, nil, []byte("hello")), uint8(0))
	f.Add(adversarialWire(1, 0x10, []byte{1, 1, 1, 30}, []byte("malformed")), uint8(0))
	f.Add([]byte{0, 0, 0, 0}, uint8(1))
	f.Fuzz(func(t *testing.T, raw []byte, encapsulation uint8) {
		if len(raw) > 4096 {
			return
		}
		link := []layers.LinkType{layers.LinkTypeEthernet, layers.LinkTypeRaw, layers.LinkTypeNull}[encapsulation%3]
		p := newTrafficPool(context.Background(), TCPReassemblyOptions{Stream: true, MaxFrameBytes: 64, MaxPendingBytes: 4096, MaxFlows: 4})
		defer p.Close()
		var received int
		p.onFlowFrameDataFrameArrived = append(p.onFlowFrameDataFrameArrived, func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { received += len(f.Payload) })
		conf := NewDefaultConfig()
		conf.trafficPool = p
		d := offlineDecoder{conf: conf, link: link}
		d.feed(context.Background(), raw, gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: len(raw), Length: len(raw)})
		_, _, _ = rawFlowKey(raw, link)
		p.Close()
		if received > len(raw) || p.pendingBytes != 0 || p.pendingSegments != 0 {
			t.Fatalf("invalid byte/budget accounting: delivered=%d raw=%d pending=%d/%d", received, len(raw), p.pendingBytes, p.pendingSegments)
		}
	})
}
