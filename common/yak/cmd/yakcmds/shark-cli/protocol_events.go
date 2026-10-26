package sharkcli

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

var semanticFieldPriority = map[string]int{
	"Command": 4, "Message Stream ID": 5, "Chunk Stream ID": 6, "Transaction Number": 6, "Timestamp": 7, "AMF0 Values": 8,
	"Message Kind": 2, "Notification Subtype": 3, "Discovery Target": 4, "USN": 5, "Location": 6, "Maximum Age Seconds": 7,
	"Protocol": 0, "ID": 0, "Status": 1, "Packet Name": 2, "Message Name": 2, "Mechanism": 2,
	"Authentication Verified": 3, "User Name": 4, "Domain": 5, "Method": 4, "Request Method": 4, "Request URI": 5, "Query": 5, "SQL": 5, "Topic": 5,
	"Message Type": 6, "QNAME": 4, "QTYPE Name": 5, "Response": 3, "RCODE": 4, "Questions": 5, "Answers": 6, "Headers": 5,
	"MIC Verified": 7, "Identity Evidence": 8, "Decoded Bytes": 900, "Mechanism Token": 900, "Decoded Fields": 1000,
}

var ripngEntryPriority = map[string]int{
	"Entry Type": 1, "IPv6 Prefix": 2, "Prefix Length": 3, "Route Tag": 4, "Metric": 5,
	"Canonical Prefix": 6, "Effective Next Hop Advisory": 7, "Advertised Unreachable": 8,
	"Ignored Next Hop Parameters": 9, "Conformance Warning": 10,
}

var isisEntryPriority = map[string]int{
	"TLV Name": 0, "Type": 1, "Length": 2, "Area Addresses": 3, "Protocols Supported": 3, "Interface Addresses": 3,
	"IS Neighbors": 3, "ES Neighbor Count": 3, "Shared Metrics": 4, "Neighbors": 5, "LSP Entries": 3, "Reachability Entries": 3, "Hostname": 3, "Adjacency State Name": 3,
	"Neighbor Node ID": 4, "System ID": 4, "SNPA": 4, "Address": 4, "LSP ID": 4, "Prefix Address": 4, "Prefix Length": 5,
	"Canonical Prefix": 6, "Metric": 7, "Sequence Number": 7, "Remaining Lifetime Seconds": 8,
	"Reported Checksum": 9, "Sub-TLVs": 20, "Unselected Extensions": 800, "Conformance Warning": 801,
	"Observation Scope": 802, "Opaque Value": 900, "Padding Bytes": 901,
}

var netflowV9EntryPriority = map[string]int{
	"Message Length": 1, "Observation Domain ID": 2, "Export Time": 5, "Scope Field Count": 2, "Enterprise": 1, "Enterprise Number": 2,
	"Protocol Name": 0, "Version": 1, "Source ID": 2, "Sequence Number": 3, "System Uptime Milliseconds": 4, "Unix Seconds": 5, "Unix Nanoseconds": 5,
	"Export Timestamp Valid": 5, "Export Timestamp Failure": 5,
	"Engine Type": 5, "Engine ID": 5, "Sampling Mode": 6, "Sampling Interval": 6, "Sequence Scope": 7, "Header Record Count": 6, "Template Record Count": 7, "Data Record Count": 8, "Record Count Verified": 9,
	"Data Decode Unavailable": 10, "Context Reset": 11, "Context Retention Warning": 12, "FlowSets": 20,
	"FlowSet ID": 0, "FlowSet Length": 1, "Record Count": 2, "Template Scope": 3, "Template Packet References": 4,
	"Templates": 5, "Records": 6, "Template ID": 0, "Field Count": 1, "Scope Length": 1, "Options Length": 2, "Fields": 4,
	"Field Type": 0, "Field Length": 1, "Scope": 2, "Field Name": 3, "Value": 4, "Value Kind": 5,
	"Template Definition SHA256": 800, "Template PDU SHA256": 801, "Template Definition Offset": 802, "Template Definition Length": 803,
	"Opaque Data": 900, "Padding Bytes": 901, "Observation Scope": 950, "Template Lease Policy": 951,
}

