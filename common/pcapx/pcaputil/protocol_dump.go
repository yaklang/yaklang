package pcaputil

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DumpLine returns one log line without decoding fields or changing the event.
// It includes message identity, protocol/status, endpoints, direction, stream
// offset, byte length and available transaction/diagnostic context. Strings are
// quoted so captured newlines, tabs and terminal escapes cannot split the line.
// TCP A/B are the first observed endpoints, not inferred client/server roles;
// UDP direction is represented by its source and destination.
//
// Example:
//
//	pcapx.StartSniff("en1", pcapx.pcap_onProtocolMessage(func(message) {
//	    println(message.DumpLine())
//	}))~
func (e *ProtocolEvent) DumpLine() string {
	if e == nil {
		return "<nil ProtocolEvent>"
	}
	var line strings.Builder
	stamp := "-"
	if !e.Timestamp.IsZero() {
		stamp = e.Timestamp.Format(time.RFC3339Nano)
	}
	protocol := e.Protocol
	if protocol == "" {
		protocol = "unknown"
	}
	direction := "source->destination"
	if e.Transport == "tcp" {
		switch e.Direction {
		case 0:
			direction = "A->B"
		case 1:
			direction = "B->A"
		default:
			direction = "unknown"
		}
	}
	fmt.Fprintf(&line, "time=%s id=%d flow=%d proto=%s transport=%s status=%s direction=%s src=%s dst=%s offset=%d bytes=%d",
		stamp, e.ID, e.FlowID, strconv.Quote(protocol), strconv.Quote(e.Transport),
		strconv.Quote(e.Status), strconv.Quote(direction), strconv.Quote(e.Source),
		strconv.Quote(e.Destination), e.Offset, e.Length)
	add := func(key, value string) {
		if value != "" {
			line.WriteByte(' ')
			line.WriteString(key)
			line.WriteByte('=')
			line.WriteString(strconv.Quote(value))
		}
	}
	add("profile", e.Profile)
	add("completeness", e.Completeness)
	add("byte_source", e.SourceBytes.Kind)
	for _, key := range []string{"Packet Name", "Opcode Name", "Method", "Frame Type", "Record Type", "Operation", "Type"} {
		if value, ok := e.Session[key]; ok && value != nil {
			add("op", fmt.Sprint(value))
			break
		}
	}
	if e.TransactionID != 0 {
		fmt.Fprintf(&line, " transaction=%d", e.TransactionID)
	}
	if e.ResponseTo != 0 {
		fmt.Fprintf(&line, " response_to=%d", e.ResponseTo)
	}
	if retained, ok := e.Session["Payload Retained"].(bool); ok {
		fmt.Fprintf(&line, " payload_retained=%t", retained)
		if declared, ok := e.Session["Declared Message Bytes"].(int64); ok && declared >= 0 {
			fmt.Fprintf(&line, " declared_bytes=%d", declared)
		}
	}
	add("code", e.ExpertCode)
	add("summary", e.Summary)
	add("error", e.Error)
	return line.String()
}
