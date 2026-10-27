package sharkcli

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

const observedPacketFieldBytes = 256 << 10

// Freeze scalar typed values and owned byte-source metadata while the native
// event is being published. Output speed must not decide whether a later
// global history eviction or EOF diagnostic changes an earlier JSON row.
// The TUI retains its separate revision-aware historical contributor queries.
func (p *capturedPacket) recordObservedPDU(e *pcaputil.ProtocolEvent) {
	p.recordPDUAtObservation(e, false)
}

func (p *capturedPacket) recordPublishedPDU(e *pcaputil.ProtocolEvent) {
	p.recordPDUAtObservation(e, true)
}

func (p *capturedPacket) recordPDUAtObservation(e *pcaputil.ProtocolEvent, publication bool) {
	if p.observedFieldsLimited {
		return
	}
	appendPDU := func(row map[string]any) {
		if publication {
			p.publishedPDUs = append(p.publishedPDUs, row)
		} else {
			p.observedPDUs = append(p.observedPDUs, row)
		}
	}
	limited := func() {
		p.observedFieldsLimited = true
		snapshot := pcaputil.DisplayFieldSnapshot{SchemaVersion: pcaputil.DisplayFieldSchemaVersion, Completeness: "limited"}
		p.observedFields = append(p.observedFields, snapshot)
		appendPDU(map[string]any{"id": uint64(0), "protocol": "message", "status": "limited", "completeness": "limited", "summary": "Packet observation field snapshot budget exceeded", "field_values": snapshot})
	}
	if len(p.observedPDUs)+len(p.publishedPDUs) >= 128 {
		limited()
		return
	}
	values := e.DisplayFieldValues()
	row := map[string]any{"id": e.ID, "protocol": e.Protocol, "status": e.Status, "completeness": e.Completeness, "response_to": e.ResponseTo, "byte_source": values.Source, "fields": values.FieldMap(), "field_values": values}
	// Missing context, unsupported profiles and resource refusal are meaningful
	// outcomes on every carrier. Retain their reason just like a malformed UDP
	// packet; omitting it makes a compact row disagree with the owned detail.
	if e.Error != "" || e.ExpertCode != "" {
		row["summary"], row["error"], row["expert"] = e.Summary, e.Error, e.ExpertCode
	}
	wire, err := json.Marshal(row)
	// This queue retains scalar occurrences, not Raw or the parser's private
	// evidence trees. Charge what is actually frozen here; a normal exporter
	// batch can have large private trees and still fit this bounded projection.
	if err != nil || len(wire) > p.observedFieldBudget {
		limited()
		return
	}
	p.observedFieldBudget -= len(wire)
	appendPDU(row)
	if !publication {
		// Availability during processing is separate from a physical packet's
		// field union. Foreign/late publications must not match its filter.
		p.observedFields = append(p.observedFields, values)
	}
}
