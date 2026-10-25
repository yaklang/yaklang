// Replay a complete capture with an optional processing delay. Output contains
// flow associations and accepted chunk hashes, never payload bytes or secrets.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func main() {
	capture := flag.String("capture", "", "original capture outside Git")
	delay := flag.Duration("delay", 0, "processing delay after every N normalized packets")
	every := flag.Uint64("every", 256, "delay interval; positive")
	flag.Parse()
	if *capture == "" || *every == 0 || *delay < 0 {
		panic("expected -capture FILE [-delay 50ms] [-every 256]")
	}
	var mu sync.Mutex
	encoder := json.NewEncoder(os.Stdout)
	emit := func(row map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		if err := encoder.Encode(row); err != nil {
			panic(err)
		}
	}
	var packets uint64
	err := pcaputil.ReplayPcapFile(*capture,
		pcaputil.WithIgnoreEmbeddedSecrets(true),
		pcaputil.WithTCPReassemblyOptions(pcaputil.TCPReassemblyOptions{AllowIncomplete: true}),
		pcaputil.WithOnProtocolMessage(func(*pcaputil.ProtocolEvent) {}),
		pcaputil.WithBeforeTransportPacket(func(gopacket.Packet) {
			packets++
			if packets%*every == 0 && *delay > 0 {
				time.Sleep(*delay)
			}
		}),
		pcaputil.WithOnTrafficFlowPacket(func(flow *pcaputil.TrafficFlow, _ *pcaputil.TrafficConnection, refs []pcaputil.PacketReference, _ time.Time) {
			emit(map[string]any{"kind": "association", "flow": flow.Index, "packet_refs": refs})
		}),
		pcaputil.WithOnTrafficFlowOnDataFrameArrived(func(flow *pcaputil.TrafficFlow, conn *pcaputil.TrafficConnection, frame *pcaputil.TrafficFrame) {
			emit(map[string]any{"kind": "admitted", "flow": flow.Index, "client_direction": flow.ClientConn == conn, "sequence": frame.Seq, "length": len(frame.Payload), "sha256": fmt.Sprintf("%x", sha256.Sum256(frame.Payload)), "packet_refs": frame.PacketReferences()})
		}))
	if err != nil {
		panic(err)
	}
}