var ipfixHeaderPriority = map[string]int{
	"Protocol Name": 0, "Version": 1, "Message Length": 2, "Export Time": 3,
	"Sequence Number": 4, "Observation Domain ID": 5,
}

var http3FramePriority = map[string]int{"Frame Type": 0, "Frame Type Value": 1, "Payload Length": 2, "Frame Wire Length": 3, "Stream ID": 4, "Stream Offset": 5, "Direction": 6, "Frame Header Bytes": 7, "Headers": 8, "DATA Length": 9, "Payload": 10, "QPACK Block": 10, "Header Conformance Status": 11, "Header Conformance Error": 12, "Header Conformance Scope": 13, "Message Conformance": 14, "Byte Source": 50, "Payload Byte Source": 51, "Frame SHA256": 52}

var http3StreamPriority = map[string]int{"Frame Type": 0, "Stream ID": 1, "Offset": 2, "Length": 3, "FIN": 4, "Contiguous Offset": 5, "Delivered Bytes": 6, "HTTP3 Frame Error": 7, "Stream Data": 8, "HTTP3 Frames": 9, "Byte Source": 50, "Delivered Byte Source": 51}

// Detail snapshots include owned public and private parser evidence, unlike
// the compact scalar observations used by streaming JSON. Keep a separate
// bounded allowance for the selected packet's native protocol tree. Rendering
// still enforces its 4096-node/32-depth limit and reports omitted content.
const packetDetailSnapshotBytes = 4 << 20

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
	seenH3Fault := map[uint64]bool{}
	for _, e := range ordered {
		if e == nil {
			continue
		}
		fault, ok := e.Session["HTTP3 Connection Error"].(map[string]any)
		if !ok || fault["Error Name"] != "H3_FRAME_ERROR" {
			continue
		}
		first := number(fault["First PDU ID"])
		if !seenH3Fault[first] {
			add(fmt.Sprintf("HTTP/3 H3_FRAME_ERROR: connection violation · first PDU #%d", first), false)
			add("Later decoded frames do not certify a conforming connection", false)
			seenH3Fault[first] = true
		}
	}
	if verified > 0 && rejected > 0 {
		add(fmt.Sprintf("TLS records: %d authenticated, %d rejected · verification is per record", verified, rejected), false)
	}
	// Packet authentication never covers an unparsed datagram neighbour. Put
	// this range warning before possibly large authenticated CRYPTO/STREAM trees.
	for _, e := range ordered {
		if e != nil && e.Protocol == "quic" && e.Admission == "unparsed-datagram-remainder" {
			add(fmt.Sprintf("QUIC remainder #%d: %d bytes · authentication not verified", e.ID, e.Length), false)
			add(e.Summary, false)
		}
	}
	var protocol string
	var quicHeaderForm string
	var routingVersion uint64
	var currentSource int
	var wireRanges map[string][]int
	var authenticatedQUIC bool
	var authenticatedTLS bool
	var http3FrameKind string
	var http3StreamID uint64
	var http3HeaderSource int
	var http3HeaderRanges map[string][]int
	var fieldViewLimited bool
	var walk func(string, any, int)
	walk = func(name string, v any, depth int) {
		if len(out) >= 4096 || depth > 32 {
			fieldViewLimited = true
			return
		}
		if name == "Protected Payload" && (protocol == "quic" || protocol == "http3" || protocol == "doq") {
			if octets, ok := fieldOctetList(v); ok {
				v = octets
			}
		}
		switch x := v.(type) {
		case map[string]any:
			if protocol == "http3" {
				if x["Frame Type"] == "STREAM" {
					previous := http3StreamID
					defer func() { http3StreamID = previous }()
					http3StreamID = number(x["Stream ID"])
				}
				if strings.HasPrefix(name, "HTTP3 Frames[") {
					previous := http3FrameKind
					previousID, previousHeader, previousRanges := http3StreamID, http3HeaderSource, http3HeaderRanges
					defer func() {
						http3FrameKind = previous
						http3StreamID = previousID
						http3HeaderSource = previousHeader
						http3HeaderRanges = previousRanges
					}()
					http3FrameKind = textValue(x["Frame Type"])
					if id, ok := x["Stream ID"]; ok {
						http3StreamID = number(id)
					}
					http3HeaderSource, http3HeaderRanges = 0, nil
					if header := bytesValue(x["Frame Header Bytes"]); authenticatedQUIC && len(header) >= 2 && len(header) <= 16 {
						sources = append(sources, detailSource{Name: fmt.Sprintf("HTTP/3 header · stream %d", http3StreamID), Data: bytes.Clone(header)})
						http3HeaderSource = len(sources)
						http3HeaderRanges = map[string][]int{}
						if ranges, ok := x["Frame Header Ranges"].(map[string]any); ok {
							for key, value := range ranges {
								if pair, ok := value.([]any); ok && len(pair) == 2 {
									a, b := int(number(pair[0])), int(number(pair[1]))
									if a >= 0 && b > a && b <= len(header) {
										http3HeaderRanges[key] = []int{a, b}
									}
								}
							}
						}
					}
				}
			}
			if protocol == "ntp" || protocol == "hsrp" || protocol == "glbp" || protocol == "bfd" || protocol == "ldp" || protocol == "pim" || protocol == "l2tp" || protocol == "stp" || protocol == "ripng" || protocol == "isis" || (protocol == "netflow-v9" || protocol == "netflow-v5" || protocol == "ipfix") {
				if local, ok := x["Wire Field Ranges"].(map[string]any); ok {
					previous := wireRanges
					defer func() { wireRanges = previous }()
					wireRanges = map[string][]int{}
					for k, v := range local {
						if a, ok := v.([]any); ok && len(a) == 2 {
							wireRanges[k] = []int{int(number(a[0])), int(number(a[1]))}
						}
					}
				}
			}
			if name, ok := x["Display Name"].(string); ok && strings.Contains("|dns|mdns|llmnr|dot|doh|doq|", "|"+protocol+"|") {
				view := make(map[string]any, len(x))
				for k, v := range x {
					if k != "Display Name" {
						view[k] = v
					}
				}
				if escaped, exists := x["Name"].(string); exists && escaped != name {
					view["Escaped Name"] = escaped
				}
				view["Name"] = name
				x = view
			}
			add(name, true)
			keys := make([]string, 0, len(x))
			for k := range x {
				if k == "Wire Field Ranges" || protocol == "http3" && k == "Frame Header Ranges" {
					continue
				}
				if protocol == "ipfix" && k == "Enterprise Number" && x["Enterprise"] == false {
					continue // the standard IE does not carry a PEN, including PEN0
				}
				keys = append(keys, k)
			}
			sort.Slice(keys, func(i, j int) bool {
				a, b := fieldPriority(keys[i]), fieldPriority(keys[j])
				if protocol == "ripng" {
					if rank, ok := ripngEntryPriority[keys[i]]; ok {
						a = rank
					}
					if rank, ok := ripngEntryPriority[keys[j]]; ok {
						b = rank
					}
				}
				if protocol == "isis" {
					if rank, ok := isisEntryPriority[keys[i]]; ok {
						a = rank
					}
					if rank, ok := isisEntryPriority[keys[j]]; ok {
						b = rank
					}
				}
				if protocol == "http3" && http3FrameKind != "" {
					if rank, ok := http3FramePriority[keys[i]]; ok {
						a = rank
					}
					if rank, ok := http3FramePriority[keys[j]]; ok {
						b = rank
					}
				}
				if protocol == "http3" && x["Frame Type"] == "STREAM" {
					if rank, ok := http3StreamPriority[keys[i]]; ok {
						a = rank
					}
					if rank, ok := http3StreamPriority[keys[j]]; ok {
						b = rank
					}
				}
				if protocol == "netflow-v9" || protocol == "netflow-v5" || protocol == "ipfix" {
					if rank, ok := netflowV9EntryPriority[keys[i]]; ok {
						a = rank
					}
					if rank, ok := netflowV9EntryPriority[keys[j]]; ok {
						b = rank
					}
					if protocol == "ipfix" {
						if rank, ok := ipfixHeaderPriority[keys[i]]; ok {
							a = rank
						}
						if rank, ok := ipfixHeaderPriority[keys[j]]; ok {
							b = rank
						}
					}
					if protocol == "netflow-v5" {
						if keys[i] == "Records" {
							a = 20
						}
						if keys[j] == "Records" {
							b = 20
						}
					}
				}
				if a != b {
					return a < b
				}
				return keys[i] < keys[j]
			})
			for _, k := range keys {
				label := k
				if (protocol == "netflow-v9" || protocol == "netflow-v5" || protocol == "ipfix") && k == "Value" {
					if name, ok := x["Field Name"].(string); ok && name != "" {
						label = safeText(name)
						if wireRanges != nil {
							wireRanges[label] = wireRanges[k]
						}
					}
				}
				walk(label, x[k], depth+1)
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
			if protocol == "quic" || protocol == "http3" || protocol == "doq" {
				switch name {
				case "DCID", "SCID", "CID", "Token", "Retry Token", "Reset Token":
					if octets := bytesValue(x); octets != nil {
						x = octets
					}
				}
			}
			if name == "Frame Header Bytes" && http3HeaderSource > 0 {
				v = sources[http3HeaderSource-1].Data
				x = v
			}
			var generated []byte
			sourceName := ""
			if protocol == "ntp" {
				if name == "Control Data" || name == "Padding Bytes" || name == "Authenticator Bytes" || name == "Unselected Trailer Bytes" || strings.HasSuffix(name, " Timestamp") {
					x = bytesValue(x)
				}
				if name == "Reassembled Control Data" {
					generated = bytesValue(x)
					sourceName = "NTP control response · reassembled data · sender unverified"
				}
			}
			if name == "Decoded Bytes" || name == "Mechanism Token" {
				generated = bytesValue(x)
				sourceName = "Authentication token · decoded bytes"
			}
			if name == "Stream Data" && authenticatedQUIC {
				generated = bytesValue(x)
				sourceName = "QUIC STREAM · authenticated plaintext"
			}
			if name == "Protected Payload" && (protocol == "quic" || protocol == "http3" || protocol == "doq") {
				generated = bytesValue(x)
				sourceName = "QUIC wire remainder · extracted octets"
			}
			if protocol == "http3" && authenticatedQUIC {
				if name == "Partial Frame Bytes" {
					generated = bytesValue(x)
					sourceName = fmt.Sprintf("HTTP/3 partial frame · stream %d · layout error", http3StreamID)
				}
				if name == "Payload" && http3FrameKind == "DATA" {
					generated = bytesValue(x)
					sourceName = fmt.Sprintf("HTTP/3 DATA · reassembled · stream %d", http3StreamID)
				}
				if name == "QPACK Block" && http3FrameKind == "HEADERS" {
					generated = bytesValue(x)
					sourceName = fmt.Sprintf("HTTP/3 HEADERS · QPACK · stream %d", http3StreamID)
				}
			}
			if name == "Authenticated Plaintext" && authenticatedTLS {
				generated = bytesValue(x)
				sourceName = "TLS record · authenticated plaintext · application gap"
			}
			if protocol == "syslog" && name == "Message Bytes" {
				// Worker JSON encodes this native []byte field as Base64. Its
				// owned extracted payload is useful for binary MSG-ANY inspection;
				// it is not a fabricated contiguous range in the physical frame.
				generated = bytesValue(x)
				sourceName = "Syslog MSG · extracted payload"
			}
			if generated != nil {
				x = generated
			}
			value := formatFieldValue(name, x)
			emptyCID := x == nil
			if b, ok := x.([]byte); ok {
				emptyCID = len(b) == 0
			}
			if depth == 0 && quicHeaderForm == "short" && (protocol == "quic" || protocol == "http3" || protocol == "doq") && (name == "Version" || name == "SCID") {
				value = "not carried in short header"
			}
			if depth == 0 && quicHeaderForm == "long" && (protocol == "quic" || protocol == "http3" || protocol == "doq") && (name == "DCID" || name == "SCID") && emptyCID {
				if span := wireRanges[name]; len(span) == 2 && span[0] == span[1] && span[0] >= 0 && currentSource > 0 && span[1] <= len(sources[currentSource-1].Data) {
					value = "0 bytes (empty connection ID)"
				}
			}
			if protocol == "ipfix" && name == "Export Time" && number(x) <= 0xffffffff {
				value = fmt.Sprintf("%s · Unix seconds %d", time.Unix(int64(number(x)), 0).UTC().Format(time.RFC3339), number(x))
			}
			if protocol == "syslog" && x == nil && (name == "App Name" || name == "Hostname" || name == "Proc ID" || name == "Message ID") {
				value = "not provided"
			}
			if protocol == "ldp" {
				switch name {
				case "Hold Time":
					value = fmt.Sprintf("%d s (advertised; negotiated unobserved)", number(x))
					if number(x) == 0 {
						value = "0 (advertised default; see Hold Time Meaning)"
					}
					if number(x) == 65535 {
						value = "65535 (advertised infinite; negotiated unobserved)"
					}
				case "GTSM":
					value += " (advertised capability; enforcement unverified)"
				case "Message Type":
					value = fmt.Sprintf("0x%04x", number(x))
				case "TLV Type":
					value = fmt.Sprintf("0x%04x", number(x))
				case "Generic Label":
					if label := map[uint64]string{0: "IPv4 Explicit NULL", 1: "Router Alert", 2: "IPv6 Explicit NULL", 3: "Implicit NULL"}[number(x)]; label != "" {
						value = fmt.Sprintf("%d (%s; advertised)", number(x), label)
					}
				}
			}
			if protocol == "hsrp" || protocol == "glbp" || protocol == "bfd" {
				value = routingFieldText(protocol, name, x, routingVersion, value)
			}
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
			if http3HeaderSource > 0 {
				span := http3HeaderRanges[name]
				if name == "Frame Header Bytes" {
					span = []int{0, len(sources[http3HeaderSource-1].Data)}
				}
				if len(span) == 2 {
					f := &out[len(out)-1]
					f.source, f.start, f.end, f.hasRange = http3HeaderSource, span[0], span[1], true
				}
			}
			if span := wireRanges[name]; (depth == 0 || protocol == "hsrp" || protocol == "glbp" || protocol == "bfd" || protocol == "ldp" || protocol == "pim" || protocol == "l2tp" || protocol == "stp" || protocol == "ripng" || protocol == "isis" || (protocol == "netflow-v9" || protocol == "netflow-v5" || protocol == "ipfix")) && len(span) == 2 && currentSource > 0 && span[0] >= 0 && span[1] > span[0] && span[1] <= len(sources[currentSource-1].Data) {
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
		quicHeaderForm = textValue(e.Session["Header Form"])
		routingVersion = number(e.Session["Protocol Version"])
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
		http3FrameKind, http3StreamID = "", number(e.Session["HTTP3 Stream ID"])
		http3HeaderSource, http3HeaderRanges = 0, nil
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
		if e.Protocol == "" && e.Transport == "udp" && e.Error != "" {
			label = "UDP carrier diagnostic"
		}
		if !credibleTLSIdentity(e) {
			label = "TLS?"
			if strongTLSRecord(e) {
				label = "TLS · record framing only"
			}
		}
		add(fmt.Sprintf("%s · PDU #%d · %s", label, e.ID, kind), true)
		if len(e.Raw) > 0 {
			name := eventProtocolName(e)
			if e.Protocol == "" && e.Transport == "udp" && e.Error != "" {
				name = "Unadmitted UDP payload"
			}
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
		if e.Protocol == "http3" && e.Error != "" {
			add("HTTP message diagnostic: "+e.Error, false)
			if rows, ok := e.Session["HTTP3 Stream Errors"]; ok {
				walk("HTTP3 Stream Errors", rows, 0)
			}
		}
		if e.Protocol == "" && e.Transport == "udp" && e.Error != "" {
			add("Carrier failure: "+e.Error, false)
			add("Application identity unavailable: UDP payload not decoded", false)
		}
		if notice := isisMalformedNotice(e); notice != "" {
			// A long otherwise valid header/checksum summary can hide the reason
			// for rejection. Keep the diagnostic near the beginning of details.
			add(notice+" · "+textValue(e.Session["TLV Decode Failure"]), false)
		}
		if e.SourceBytes.CompletedIn != nil {
			kind := "Message"
			if e.Session["Native Carrier"] == true && (e.Protocol == "quic" || e.Protocol == "http3" || e.Protocol == "doq") {
				kind = "QUIC wire packet"
			}
			add(fmt.Sprintf("%s bytes complete in frame #%d · %d contributing packet(s)", kind, e.SourceBytes.CompletedIn.Number, len(e.SourceBytes.PacketRefs)), false)
		}
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
		if e.Protocol == "ntp" {
			for _, key := range []string{"Packet Name", "Version", "Mode", "Legacy Reserved", "Leap Indicator", "Role", "Opcode Name", "Opcode", "Sequence Number", "Response", "Control Error", "Status Word", "Status Word Type", "Association ID", "Fragment Offset", "Data Count", "More Fragments", "Association", "Request PDU ID", "Requested Association ID", "Response Association Observation", "Observed Duplicate PDU", "Control Reassembly", "Reassembly Complete", "Context Retention Warning", "Stratum", "Poll", "Precision", "Root Delay Seconds", "Root Dispersion Seconds", "Synchronizing Distance Seconds", "Synchronizing Dispersion Seconds", "Estimated Drift Rate", "Reference ID", "Transmit Timestamp", "Transmit Timestamp Seconds", "Transmit Timestamp Fraction", "Control Data", "Reassembled Control Data", "Control Payload Failure", "Reported Association Count", "Reported Association List Decode", "Reported Associations"} {
				if value, ok := values[key]; ok {
					walk(key, value, 0)
					delete(values, key)
				}
			}
		}
		delete(values, "Observed Header Layers") // already rendered as the ordered breadcrumb
		if e.Protocol == "http3" && e.Error != "" {
			delete(values, "HTTP3 Stream Errors") // early, before possibly large byte witnesses
		}
		if e.Protocol == "isis" {
			for _, key := range []string{"Protocol Name", "Packet Name", "PDU Type", "Source System ID", "Holding Timer Seconds", "Local Circuit ID", "LAN ID", "Priority", "LSP ID", "Sequence Number", "Remaining Lifetime Seconds", "Wire Checksum", "Checksum Observation", "Checksum Verified", "Checksum Failure", "Source Node ID", "Source Circuit ID", "Start LSP ID", "End LSP ID", "Fixed Header Complete", "Wire Grammar Complete", "TLV Decode Failure", "TLVs", "Conformance Warning", "Authentication Verified", "Database Completeness", "Observation Scope"} {
				if v, ok := values[key]; ok {
					walk(key, v, 0)
					delete(values, key)
				}
			}
		}
		if e.Protocol == "ripng" {
			for _, key := range []string{"Packet Name", "Command", "Version", "Reserved", "Conformance Warning", "Whole Table Request", "Route Count", "Next Hop Count", "Entries", "Authentication Verified", "Observation Scope"} {
				if v, ok := values[key]; ok {
					walk(key, v, 0)
					delete(values, key)
				}
			}
		}
		if e.Protocol == "l2tp" && number(eventValues(e)["Version"]) == 3 {
			delete(values, "Opaque Session Payload")
			delete(values, "Ethernet Bytes")
			for _, key := range []string{"Packet Name", "Version", "Control", "Control Connection ID", "Session ID", "Flags", "Length", "Ns", "Nr", "Connection Context", "Control Setup Complete", "Session Setup Complete", "Observed Control Connection", "Observed Session", "Receiver Proposal Side", "Cookie Length", "Cookie", "Sublayer Type", "Data Sequencing", "Sequence Present", "Sequence Number", "Sequence Observation", "Unobserved Sequence Count", "Payload Decoded", "Payload Admission", "Opaque Payload Bytes", "Association Diagnostic", "Conformance Warning", "AVPs", "Control Context Proof", "Context Proof", "Wire Completeness", "Authentication Verified", "Observation Scope"} {
				if v, ok := values[key]; ok {
					walk(key, v, 0)
					delete(values, key)
				}
			}
		}
		if e.Protocol == "stp" {
			for _, key := range []string{"Protocol Name", "Packet Name", "Version", "BPDU Type", "Conformance Warning", "Root Priority", "Root System ID Extension", "Root MAC", "Root Path Cost", "Bridge Priority", "Bridge System ID Extension", "Bridge MAC", "Port ID", "Port Role Name", "Topology Change", "Proposal", "Learning", "Forwarding", "Agreement", "Message Age Seconds", "Max Age Seconds", "Hello Time Seconds", "Forward Delay Seconds", "MST Configuration Name", "MST Configuration Revision", "MST Configuration Digest", "CIST Internal Root Path Cost", "CIST Bridge MAC", "CIST Remaining Hops", "MST Instances", "PVST TLVs", "MST Digest Verified", "Observation Scope"} {
				if v, ok := values[key]; ok {
					walk(key, v, 0)
					delete(values, key)
				}
			}
		}
		if e.Protocol == "pim" {
			// Lead with the actual control message and advertised (S,G), not
			// unrelated generic metadata or a large base64 copy of its body.
			// The complete PDU remains selectable in the byte-source pane.
			delete(values, "Encapsulated Bytes")
			for _, key := range []string{"Packet Name", "Version", "Type", "Null Register", "Border", "Register IP Header", "Upstream Neighbor", "Group Count", "Hold Time", "Group", "Source", "Originator", "Groups", "Options", "Fragment Tag", "Hash Mask Length", "BSR Priority", "BSR", "Priority", "RP", "Prefix Count", "Advertised Groups", "RPT", "Metric Preference", "Metric", "Mask Length", "TTL", "Prune Indicator", "Prune Now", "Assert Override", "Interval", "Checksum", "Checksum Scope", "Checksum Verified", "Authentication Verified", "Observation Scope", "Bootstrap Reassembly"} {
				if v, ok := values[key]; ok {
					walk(key, v, 0)
					delete(values, key)
				}
			}
		}
		if e.Protocol == "ldp" {
			for _, key := range []string{"Packet Name", "Version", "PDU Length", "LSR ID", "Label Space ID", "Observation Scope", "Messages"} {
				if v, ok := values[key]; ok {
					walk(key, v, 0)
					delete(values, key)
				}
			}
		}
		if e.Protocol == "hsrp" || e.Protocol == "glbp" || e.Protocol == "bfd" {
			delete(values, "Strong Identity")
			for _, key := range []string{"Packet Name", "Version", "Protocol Version", "Opcode", "State", "Group", "Virtual IP", "Identifier", "Owner ID", "Priority", "Hello Time", "Hold Time", "My Discriminator", "Your Discriminator", "Poll", "Final", "Detect Multiplier", "Desired Min TX", "Required Min RX", "Required Min Echo RX"} {
				if v, ok := values[key]; ok {
					walk(key, v, 0)
					delete(values, key)
				}
			}
		}
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
		for _, key := range []string{"Negotiated Version", "Content Visibility", "Secret Lookup Status", "Cipher Suite", "SNI", "ALPN", "HTTP3 Connection Error", "HTTP3 Connection Conformance", "HTTP3 Message Conformance", "Handshake Messages", "Alert Level", "Alert Description", "Stream ID", "Request Method", "Headers", "GRPC Status", "GRPC Messages", "Authentication", "Guest Session", "Record Type", "Record Version", "Record Length", "Inner Content Type", "Authentication Verified", "Certificate Trust"} {
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
		state := e.Status
		if e.Completeness != "" && e.Completeness != state {
			state += " · " + e.Completeness
		}
		add(fmt.Sprintf("Status: %s · source %s → %s", state, e.Source, e.Destination), false)
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
	if fieldViewLimited {
		out = append([]field{{text: "Field view limited · retained protocol detail exceeds this view's node/depth budget"}}, out...)
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
	events, _ := p.protocolEventSnapshot(nil)
	return events
}

func (p *capturedPacket) protocolEventSnapshot(known []uint64) ([]*pcaputil.ProtocolEvent, []uint64) {
	if p.history == nil {
		return p.events, nil
	}
	var events []*pcaputil.ProtocolEvent
	remaining := packetDetailSnapshotBytes
	current, complete := p.history.PacketEventWindow(p.reference, p.physicalRecord)
	var ids []uint64
	for _, source := range [][]uint64{p.eventIDs, known, current} {
		for _, id := range source {
			found := false
			for _, old := range ids {
				if old == id {
					found = true
					break
				}
			}
			if found {
				continue
			}
			if len(ids) < 128 {
				ids = append(ids, id)
			} else {
				complete = false
			}
		}
	}
	unavailable := func(id uint64, reason string) {
		events = append(events, &pcaputil.ProtocolEvent{ID: id, Protocol: "message", Status: "limited", Completeness: "evicted", Summary: reason, Error: reason, ExpertCode: string(pcaputil.ErrResourceExceeded)})
	}
	for _, id := range ids {
		if e := retainedProtocolDetail(p.history, id); e != nil {
			if p.retainedEventContributes(e) {
				events = append(events, boundedEvent(e, &remaining))
			} else {
				unavailable(id, "Retained message ID does not prove this packet's contributor identity")
			}
		} else if p.reference.Number != 0 || p.history.Evicted() > 0 {
			unavailable(id, "Protocol detail absent or evicted from bounded message history")
		}
	}
	if !complete {
		unavailable(0, "Packet PDU reference window exceeded; omitted fields are unavailable")
	}
	if len(ids) == 0 && p.history.Evicted() > 0 {
		unavailable(0, "No retained contributor details; message history has evictions")
	}
	return events, ids
}

func (p *capturedPacket) retainedEventContributes(e *pcaputil.ProtocolEvent) bool {
	if p.reference.Number == 0 {
		return true
	} // Legacy caller-supplied associations.
	if len(e.SourceBytes.PacketRefs) == 0 {
		for _, id := range p.eventIDs {
			if id == e.ID {
				return true
			}
		}
		return false
	}
	for _, ref := range e.SourceBytes.PacketRefs {
		want := p.reference
		if p.physicalRecord {
			ref.Domain.Encapsulation, want.Domain.Encapsulation = "", ""
		}
		if ref == want {
			return true
		}
	}
	return false
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
