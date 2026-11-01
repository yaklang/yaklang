package sharkcli

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// These are known same-PDU copies. Captured-frame trees and separate
// decrypted/encapsulated sources are never candidates for this comparison.
func duplicateDNSSessionFields(e *pcaputil.ProtocolEvent) bool {
	if e == nil || len(e.Fields) == 0 || (e.Protocol != "dns" && e.Protocol != "mdns" && e.Protocol != "llmnr") {
		return false
	}
	dns, ok := e.Session["DNS"].(map[string]any)
	return ok && reflect.DeepEqual(e.Fields, dns)
}

func duplicateSSHInitialFields(e *pcaputil.ProtocolEvent) bool {
	if e == nil || e.Protocol != "ssh" || len(e.Fields) == 0 {
		return false
	}
	initial, ok := e.Session["Initial Packet Fields"].(map[string]any)
	return ok && reflect.DeepEqual(e.Fields, initial)
}

// IPC collapses Go integer widths into JSON numbers. Make the exact typed
// equality decision in the parent, while both original maps are still owned
// Go values. Only the outgoing display copy loses its redundant subtree.
func protocolDisplayEvent(e *pcaputil.ProtocolEvent) *pcaputil.ProtocolEvent {
	redundant := ""
	if duplicateDNSSessionFields(e) {
		redundant = "DNS"
	} else if duplicateSSHInitialFields(e) {
		redundant = "Initial Packet Fields"
	}
	if redundant == "" {
		return e
	}
	projected := *e
	projected.Session = make(map[string]any, len(e.Session)-1)
	for key, value := range e.Session {
		if key != redundant {
			projected.Session[key] = value
		}
	}
	return &projected
}

func hasTLSConnectionObservation(protocol string) bool {
	return protocol == "tls" || protocol == "quic" || protocol == "http3" || protocol == "doq"
}

var connectionObservationPriority = map[string]int{
	"Evidence Scope": 0, "Session ID Echo": 1, "Compatibility Session ID Echo": 2,
	"Session ID Resumption Indicated": 3, "PSK Selected": 4, "PSK Selection": 5,
	"Binder Verified": 0, "Ticket Association": 1, "Offer Match": 2, "Key Exchange Mode": 3,
	"Certificate Trust": 10,
}

// Retained connection observations are generated metadata. Do not route their
// keys through the wire/PDU walker: a metadata key named Payload, GRPC Messages
// or HTTP3 Frames cannot authorize another byte source or a captured range.
func connectionObservationFields(values map[string]any, budget int) ([]field, bool) {
	var fields []field
	limited := false
	add := func(text string, depth int, branch bool) bool {
		if len(fields) >= max(0, budget) {
			limited = true
			return false
		}
		fields = append(fields, field{text: clipText(text, 2048), depth: depth, branch: branch})
		return true
	}
	var walk func(string, any, int)
	var mapChildren func(map[string]any, int)
	mapChildren = func(value map[string]any, depth int) {
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			a, aKnown := connectionObservationPriority[keys[i]]
			b, bKnown := connectionObservationPriority[keys[j]]
			if !aKnown {
				a = 100
			}
			if !bKnown {
				b = 100
			}
			if a != b {
				return a < b
			}
			return keys[i] < keys[j]
		})
		for _, key := range keys {
			if len(fields) >= max(0, budget) {
				limited = true
				break
			}
			walk(key, value[key], depth)
		}
	}
	walk = func(name string, value any, depth int) {
		if depth > 32 || len(fields) >= max(0, budget) {
			limited = true
			return
		}
		switch value := value.(type) {
		case map[string]any:
			if add(name, depth, true) {
				mapChildren(value, depth+1)
			}
		case []map[string]any:
			for i, item := range value {
				if len(fields) >= max(0, budget) {
					limited = true
					break
				}
				walk(fmt.Sprintf("%s[%d]", name, i), item, depth+1)
			}
		case []any:
			for i, item := range value {
				if len(fields) >= max(0, budget) {
					limited = true
					break
				}
				walk(fmt.Sprintf("%s[%d]", name, i), item, depth+1)
			}
		default:
			text := formatFieldValue(name, value)
			if _, binary := fieldValueOctets(name, value); binary && strings.HasSuffix(text, " (select for full bytes)") {
				text = strings.TrimSuffix(text, " (select for full bytes)") + " (metadata bytes; frame offsets unavailable)"
			}
			add(name+": "+text, depth, false)
		}
	}
	if add("Session Establishment · observed connection context", 0, true) {
		add("Scope: retained hello metadata; no new handshake or authentication result implied", 1, false)
		mapChildren(values, 1)
	}
	return fields, limited
}

func protocolEventStatusText(e *pcaputil.ProtocolEvent, state string) string {
	text := "Status: " + state
	if e == nil {
		return text
	}
	if e.Source != "" && e.Destination != "" {
		return text + " · source " + e.Source + " → " + e.Destination
	}
	if e.Source != "" {
		return text + " · source " + e.Source
	}
	if e.Destination != "" {
		return text + " · destination " + e.Destination
	}
	return text
}
