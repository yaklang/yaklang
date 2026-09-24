package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const natsControlLineMax = 8 << 10

type natsFrameInfo struct {
	length      int
	operation   string
	role        string
	fields      map[string]any
	payloadSize int
	headerSize  int
}

type binNATS struct {
	clientDir int // -1 until an unambiguous client/server operation is observed
}

// natsOperationAt returns the case-insensitive operation token at the start of
// a control line. A token must end at a space, tab, CR, or end of input.
func natsOperationAt(line []byte) (string, int, bool) {
	if len(line) == 0 {
		return "", 0, false
	}
	end := 0
	for end < len(line) && line[end] != ' ' && line[end] != '\t' && line[end] != '\r' && line[end] != '\n' {
		end++
	}
	if end == 0 {
		return "", 0, false
	}
	op := strings.ToUpper(string(line[:end]))
	switch op {
	case "INFO", "CONNECT", "PING", "PONG", "PUB", "HPUB", "SUB", "UNSUB", "MSG", "HMSG", "+OK", "-ERR":
		return op, end, true
	default:
		return "", 0, false
	}
}

// natsAdmission requires a distinctive, syntactically valid opening command.
// PING/PONG and +OK/-ERR are only meaningful after a NATS session is established:
// alone they also occur in other text protocols and RESP, respectively.
func natsAdmission(w []byte) (ProbeResult, string) {
	if len(w) < 2 {
		return ProbeResult{Verdict: ProbeReject}, ""
	}
	if bytes.IndexByte(w, '\n') >= 0 {
		line, _, err := natsLine(w)
		if err != nil {
			return ProbeResult{Verdict: ProbeReject, Reason: err.Error()}, ""
		}
		op, _, ok := natsOperationAt(line)
		if !ok || !natsInitialOperation(op) {
			return ProbeResult{Verdict: ProbeReject, Reason: "not a distinctive NATS opening command"}, ""
		}
		info, err := natsFrameInfoFor(w, DefaultParserBudget().MaxMessageBytes)
		if err != nil {
			return ProbeResult{Verdict: ProbeReject, Reason: err.Error()}, ""
		}
		if info.length > len(w) {
			return ProbeResult{Verdict: ProbeNeedMore, NeedBytes: info.length - len(w), Reason: "NATS payload is incomplete"}, op
		}
		return probeAccept("nats", "client", 92), op
	}
	if cr := bytes.IndexByte(w, '\r'); cr >= 0 && cr != len(w)-1 {
		return ProbeResult{Verdict: ProbeReject, Reason: "NATS command line contains a bare CR"}, ""
	}
	if len(w) >= natsControlLineMax {
		return ProbeResult{Verdict: ProbeReject, Reason: "NATS command line exceeds limit"}, ""
	}
	if op, end, ok := natsOperationAt(w); ok {
		if !natsInitialOperation(op) || !natsCandidateLine(op, w) {
			return ProbeResult{Verdict: ProbeReject}, ""
		}
		if end < len(w) && op == "CONNECT" && !natsJSONCommandArgument(op, w) {
			return ProbeResult{Verdict: ProbeReject, Reason: "not a NATS JSON command"}, ""
		}
		return ProbeResult{Verdict: ProbeNeedMore, NeedBytes: 1, Reason: "NATS command line is incomplete"}, op
	}
	for _, op := range []string{"INFO", "CONNECT", "PUB", "HPUB", "MSG", "HMSG"} {
		if len(w) < len(op) && bytes.Equal(w, []byte(op[:len(w)])) {
			return ProbeResult{Verdict: ProbeNeedMore, NeedBytes: 1, Reason: "NATS operation is incomplete"}, op
		}
	}
	return ProbeResult{Verdict: ProbeReject, Reason: "not a NATS client-protocol prefix"}, ""
}

func natsInitialOperation(op string) bool {
	switch op {
	case "INFO", "CONNECT", "PUB", "HPUB", "MSG", "HMSG":
		return true
	}
	return false
}

