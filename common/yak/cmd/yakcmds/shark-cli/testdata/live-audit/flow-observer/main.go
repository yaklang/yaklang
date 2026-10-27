// Command flow-observer exports packet ownership and literal flow metadata from
// ReplayPcap. It is an audit harness, not a second TCP implementation.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

type created struct {
	Flow                uint64                 `json:"flow"`
	Source              string                 `json:"source"`
	Destination         string                 `json:"destination"`
	Domain              pcaputil.CaptureDomain `json:"domain"`
	HalfOpen            bool                   `json:"half_open"`
	Ethernet            bool                   `json:"ethernet"`
	HardwareSource      string                 `json:"hardware_source"`
	HardwareDestination string                 `json:"hardware_destination"`
}
type packet struct {
	Flow      uint64                     `json:"flow"`
	Direction int                        `json:"direction"`
	Refs      []pcaputil.PacketReference `json:"refs"`
	Captured  string                     `json:"captured"`
}
type closed struct {
	Flow        uint64 `json:"flow"`
	Reason      string `json:"reason"`
	PendingGaps bool   `json:"pending_gaps"`
}

func main() {
	workers := flag.Int("workers", 1, "capture workers")
	flag.Parse()
	if flag.NArg() != 1 {
		panic("usage: flow-observer [-workers N] CAPTURE")
	}
	wire, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		panic(err)
	}
	if len(wire) > 32<<20 {
		panic("this scoped observer accepts captures up to 32 MiB")
	}
	var mu sync.Mutex
	var flows []created
	var packets []packet
	var closes []closed
	var stats pcaputil.TCPReassemblyStats
	err = pcaputil.ReplayPcap(bytes.NewReader(wire), pcaputil.WithTCPReassemblyOptions(pcaputil.TCPReassemblyOptions{AllowIncomplete: true, Stream: true}), pcaputil.WithTCPReassemblyWorkers(*workers),
		pcaputil.WithOnTrafficFlowCreated(func(f *pcaputil.TrafficFlow) {
			mu.Lock()
			defer mu.Unlock()
			flows = append(flows, created{f.Index, f.ClientConn.LocalAddr().String(), f.ServerConn.LocalAddr().String(), f.CaptureDomain(), f.IsHalfOpen, f.IsEthernetLinkLayer, f.HardwareSrcMac, f.HardwareDstMac})
		}), pcaputil.WithOnTrafficFlowPacket(func(f *pcaputil.TrafficFlow, c *pcaputil.TrafficConnection, refs []pcaputil.PacketReference, ts time.Time) {
			mu.Lock()
			defer mu.Unlock()
			direction := 0
			if c != f.ClientConn {
				direction = 1
			}
			packets = append(packets, packet{f.Index, direction, append([]pcaputil.PacketReference(nil), refs...), ts.UTC().Format(time.RFC3339Nano)})
		}), pcaputil.WithOnTrafficFlowClosed(func(reason pcaputil.TrafficFlowCloseReason, f *pcaputil.TrafficFlow) {
			mu.Lock()
			defer mu.Unlock()
			closes = append(closes, closed{f.Index, string(reason), f.HasPendingGaps()})
		}), pcaputil.WithOnTCPReassemblyStats(func(s pcaputil.TCPReassemblyStats) { stats = s }))
	digest := sha256.Sum256(wire)
	failure := ""
	if err != nil {
		failure = fmt.Sprint(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"capture_SHA256": hex.EncodeToString(digest[:]), "flows": flows, "packets": packets, "closed": closes, "stats": stats, "error": failure}); err != nil {
		panic(err)
	}
}
