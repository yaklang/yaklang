package sharkcli

import (
	"strconv"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Only the inspector's source-indexed arrivals can provide identity witnesses
// after detail eviction. Placeholder/eventIDs/knownIDs are not field evidence.
func (p *capturedPacket) protocolQuerySnapshots(events []*pcaputil.ProtocolEvent) []pcaputil.DisplayFieldSnapshot {
	var snapshots []pcaputil.DisplayFieldSnapshot
	fullArrivals := make(map[uint64]struct{}, len(events))
	for _, e := range events {
		if e == nil {
			continue
		}
		if e.Completeness == "evicted" {
			snapshots = append(snapshots, pcaputil.DisplayFieldSnapshot{Completeness: "evicted"})
			continue
		}
		snapshots = append(snapshots, e.DisplayFieldValues())
		if arrival := e.HistoryArrival(); arrival != 0 {
			fullArrivals[arrival] = struct{}{}
		}
	}
	if p.history == nil || p.reference.Number == 0 {
		return snapshots
	}
	w := p.history.PacketContributorWindow(p.reference, p.physicalRecord, 4096)
	missing := !w.Complete
	for _, id := range w.Identities {
		if _, ok := fullArrivals[id.Arrival]; !ok {
			missing = true
			break
		}
	}
	if !missing {
		return snapshots
	}
	witness := pcaputil.DisplayFieldSnapshot{
		SchemaVersion: pcaputil.DisplayFieldSchemaVersion,
		Completeness:  "limited", Incomplete: true,
	}
	if w.Complete {
		witness.Completeness = "detail-unavailable"
	}
	seen := make(map[uint64]struct{}, len(w.Identities))
	for _, id := range w.Identities {
		if _, ok := seen[id.ID]; ok {
			continue
		}
		seen[id.ID] = struct{}{}
		witness.Values = append(witness.Values, pcaputil.DisplayFieldValue{
			Name: "pdu.id", Value: id.ID, ExactValue: strconv.FormatUint(id.ID, 10), Origin: "event",
		})
	}
	// Identity-only witnesses are partial, including an empty witness after
	// index loss. Retain no new per-packet field trees or identity arrays.
	return append([]pcaputil.DisplayFieldSnapshot{witness}, snapshots...)
}