// natsJSONCommandArgument is an admission discriminator, not a JSON validator:
// the pcapx framer later rejects malformed JSON objects. It keeps HTTP CONNECT
// authority-form requests out of the NATS path while accepting split JSON.
func natsJSONCommandArgument(op string, line []byte) bool {
	if op != "CONNECT" {
		return true
	}
	_, end, ok := natsOperationAt(line)
	if !ok || end == len(line) || line[end] != ' ' && line[end] != '\t' {
		return false
	}
	arg := bytes.TrimLeft(line[end:], " \t")
	return len(arg) > 0 && arg[0] == '{'
}

func natsNeedsMore(w []byte) bool {
	result, _ := natsAdmission(w)
	return result.Verdict == ProbeNeedMore
}

func natsCandidateLine(op string, line []byte) bool {
	_, end, ok := natsOperationAt(line)
	if !ok {
		return false
	}
	if end == len(line) {
		return true
	}
	return line[end] == ' ' || line[end] == '\t'
}

func natsRole(op string) string {
	switch op {
	case "INFO", "MSG", "HMSG", "+OK", "-ERR":
		return "server"
	case "CONNECT", "PUB", "HPUB", "SUB", "UNSUB":
		return "client"
	default:
		return ""
	}
}

func natsFields(line []byte) [][]byte {
	fields := make([][]byte, 0, 5)
	for i := 0; i < len(line); {
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i == len(line) {
			break
		}
		start := i
		for i < len(line) && line[i] != ' ' && line[i] != '\t' {
			i++
		}
		fields = append(fields, line[start:i])
	}
	return fields
}

func natsLine(w []byte) ([]byte, int, error) {
	lf := bytes.IndexByte(w, '\n')
	if lf < 0 {
		if len(w) > natsControlLineMax || len(w) == natsControlLineMax && w[len(w)-1] != '\r' {
			return nil, 0, protocolError(ErrResourceExceeded, "NATS control line exceeds limit")
		}
		if cr := bytes.IndexByte(w, '\r'); cr >= 0 && cr != len(w)-1 {
			return nil, 0, protocolError(ErrMalformedMessage, "NATS command line contains a bare CR")
		}
		return nil, 0, nil
	}
	if lf == 0 || w[lf-1] != '\r' {
		return nil, 0, protocolError(ErrMalformedMessage, "NATS command line must end in CRLF")
	}
	if lf-1 > natsControlLineMax {
		return nil, 0, protocolError(ErrResourceExceeded, "NATS control line exceeds limit")
	}
	line := w[:lf-1]
	for _, b := range line {
		if b == '\r' || b == 0 || b < 0x20 && b != '\t' || b == 0x7f {
			return nil, 0, protocolError(ErrMalformedMessage, "NATS command line contains a control byte")
		}
	}
	return line, lf + 1, nil
}

func natsParseSize(text []byte) (int, error) {
	if len(text) == 0 {
		return 0, protocolError(ErrMalformedMessage, "NATS payload length is empty")
	}
	for _, b := range text {
		if b < '0' || b > '9' {
			return 0, protocolError(ErrMalformedMessage, "NATS payload length is not an unsigned decimal")
		}
	}
	n, err := strconv.ParseUint(string(text), 10, 64)
	if err != nil {
		return 0, protocolError(ErrResourceExceeded, "NATS payload length exceeds addressable memory")
	}
	if n > uint64(maxInt()) {
		return 0, protocolError(ErrResourceExceeded, "NATS payload length exceeds addressable memory")
	}
	return int(n), nil
}

func natsJSONArgument(line []byte, op string, tokenEnd int) (map[string]any, error) {
	if tokenEnd == len(line) {
		return nil, protocolError(ErrMalformedMessage, "NATS %s requires a JSON object", op)
	}
	if line[tokenEnd] != ' ' && line[tokenEnd] != '\t' {
		return nil, protocolError(ErrMalformedMessage, "NATS %s operation is not separated from its JSON object", op)
	}
	argument := bytes.TrimLeft(line[tokenEnd:], " \t")
	if len(argument) == 0 || argument[0] != '{' {
		return nil, protocolError(ErrMalformedMessage, "NATS %s argument is not a JSON object", op)
	}
	var object map[string]any
	if err := json.Unmarshal(argument, &object); err != nil || object == nil {
		return nil, protocolError(ErrMalformedMessage, "NATS %s JSON object is malformed", op)
	}
	return object, nil
}

