// Compare observed flow association with bytes actually admitted to reassembly.
// This prints hashes and frame references, never payloads or secrets. Captures
// and output must be kept outside the repository by the caller.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

type admitted struct {
	Sequence uint32                     `json:"sequence"`
	Length   int                        `json:"length"`
	SHA256   string                     `json:"sha256"`
	Refs     []pcaputil.PacketReference `json:"packet_refs"`
}

type review struct {
	Observed []pcaputil.PacketReference `json:"observed"`
	Admitted []admitted                 `json:"admitted"`
}

func main() {
	if len(os.Args) != 4 {
		panic("usage: flow-admission CAPTURE INTERFACE FLOW_ID,FLOW_ID,...")
	}
	iface, err := strconv.ParseUint(os.Args[2], 10, 32)
	if err != nil {
		panic(err)
	}
	selected := map[uint64]*review{}
	for _, text := range strings.Split(os.Args[3], ",") {
		id, err := strconv.ParseUint(text, 10, 64)
		if err != nil || id == 0 {
			panic("invalid flow ID")
		}
		selected[id] = &review{}
	}
	get := func(flow *pcaputil.TrafficFlow) *review {
		if flow.CaptureDomain().Interface != int(iface) {
			return nil
		}
		return selected[flow.Index]
	}
	err = pcaputil.ReplayPcapFile(os.Args[1],
		pcaputil.WithIgnoreEmbeddedSecrets(true),
		pcaputil.WithTCPReassemblyOptions(pcaputil.TCPReassemblyOptions{AllowIncomplete: true}),
		pcaputil.WithOnProtocolMessage(func(*pcaputil.ProtocolEvent) {}),
		pcaputil.WithOnTrafficFlowPacket(func(flow *pcaputil.TrafficFlow, _ *pcaputil.TrafficConnection, refs []pcaputil.PacketReference, _ time.Time) {
			if r := get(flow); r != nil {
				r.Observed = append(r.Observed, refs...)
			}
		}),
		pcaputil.WithOnTrafficFlowOnDataFrameArrived(func(flow *pcaputil.TrafficFlow, _ *pcaputil.TrafficConnection, frame *pcaputil.TrafficFrame) {
			if r := get(flow); r != nil {
				r.Admitted = append(r.Admitted, admitted{Sequence: frame.Seq, Length: len(frame.Payload), SHA256: fmt.Sprintf("%x", sha256.Sum256(frame.Payload)), Refs: frame.PacketReferences()})
			}
		}))
	if err != nil {
		panic(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(selected); err != nil {
		panic(err)
	}
}
