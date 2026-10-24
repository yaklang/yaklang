package sharkcli

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

var semanticFieldPriority = map[string]int{
	"Message Kind": 2, "Notification Subtype": 3, "Discovery Target": 4, "USN": 5, "Location": 6, "Maximum Age Seconds": 7,
	"Protocol": 0, "ID": 0, "Status": 1, "Packet Name": 2, "Message Name": 2, "Mechanism": 2,
	"Authentication Verified": 3, "User Name": 4, "Domain": 5, "Method": 4, "Request Method": 4, "Request URI": 5, "Query": 5, "SQL": 5, "Topic": 5,
	"Message Type": 6, "QNAME": 4, "QTYPE Name": 5, "Response": 3, "RCODE": 4, "Questions": 5, "Answers": 6, "Headers": 5,
	"MIC Verified": 7, "Identity Evidence": 8, "Decoded Bytes": 900, "Mechanism Token": 900, "Decoded Fields": 1000,
}

func fieldPriority(name string) int {
	if rank, ok := semanticFieldPriority[name]; ok {
		return rank
	}
	return 100
}

// The worker only projects owned snapshots. All protocol state advances in the
// capture pipeline before the lossy display queue, including unselected flows.
func protocolEventFields(events []*pcaputil.ProtocolEvent) []field {
	fields, _ := protocolEventDetail(events)
	return fields
}