func natsValidateHeader(header []byte) error {
	const prefix = "NATS/1.0"
	if !bytes.HasPrefix(header, []byte(prefix)) || len(header) <= len(prefix) || header[len(prefix)] != '\r' && header[len(prefix)] != ' ' {
		return protocolError(ErrMalformedMessage, "NATS header section has no NATS/1.0 version line")
	}
	if !bytes.HasSuffix(header, []byte("\r\n\r\n")) {
		return protocolError(ErrMalformedMessage, "NATS header section has no terminating empty line")
	}
	if bytes.IndexByte(header, 0) >= 0 {
		return protocolError(ErrMalformedMessage, "NATS header section contains NUL")
	}
	return nil
}

// natsFrameInfo validates one complete command header and calculates its exact
// message boundary. A short line or body returns length zero without error so
// TCP callers retain it for later input. maxBytes includes the control line,
// payload, and terminal CRLF.
func natsFrameInfoFor(w []byte, maxBytes int) (natsFrameInfo, error) {
	line, lineBytes, err := natsLine(w)
	if err != nil || lineBytes == 0 {
		return natsFrameInfo{}, err
	}
	op, tokenEnd, ok := natsOperationAt(line)
	if !ok || !natsCandidateLine(op, line) {
		return natsFrameInfo{}, protocolError(ErrMalformedMessage, "unknown NATS client-protocol operation")
	}
	all := natsFields(line)
	args := all[1:]
	info := natsFrameInfo{operation: op, role: natsRole(op), length: lineBytes, fields: map[string]any{"Operation": op}}
	set := func(k string, v any) { info.fields[k] = v }
	subject := func(b []byte, name string) error {
		if len(b) == 0 {
			return protocolError(ErrMalformedMessage, "NATS %s is empty", name)
		}
		for _, ch := range b {
			if ch <= 0x20 || ch == 0x7f {
				return protocolError(ErrMalformedMessage, "NATS %s contains whitespace or a control byte", name)
			}
		}
		return nil
	}
	uintField := func(b []byte, name string) (uint64, error) {
		if len(b) == 0 {
			return 0, protocolError(ErrMalformedMessage, "NATS %s is empty", name)
		}
		for _, ch := range b {
			if ch < '0' || ch > '9' {
				return 0, protocolError(ErrMalformedMessage, "NATS %s is not an unsigned decimal", name)
			}
		}
		n, err := strconv.ParseUint(string(b), 10, 64)
		if err != nil {
			return 0, protocolError(ErrMalformedMessage, "NATS %s is out of range", name)
		}
		return n, nil
	}
	payload := func(size uint64) error {
		if size > uint64(maxBytes) {
			return protocolError(ErrResourceExceeded, "NATS message payload exceeds configured byte limit")
		}
		if size > uint64(maxInt())-uint64(lineBytes)-2 {
			return protocolError(ErrResourceExceeded, "NATS message length overflows")
		}
		total := lineBytes + int(size) + 2
		if total > maxBytes {
			return protocolError(ErrResourceExceeded, "NATS framed message exceeds configured byte limit")
		}
		info.length, info.payloadSize = total, int(size)
		if len(w) < total {
			return nil
		}
		if !bytes.Equal(w[total-2:total], []byte("\r\n")) {
			return protocolError(ErrMalformedMessage, "NATS payload is not followed by CRLF")
		}
		return nil
	}
	if op == "CONNECT" || op == "INFO" {
		object, err := natsJSONArgument(line, op, tokenEnd)
		if err != nil {
			return natsFrameInfo{}, err
		}
		set("JSON Field Count", len(object))
		if op == "INFO" {
			set("Server Info Observed", true)
		} else {
			set("Connect Options Observed", true)
		}
		return info, nil
	}
	switch op {
	case "PING", "PONG", "+OK":
		if len(args) != 0 {
			return natsFrameInfo{}, protocolError(ErrMalformedMessage, "NATS %s does not take arguments", op)
		}
	case "-ERR":
		if tokenEnd < len(line) {
			set("Error Text", strings.TrimSpace(string(line[tokenEnd:])))
		}
	case "PUB":
		if len(args) != 2 && len(args) != 3 {
			return natsFrameInfo{}, protocolError(ErrMalformedMessage, "NATS PUB requires subject, optional reply subject, and payload length")
		}
		if err := subject(args[0], "subject"); err != nil {
			return natsFrameInfo{}, err
		}
		lengthIndex := len(args) - 1
		if lengthIndex == 2 {
			if err := subject(args[1], "reply subject"); err != nil {
				return natsFrameInfo{}, err
			}
			set("Reply Subject", string(args[1]))
		}
		n, err := natsParseSize(args[lengthIndex])
		if err != nil {
			return natsFrameInfo{}, err
		}
		set("Subject", string(args[0]))
		set("Payload Length", n)
		if err := payload(uint64(n)); err != nil {
			return natsFrameInfo{}, err
		}
	case "HPUB":
		if len(args) != 3 && len(args) != 4 {
			return natsFrameInfo{}, protocolError(ErrMalformedMessage, "NATS HPUB requires subject, optional reply subject, header length, and total length")
		}
		if err := subject(args[0], "subject"); err != nil {
			return natsFrameInfo{}, err
		}
		at := 1
		if len(args) == 4 {
			if err := subject(args[1], "reply subject"); err != nil {
				return natsFrameInfo{}, err
			}
			set("Reply Subject", string(args[1]))
			at++
		}
		hdr, err := natsParseSize(args[at])
		if err != nil {
			return natsFrameInfo{}, err
		}
		total, err := natsParseSize(args[at+1])
		if err != nil {
			return natsFrameInfo{}, err
		}
		if hdr > total {
			return natsFrameInfo{}, protocolError(ErrMalformedMessage, "NATS HPUB header length exceeds total payload length")
		}
		set("Subject", string(args[0]))
		set("Header Length", hdr)
		set("Total Payload Length", total)
		info.headerSize = hdr
		if err := payload(uint64(total)); err != nil {
			return natsFrameInfo{}, err
		}
		if len(w) >= info.length && hdr > 0 {
			if err := natsValidateHeader(w[lineBytes : lineBytes+hdr]); err != nil {
				return natsFrameInfo{}, err
			}
		} else if len(w) >= info.length && hdr == 0 {
			return natsFrameInfo{}, protocolError(ErrMalformedMessage, "NATS HPUB header section is empty")
		}
	case "SUB":
		if len(args) != 2 && len(args) != 3 {
			return natsFrameInfo{}, protocolError(ErrMalformedMessage, "NATS SUB requires subject, optional queue group, and subscription id")
		}
		if err := subject(args[0], "subject"); err != nil {
			return natsFrameInfo{}, err
		}
		sidAt := 1
		set("Subject", string(args[0]))
		if len(args) == 3 {
			if err := subject(args[1], "queue group"); err != nil {
				return natsFrameInfo{}, err
			}
			set("Queue Group", string(args[1]))
			sidAt++
		}
		if err := subject(args[sidAt], "subscription id"); err != nil {
			return natsFrameInfo{}, err
		}
		set("Subscription ID", string(args[sidAt]))
	case "UNSUB":
		if len(args) != 1 && len(args) != 2 {
			return natsFrameInfo{}, protocolError(ErrMalformedMessage, "NATS UNSUB requires subscription id and optional message count")
		}
		if err := subject(args[0], "subscription id"); err != nil {
			return natsFrameInfo{}, err
		}
		set("Subscription ID", string(args[0]))
		if len(args) == 2 {
			n, err := uintField(args[1], "maximum message count")
			if err != nil {
				return natsFrameInfo{}, err
			}
			set("Maximum Message Count", n)
		}
	case "MSG", "HMSG":
		header := op == "HMSG"
		minArgs, maxArgs := 3, 4
		if header {
			minArgs, maxArgs = 4, 5
		}
		if len(args) < minArgs || len(args) > maxArgs {
			return natsFrameInfo{}, protocolError(ErrMalformedMessage, "NATS %s requires subject, subscription id, optional reply subject, and message length", op)
		}
		if err := subject(args[0], "subject"); err != nil {
			return natsFrameInfo{}, err
		}
		if err := subject(args[1], "subscription id"); err != nil {
			return natsFrameInfo{}, err
		}
		at := 2
		if len(args) == maxArgs {
			if err := subject(args[2], "reply subject"); err != nil {
				return natsFrameInfo{}, err
			}
			set("Reply Subject", string(args[2]))
			at++
		}
		set("Subject", string(args[0]))
		set("Subscription ID", string(args[1]))
		if header {
			hdr, err := natsParseSize(args[at])
			if err != nil {
				return natsFrameInfo{}, err
			}
			total, err := natsParseSize(args[at+1])
			if err != nil {
				return natsFrameInfo{}, err
			}
			if hdr > total {
				return natsFrameInfo{}, protocolError(ErrMalformedMessage, "NATS HMSG header length exceeds total payload length")
			}
			set("Header Length", hdr)
			set("Total Payload Length", total)
			info.headerSize = hdr
			if err := payload(uint64(total)); err != nil {
				return natsFrameInfo{}, err
			}
			if len(w) >= info.length && hdr > 0 {
				if err := natsValidateHeader(w[lineBytes : lineBytes+hdr]); err != nil {
					return natsFrameInfo{}, err
				}
			} else if len(w) >= info.length && hdr == 0 {
				return natsFrameInfo{}, protocolError(ErrMalformedMessage, "NATS HMSG header section is empty")
			}
		} else {
			n, err := natsParseSize(args[at])
			if err != nil {
				return natsFrameInfo{}, err
			}
			set("Payload Length", n)
			if err := payload(uint64(n)); err != nil {
				return natsFrameInfo{}, err
			}
		}
	}
	return info, nil
}

