package sharkcli

import (
	"bytes"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

const wcfObservedContext = "Observed WCF Session"

// A display projection, not another dissector. All offsets and chunk lengths
// come from the owned native observation and are bound to its exact Raw hash.
func wcfDisplayEvent(e *pcaputil.ProtocolEvent) *pcaputil.ProtocolEvent {
	observation, ok := e.NativeWCFObservation()
	if !ok {
		return e
	}
	owned, _ := e.NativeWCFFields()
	projected := *e
	projected.Protocol, projected.Status = "wcf", observation.Status
	projected.Summary, projected.Error = observation.Summary, observation.Error
	projected.Fields = nil
	projected.Session = map[string]any{}
	context := map[string]any{}
	ranges := detailWireRanges(owned["Wire Field Ranges"])
	if !e.NativeWCFWireMatches() {
		ranges = nil
		projected.Raw = nil
		context["Record Bytes"] = "owned Raw changed; original field offsets and body source unavailable"
	}
	for key, value := range owned {
		if key == "Wire Field Ranges" {
			continue
		}
		literal := len(ranges[key]) == 2
		switch key {
		case "Record Name", "Record Length", "Envelope Kind", "Envelope Sequence", "Payload Length", "Envelope Body SHA256", "Envelope Body Opaque", "Chunk Count", "Chunks", "Terminal", "Upgrade Accepted", "Upgrade Protocol Known", "Protocol Transition":
			literal = true // record-derived annotations; they acquire no span.
		case "Declared Length":
			literal = len(ranges["Envelope Length"]) == 2
		case "Mode Name":
			literal = len(ranges["Mode"]) == 2
		case "Encoding Name":
			literal = len(ranges["Encoding"]) == 2 || len(ranges["Encoding Type"]) == 2
		}
		if observation.Status != "decoded" && observation.Status != "deferred" {
			literal = false // A retained context flag is not a complete record field.
		}
		if literal {
			projected.Session[key] = value
		} else {
			context[key] = value
		}
	}
	if len(ranges) > 0 {
		if span := ranges["Envelope Length"]; len(span) == 2 {
			ranges["Declared Length"] = span
		}
		projected.Session["Wire Field Ranges"] = ranges
	}
	if observation.Status == "decoded" && e.NativeWCFWireMatches() {
		var body []byte
		switch number(owned["Record Type"]) {
		case 6:
			if span := ranges["Envelope Body"]; len(span) == 2 && span[0] >= 0 && span[1] > span[0] && span[1] <= len(e.Raw) {
				body = e.Raw[span[0]:span[1]]
				ranges["Envelope Payload (opaque)"] = span
			}
		case 5:
			// Chunk headers and the zero terminator are framing, not SOAP.
			// Their removal creates an extracted source, never a frame span.
			rows := mapList(owned["Chunks"])
			valid, previousEnd := len(rows) > 0, uint64(1)
			for _, chunk := range rows {
				at, size := number(chunk["Body Offset"]), number(chunk["Body Length"])
				if size == 0 || at <= previousEnd || at > uint64(len(e.Raw)) || size > uint64(len(e.Raw))-at || size > (1<<20)-uint64(len(body)) {
					valid = false
					break
				}
				body = append(body, e.Raw[int(at):int(at+size)]...)
				previousEnd = at + size
			}
			if !valid || uint64(len(body)) != number(owned["Payload Length"]) {
				body = nil
				projected.Session["Opaque Payload View"] = "unavailable: chunk projection or detail byte budget"
			}
		}
		if len(body) > 0 {
			projected.Session["Envelope Payload (opaque)"] = bytes.Clone(body)
		}
	}
	projected.Session[wcfObservedContext] = context
	return &projected
}
