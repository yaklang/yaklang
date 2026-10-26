package sharkcli

import (
	"bytes"
	"encoding/json"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// The message overview needs header sections and frame summaries, while full
// payloads and byte witnesses remain in the selected packet's detail history.
// Build a separate public snapshot: private decoder caches and repeated source
// trees must not consume the bounded conversation budget or mutate history.
func http3ConversationEvent(e *pcaputil.ProtocolEvent) *pcaputil.ProtocolEvent {
	if e.Protocol != "http3" || e.Session == nil {
		return e
	}
	m := map[string]any{}
	for k, v := range e.Session {
		switch v.(type) {
		case string, bool, uint8, uint16, uint32, uint64, int, int64, float64, json.Number:
			m[k] = v
		}
	}
	copyHeaders := func(v any) []map[string]any {
		var out []map[string]any
		for _, h := range mapList(v) {
			out = append(out, map[string]any{"Name": h["Name"], "Value": h["Value"]})
		}
		return out
	}
	copyConformance := func(v any) map[string]any {
		out := map[string]any{}
		if source, ok := v.(map[string]any); ok {
			for k, value := range source {
				switch value.(type) {
				case string, bool, uint64, int, json.Number:
					out[k] = value
				}
			}
		}
		return out
	}
	var groups, frames []map[string]any
	addFrame := func(h map[string]any, sid uint64) {
		if v, ok := h["Stream ID"]; ok {
			sid = number(v)
		}
		row := map[string]any{"Stream ID": sid}
		for _, k := range []string{"Frame Type", "Frame Type Value", "Payload Length", "DATA Length", "Header Kind"} {
			if v, ok := h[k]; ok {
				row[k] = v
			}
		}
		frames = append(frames, row)
		if c, ok := h["Message Conformance"]; ok {
			row["Message Conformance"] = copyConformance(c)
		}
		if headers := copyHeaders(h["Headers"]); len(headers) > 0 {
			groups = append(groups, map[string]any{"Stream ID": sid, "Header Kind": h["Header Kind"], "Headers": headers, "Header Conformance Status": h["Header Conformance Status"], "Header Conformance Error": h["Header Conformance Error"]})
		}
	}
	var frameErrors []map[string]any
	for _, fr := range mapList(e.Session["Frames"]) {
		if fault, ok := fr["HTTP3 Frame Error"]; ok {
			frameErrors = append(frameErrors, copyConformance(fault))
		}
		for _, h := range mapList(fr["HTTP3 Frames"]) {
			addFrame(h, number(fr["Stream ID"]))
		}
	}
	for _, resumed := range mapList(e.Session["QPACK Resumed Streams"]) {
		for _, h := range mapList(resumed["HTTP3 Frames"]) {
			addFrame(h, number(resumed["HTTP3 Stream ID"]))
		}
	}
	if len(frames) == 0 {
		for _, h := range mapList(e.Session["HTTP3 Frames"]) {
			addFrame(h, number(e.Session["HTTP3 Stream ID"]))
		}
	}
	m["HTTP3 Frames"], m["Headers"] = frames, copyHeaders(e.Session["Headers"])
	if len(groups) == 0 && len(mapList(m["Headers"])) > 0 {
		groups = append(groups, map[string]any{"Stream ID": number(e.Session["HTTP3 Stream ID"]), "Headers": m["Headers"]})
	}
	m["HTTP3 Message Headers"] = groups
	if c, ok := e.Session["HTTP3 Connection Error"]; ok {
		m["HTTP3 Connection Error"] = copyConformance(c)
	}
	if len(frameErrors) > 0 {
		m["HTTP3 Frame Errors"] = frameErrors
	}
	if c, ok := e.Session["HTTP3 Message Conformance"]; ok {
		m["HTTP3 Message Conformance"] = copyConformance(c)
	}
	m["Conversation View Scope"] = "message headers and frame summaries; full payload and byte provenance remain in packet fields"
	source := e.SourceBytes
	source.PacketRefs = append([]pcaputil.PacketReference(nil), source.PacketRefs...)
	source.ParentPDUs = append([]uint64(nil), source.ParentPDUs...)
	if source.CompletedIn != nil {
		ref := *source.CompletedIn
		source.CompletedIn = &ref
	}
	return &pcaputil.ProtocolEvent{ID: e.ID, FlowID: e.FlowID, ResponseTo: e.ResponseTo, TransactionID: e.TransactionID, Timestamp: e.Timestamp, Transport: e.Transport, Source: e.Source, Destination: e.Destination, Protocol: e.Protocol, Direction: e.Direction, Offset: e.Offset, Length: e.Length, Status: e.Status, Summary: e.Summary, Rule: e.Rule, Entry: e.Entry, Error: e.Error, ExpertCode: e.ExpertCode, Profile: e.Profile, Admission: e.Admission, Completeness: e.Completeness, Domain: e.Domain, SourceBytes: source, CarrierLayers: append([]string(nil), e.CarrierLayers...), Raw: bytes.Clone(e.Raw), Session: m}
}