func maxInt() int { return int(^uint(0) >> 1) }

func (f *binNATS) consume(dir int, raw []byte, maxBytes int) (map[string]any, error) {
	info, err := natsFrameInfoFor(raw, maxBytes)
	if err != nil {
		return nil, err
	}
	if info.length != len(raw) {
		return nil, protocolError(ErrMalformedMessage, "NATS message boundary does not match the framed bytes")
	}
	if info.role != "" {
		if f.clientDir < 0 {
			if info.role == "client" {
				f.clientDir = dir
			} else {
				f.clientDir = 1 - dir
			}
		} else {
			want := f.clientDir
			if info.role == "server" {
				want = 1 - want
			}
			if dir != want {
				return nil, protocolError(ErrMalformedMessage, "NATS %s arrived in the wrong direction; expected %s traffic", info.operation, info.role)
			}
		}
	}
	role := "unknown"
	if f.clientDir >= 0 {
		if dir == f.clientDir {
			role = "client"
		} else {
			role = "server"
		}
	}
	info.fields["Direction Role"] = role
	info.fields["Plaintext Client Protocol"] = true
	info.fields["TCP Reassembly Performed"] = false
	info.fields["Payload Decoded"] = false
	if info.operation == "HPUB" || info.operation == "HMSG" {
		info.fields["Header Payload Size"] = info.headerSize
	}
	return info.fields, nil
}

func (f *binFlow) frameNATS(w []byte) (int, *binSpec, error) {
	info, err := natsFrameInfoFor(w, f.a.config.MaxMessageBytes)
	if err != nil || info.length == 0 {
		return info.length, nil, err
	}
	entry := "NATSControlLine"
	switch info.operation {
	case "PUB", "HPUB", "MSG", "HMSG":
		entry = "NATSPayloadFrame"
	}
	return info.length, f.spec("nats", entry), nil
}

func parseNATSFrame(w []byte, maxBytes int) (natsFrameInfo, error) {
	if maxBytes <= 0 {
		return natsFrameInfo{}, fmt.Errorf("NATS frame limit must be positive")
	}
	return natsFrameInfoFor(w, maxBytes)
}
