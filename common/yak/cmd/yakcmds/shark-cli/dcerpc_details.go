package sharkcli

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

var dcerpcDetailPriority = map[string]int{
	"Ordinal": 0, "Context ID": 1, "Proposed Context ID": 1, "Interface": 2,
	"Result": 3, "Ack Result": 3, "Reason": 4, "Ack Reason": 4,
	"Abstract Syntax": 5, "Interface Version Major": 6, "Interface Version Minor": 7,
	"Transfer Syntax": 8, "Transfer Version": 9, "Bind Time Feature Proposal": 10,
	"Message ID": 0, "Complete In PDU": 1, "Header In PDU": 2, "Fragment Count": 3,
	"Stub Bytes": 4, "Stub Decoded": 5, "Stub": 6, "Fragment Sources": 7, "Byte Source": 8,
}

func dcerpcEventSummary(e *pcaputil.ProtocolEvent) (string, bool) {
	if e.Protocol != "dcerpc" || e.Profile == "dcerpc-security-envelope" || e.Error != "" || (e.Status != "decoded" && e.Status != "deferred") {
		return "", false
	}
	v := eventValues(e)
	name := textValue(v["Packet Name"])
	if name == "" {
		return "", false
	}
	parts := []string{name, fmt.Sprintf("call=%d", number(v["Call ID"]))}
	if context, ok := v["Context ID"]; ok {
		parts = append(parts, fmt.Sprintf("context=%d", number(context)))
	}
	if name == "BindNak" {
		if reason, ok := v["Reject Reason"]; ok {
			parts = append(parts, "reject="+dcerpcDetailValue("Reject Reason", reason, fmt.Sprint(reason), 0, false))
		}
	}
	if e.ResponseTo != 0 {
		parts = append(parts, fmt.Sprintf("reply to PDU #%d", e.ResponseTo))
		if request := textValue(v["Matched Request"]); request != "" {
			parts = append(parts, request)
		}
	} else if v["Unmatched"] == true {
		parts = append(parts, "unmatched: "+textValue(v["Association Status"]))
	}
	if message, ok := v["Reassembled Message"].(map[string]any); ok {
		parts = append(parts, fmt.Sprintf("RPC message: %d fragments · %d stub bytes", number(message["Fragment Count"]), number(message["Stub Bytes"])))
	} else if v["Fragment"] == true {
		parts = append(parts, "RPC fragment")
	}
	return strings.Join(parts, " · "), true
}

func dcerpcDetailValue(name string, value any, fallback string, result uint64, resultKnown bool) string {
	switch name {
	case "Reject Reason":
		if code, ok := detailRangeOffset(value); ok {
			// MS-RPCE also uses reason 4 for a protocol/state error. Do not
			// diagnose an unsupported version solely from this ambiguous code.
			if label := map[int]string{0: "reason not specified", 1: "temporary congestion", 2: "local limit exceeded", 4: "protocol error / protocol version not supported", 8: "authentication type not recognized", 9: "invalid checksum"}[code]; label != "" {
				return fmt.Sprintf("%d (%s)", code, label)
			}
		}
	case "Result", "Ack Result":
		if code, ok := detailRangeOffset(value); ok {
			if label := map[int]string{0: "Acceptance", 1: "User rejection", 2: "Provider rejection", 3: "Feature negotiation ACK"}[code]; label != "" {
				return fmt.Sprintf("%d (%s)", code, label)
			}
		}
	case "Reason", "Ack Reason":
		code, ok := detailRangeOffset(value)
		if !ok {
			return fallback
		}
		if resultKnown && result == 3 {
			return fmt.Sprintf("0x%04x (observed feature bits)", code)
		}
		if resultKnown && (result == 1 || result == 2) {
			if label := map[int]string{0: "not specified", 1: "abstract syntax not supported", 2: "transfer syntaxes not supported", 3: "local limit exceeded"}[code]; label != "" {
				return fmt.Sprintf("%d (%s)", code, label)
			}
		}
	case "Abstract Syntax", "Transfer Syntax", "Object UUID":
		if wire, ok := value.([]byte); ok && len(wire) == 16 {
			return fmt.Sprintf("%08x-%04x-%04x-%x-%x · %s", binary.LittleEndian.Uint32(wire[:4]), binary.LittleEndian.Uint16(wire[4:6]), binary.LittleEndian.Uint16(wire[6:8]), wire[8:10], wire[10:16], fallback)
		}
	}
	return fallback
}
