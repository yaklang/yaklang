package sharkcli

import (
	"encoding/binary"
	"fmt"
)

var dcerpcDetailPriority = map[string]int{
	"Ordinal": 0, "Context ID": 1, "Proposed Context ID": 1, "Interface": 2,
	"Result": 3, "Ack Result": 3, "Reason": 4, "Ack Reason": 4,
	"Abstract Syntax": 5, "Interface Version Major": 6, "Interface Version Minor": 7,
	"Transfer Syntax": 8, "Transfer Version": 9, "Bind Time Feature Proposal": 10,
}

func dcerpcDetailValue(name string, value any, fallback string, result uint64, resultKnown bool) string {
	switch name {
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
