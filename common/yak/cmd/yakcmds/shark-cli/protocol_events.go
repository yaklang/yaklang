package sharkcli

import (
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"sort"
	"strings"
)

// The worker only projects owned snapshots. All protocol state advances in the
// capture pipeline before the lossy display queue, including unselected flows.
func protocolEventFields(events []*pcaputil.ProtocolEvent) []field {
	var out []field
	var walk func(string, any, int)
	walk = func(name string, v any, depth int) {
		if len(out) >= 4096 || depth > 32 {
			return
		}
		switch x := v.(type) {
		case map[string]any:
			out = append(out, field{depth: depth, branch: true, text: safeText(name)})
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(k, x[k], depth+1)
			}
		case []map[string]any:
			for i, r := range x {
				walk(fmt.Sprintf("%s[%d]", name, i), r, depth)
			}
		case []any:
			for i, r := range x {
				walk(fmt.Sprintf("%s[%d]", name, i), r, depth)
			}
		default:
			text := fmt.Sprintf("%s: %v", name, x)
			if len(text) > 2048 {
				text = text[:2048] + "…"
			}
			out = append(out, field{depth: depth, text: safeText(text)})
		}
	}
	for _, e := range events {
		walk(fmt.Sprintf("PDU #%d · %s · %s · %s", e.ID, e.Protocol, e.Status, e.Completeness), map[string]any{"Source": e.Source, "Destination": e.Destination, "Byte Source": e.SourceBytes.Kind, "Parent PDU": e.SourceBytes.ParentPDU, "Response To": e.ResponseTo, "Packet References": e.SourceBytes.PacketRefs, "Session": e.Session, "Fields": e.Fields, "Error": e.Error}, 0)
	}
	if len(events) == 0 {
		out = append(out, field{text: "No retained protocol events; detail unavailable for this retention window."})
	}
	return out
}
func retainedStreamEvents(history *pcaputil.ProtocolInspector, stream *streamSnapshot) []*pcaputil.ProtocolEvent {
	if history == nil {
		return nil
	}
	var out []*pcaputil.ProtocolEvent
	remaining := 256 << 10
	for _, e := range history.Rows("", 0) {
		if e.Domain != stream.domain || e.Transport != "tcp" || !(e.Source == stream.endpoints[0] && e.Destination == stream.endpoints[1] || e.Source == stream.endpoints[1] && e.Destination == stream.endpoints[0]) {
			continue
		}
		if !stream.first.IsZero() && e.Timestamp.Before(stream.first) || !stream.last.IsZero() && e.Timestamp.After(stream.last) {
			continue
		}
		if len(out) >= 128 {
			break
		}
		if d := retainedProtocolDetail(history, e.ID); d != nil {
			out = append(out, boundedEvent(d, &remaining))
		}
	}
	return out
}
func eventProtocols(events []*pcaputil.ProtocolEvent) string {
	var names []string
	for _, e := range events {
		if e.Status != "decoded" && e.Status != "deferred" {
			continue
		}
		name := strings.ToUpper(e.Protocol)
		found := false
		for _, n := range names {
			found = found || n == name
		}
		if !found {
			names = append(names, name)
		}
	}
	return strings.Join(names, " / ")
}

// Packet cache retains IDs only; payload snapshots share the bounded history.
func (p *capturedPacket) protocolEvents() []*pcaputil.ProtocolEvent {
	if p.history == nil {
		return p.events
	}
	var events []*pcaputil.ProtocolEvent
	remaining := 256 << 10
	ids := append([]uint64(nil), p.eventIDs...)
	for _, id := range p.history.PacketEventIDs(p.reference, 128) {
		found := false
		for _, old := range ids {
			if old == id {
				found = true
				break
			}
		}
		if !found && len(ids) < 128 {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		if e := retainedProtocolDetail(p.history, id); e != nil {
			events = append(events, boundedEvent(e, &remaining))
		}
	}
	return events
}

func boundedEvent(e *pcaputil.ProtocolEvent, remaining *int) *pcaputil.ProtocolEvent {
	cost := e.RetainedBytes()
	if cost <= *remaining {
		*remaining -= cost
		return e
	}
	return &pcaputil.ProtocolEvent{ID: e.ID, FlowID: e.FlowID, Protocol: e.Protocol, Status: "limited", Completeness: "limited", Summary: "Detail unavailable: snapshot byte budget", Domain: e.Domain, Source: e.Source, Destination: e.Destination, ResponseTo: e.ResponseTo}
}

// Details returns an owned observation alongside its decode error. Only a nil
// observation denotes absence/eviction; a retained diagnostic remains visible.
func retainedProtocolDetail(history *pcaputil.ProtocolInspector, id uint64) *pcaputil.ProtocolEvent {
	e, err := history.Details(id)
	if e != nil && err != nil {
		if e.Error == "" {
			e.Error = err.Error()
			e.Completeness = "detail-unavailable"
		}
		if e.ExpertCode == "" {
			var typed *pcaputil.ProtocolError
			if errors.As(err, &typed) {
				e.ExpertCode = string(typed.Kind)
			}
		}
	}
	return e
}