func protocolEventDetail(events []*pcaputil.ProtocolEvent) ([]field, []detailSource) {
	var out []field
	var sources []detailSource
	ordered := append([]*pcaputil.ProtocolEvent(nil), events...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i] == nil {
			return false
		}
		if ordered[j] == nil {
			return true
		}
		a, b := ordered[i], ordered[j]
		if eventPriority(a) == eventPriority(b) && a.Protocol == "tls" && b.Protocol == "tls" {
			ar, br := a.Session["Record Recovery Verified"] == true, b.Session["Record Recovery Verified"] == true
			if ar != br {
				return ar
			}
		}
		return eventPriority(a) > eventPriority(b)
	})
	add := func(text string, branch bool) { out = append(out, field{text: clipText(text, 2048), branch: branch}) }
	verified, rejected := 0, 0
	for _, e := range ordered {
		if e == nil || e.Protocol != "tls" {
			continue
		}
		if e.Session["Authentication Verified"] == true {
			verified++
		}
		if e.ExpertCode == string(pcaputil.ErrAuthenticationFailed) {
			rejected++
		}
	}
	if verified > 0 && rejected > 0 {
		add(fmt.Sprintf("TLS records: %d authenticated, %d rejected · verification is per record", verified, rejected), false)
	}
	var protocol string
	var currentSource int
	var wireRanges map[string][]int
	var authenticatedQUIC bool
	var authenticatedTLS bool
	var walk func(string, any, int)
	walk = func(name string, v any, depth int) {
		if len(out) >= 4096 || depth > 32 {
			return
		}
		switch x := v.(type) {
		case map[string]any:
			add(name, true)
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Slice(keys, func(i, j int) bool {
				a, b := fieldPriority(keys[i]), fieldPriority(keys[j])
				if a != b {
					return a < b
				}
				return keys[i] < keys[j]
			})
			for _, k := range keys {
				walk(k, x[k], depth+1)
			}
		case []map[string]any:
			for i, r := range x {
				if name == "Headers" {
					add(textValue(r["Name"])+": "+textValue(r["Value"]), false)
					if protocol == "ssdp" && currentSource > 0 {
						var span []int
						switch p := r["Value Byte Range"].(type) {
						case []int:
							span = p
						case []any:
							for _, x := range p {
								span = append(span, int(number(x)))
							}
						}
						if len(span) == 2 && span[0] >= 0 && span[1] > span[0] && span[1] <= len(sources[currentSource-1].Data) {
							f := &out[len(out)-1]
							f.source, f.start, f.end, f.hasRange = currentSource, span[0], span[1], true
						}
					}
					continue
				}
				if name == "GRPC Messages" {
					data, kind := grpcMessageData(r)
					add(fmt.Sprintf("gRPC message %d · %d bytes", i+1, len(data)), true)
					if len(data) > 0 {
						sources = append(sources, detailSource{Name: "gRPC message · " + kind, Data: bytes.Clone(data)})
						out[len(out)-1].source, out[len(out)-1].hasRange, out[len(out)-1].end = len(sources), true, len(data)
					}
					for _, key := range []string{"Index", "Compressed", "Length", "Decoded Length", "Protobuf Wire Fields", "Protobuf Wire Status", "Payload", "Decoded Payload"} {
						if value, ok := r[key]; ok {
							if key == "Payload" {
								value = bytesValue(value)
							}
							if key == "Decoded Payload" {
								value = data
							}
							walk(key, value, depth+1)
						}
					}
					continue
				}
				if name == "Handshake Messages" {
					add(handshakeType(r["Type"]), true)
					keys := make([]string, 0, len(r))
					for key := range r {
						if key != "Type" {
							keys = append(keys, key)
						}
					}
					sort.Strings(keys)
					for _, key := range keys {
						walk(key, r[key], depth+1)
					}
					continue
				}
				walk(fmt.Sprintf("%s[%d]", name, i), r, depth)
			}
		case []any:
			if rows := mapList(x); len(rows) == len(x) && len(rows) > 0 {
				walk(name, rows, depth)
				return
			}
			for i, r := range x {
				walk(fmt.Sprintf("%s[%d]", name, i), r, depth)
			}
		default:
			var generated []byte
			sourceName := ""
			if name == "Decoded Bytes" || name == "Mechanism Token" {
				generated = bytesValue(x)
				sourceName = "Authentication token · decoded bytes"
			}
			if name == "Stream Data" && authenticatedQUIC {
				generated = bytesValue(x)
				sourceName = "QUIC STREAM · authenticated plaintext"
			}
			if name == "Authenticated Plaintext" && authenticatedTLS {
				generated = bytesValue(x)
				sourceName = "TLS record · authenticated plaintext · application gap"
			}
			if generated != nil {
				x = generated
			}
			value := formatFieldValue(name, x)
			label := name
			if protocol == "tls" {
				value = tlsDisplayValue(name, x)
				if name == "Authentication Verified" {
					label = "Record AEAD verified"
				}
			}
			add(label+": "+value, false)
			if len(generated) > 0 {
				sources = append(sources, detailSource{Name: sourceName, Data: bytes.Clone(generated)})
				f := &out[len(out)-1]
				f.source, f.start, f.end, f.hasRange = len(sources), 0, len(generated), true
			}
			if span := wireRanges[name]; depth == 0 && len(span) == 2 && currentSource > 0 && span[0] >= 0 && span[1] > span[0] && span[1] <= len(sources[currentSource-1].Data) {
				f := &out[len(out)-1]
				f.source, f.start, f.end, f.hasRange = currentSource, span[0], span[1], true
			}
			if protocol == "tls" && currentSource > 0 {
				f := &out[len(out)-1]
				if ranges, ok := map[string][2]int{"Record Type": {0, 1}, "Record Version": {1, 3}, "Record Length": {3, 5}}[name]; ok && len(sources[currentSource-1].Data) >= 5 {
					f.source, f.start, f.end, f.hasRange = currentSource, ranges[0], ranges[1], true
				}
			}
		}
	}
	for _, e := range ordered {
		if e == nil {
			continue
		}
		protocol, currentSource = e.Protocol, 0
		wireRanges = map[string][]int{}
		switch ranges := e.Session["Wire Field Ranges"].(type) {
		case map[string][]int:
			wireRanges = ranges
		case map[string]any:
			for k, v := range ranges {
				if a, ok := v.([]any); ok && len(a) == 2 {
					wireRanges[k] = []int{int(number(a[0])), int(number(a[1]))}
				}
			}
		}
		authenticatedTLS = e.Protocol == "tls" && e.Session["Authentication Verified"] == true
		authenticatedQUIC = (e.Protocol == "quic" || e.Protocol == "http3" || e.Protocol == "doq") && e.Session["Authentication Verified"] == true
		kind := e.SourceBytes.Kind
		if kind == "" {
			kind = "message"
		}
		if !credibleTLSIdentity(e) {
			if strongTLSRecord(e) {
				kind += " · TLS record framing; handshake not captured"
			} else {
				kind += " · TLS? candidate record; handshake not captured; session unconfirmed"
			}
		}
		label := eventProtocolName(e)
		if !credibleTLSIdentity(e) {
			label = "TLS?"
			if strongTLSRecord(e) {
				label = "TLS · record framing only"
			}
		}
		add(fmt.Sprintf("%s · PDU #%d · %s", label, e.ID, kind), true)
		if len(e.Raw) > 0 {
			name := eventProtocolName(e)
			if e.Protocol == "grpc" {
				name = "HTTP/2 frame (gRPC)"
			}
			if e.Protocol == "http3" || e.Protocol == "doq" {
				name = "QUIC wire packet (" + eventProtocolName(e) + ")"
			}
			if len(authenticationLayers(e)) > 0 {
				name = "SMB2 wire PDU (authentication)"
			}
			sources = append(sources, detailSource{Name: name + " · " + kind + " PDU", Data: bytes.Clone(e.Raw)})
			currentSource = len(sources)
			out[len(out)-1].source, out[len(out)-1].hasRange, out[len(out)-1].end = currentSource, true, len(e.Raw)
		}
		add(applicationEventSummary(e), false)
		if !credibleTLSIdentity(e) {
			add("TLS session identity: unconfirmed · complete record header/body, handshake context not observed", false)
			add("Negotiated TLS version: unavailable · record legacy version is not negotiation evidence", false)
			add("Plaintext: unavailable · record authentication has not been verified", false)
		}
		values := map[string]any{}
		for k, v := range e.Fields {
			values[k] = v
		}
		for k, v := range e.Session {
			values[k] = v
		}
		delete(values, "Wire Field Ranges")
		if e.Protocol == "ssh" {
			for _, key := range []string{"Packet Name", "Kex Algorithm", "Host Key Algorithm", "Host Key Fingerprint", "Encryption C2S", "Encryption S2C", "Signature Verified", "Peer Identity Verified", "Identification", "Version", "Kex Family", "Host Key Format", "First Kex Packet Follows", "Languages C2S", "Languages S2C"} {
				if v, ok := values[key]; ok {
					walk(key, v, 0)
					delete(values, key)
				}
			}
		}
		if e.Protocol == "ssdp" {
			for _, key := range []string{"Message Kind", "Notification Subtype", "Discovery Target", "USN", "Location", "Maximum Age Seconds", "Boot ID", "Next Boot ID", "Config ID", "Conformance Warnings", "Identity Verified"} {
				if v, ok := values[key]; ok {
					walk(key, v, 0)
					delete(values, key)
				}
			}
		}
		for _, key := range []string{"Negotiated Version", "Content Visibility", "Secret Lookup Status", "Cipher Suite", "SNI", "ALPN", "Handshake Messages", "Alert Level", "Alert Description", "Stream ID", "Request Method", "Headers", "GRPC Status", "GRPC Messages", "Authentication", "Guest Session", "Record Type", "Record Version", "Record Length", "Inner Content Type", "Authentication Verified", "Certificate Trust"} {
			if v, ok := values[key]; ok {
				walk(key, v, 0)
				delete(values, key)
			}
		}
		if len(values) > 0 {
			depth := 0
			if len(wireRanges) > 0 {
				depth = -1
			}
			walk("Other decoded fields", values, depth)
		}
		add(fmt.Sprintf("Status: %s · %s · source %s → %s", e.Status, e.Completeness, e.Source, e.Destination), false)
		if e.Error != "" {
			add("Error: "+e.Error, false)
		}
		if e.SourceBytes.ParentPDU != 0 {
			add(fmt.Sprintf("Parent PDU: #%d · generated fields have no direct frame offsets", e.SourceBytes.ParentPDU), false)
		}
	}
	if len(events) == 0 {
		out = append(out, field{text: "No retained protocol events; detail unavailable for this retention window."})
	}
	return out, sources
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
		if e == nil {
			continue
		}
		if e.Status != "decoded" && e.Status != "deferred" {
			continue
		}
		name := eventProtocolName(e)
		if !credibleTLSIdentity(e) {
			name = "TLS?"
			if strongTLSRecord(e) {
				name = "TLS"
			}
		}
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
	for _, id := range p.historyPacketEventIDs() {
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

func (p *capturedPacket) historyPacketEventIDs() []uint64 {
	if p.history == nil {
		return nil
	}
	if p.physicalRecord {
		return p.history.PhysicalPacketEventIDs(p.reference, 128)
	}
	return p.history.PacketEventIDs(p.reference, 128)
}

func boundedEvent(e *pcaputil.ProtocolEvent, remaining *int) *pcaputil.ProtocolEvent {
	cost := e.RetainedBytes()
	if cost <= *remaining {
		*remaining -= cost
		return e
	}
	return &pcaputil.ProtocolEvent{ID: e.ID, FlowID: e.FlowID, Protocol: e.Protocol, Status: "limited", Completeness: "limited", Summary: "Detail unavailable: snapshot byte budget", Domain: e.Domain, Source: e.Source, Destination: e.Destination, ResponseTo: e.ResponseTo, Transport: e.Transport, Direction: e.Direction, Timestamp: e.Timestamp, Admission: e.Admission, ExpertCode: string(pcaputil.ErrResourceExceeded)}
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
