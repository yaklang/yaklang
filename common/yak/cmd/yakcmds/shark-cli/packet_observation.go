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
	if p.observedFieldsLimited {
		return
	}
	limited := func() {
		p.observedFieldsLimited = true
		snapshot := pcaputil.DisplayFieldSnapshot{SchemaVersion: pcaputil.DisplayFieldSchemaVersion, Completeness: "limited"}
		p.observedFields = append(p.observedFields, snapshot)
		p.observedPDUs = append(p.observedPDUs, map[string]any{"id": uint64(0), "protocol": "message", "status": "limited", "completeness": "limited", "summary": "Packet observation field snapshot budget exceeded", "field_values": snapshot})
	}
	if len(p.observedPDUs) >= 128 || e.RetainedBytes() > p.observedFieldBudget {
		limited()
		return
	}
	values := e.DisplayFieldValues()
	row := map[string]any{"id": e.ID, "protocol": e.Protocol, "status": e.Status, "completeness": e.Completeness, "response_to": e.ResponseTo, "byte_source": values.Source, "fields": values.FieldMap(), "field_values": values}
	wire, err := json.Marshal(row)
	if err != nil || len(wire) > p.observedFieldBudget {
		limited()
		return
	}
	p.observedFieldBudget -= len(wire)
	p.observedPDUs = append(p.observedPDUs, row)
	p.observedFields = append(p.observedFields, values)
}
