package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const natsControlLineMax = 8 << 10

type natsFrameInfo struct {
	length        int
	operation     string
	role          string
	fields        map[string]any
	payloadSize   int
	headerSize    int
	headersOption *bool
}

type binNATS struct {
	clientDir                         int // -1 until an unambiguous client/server operation is observed
	serverHeadersKnown, serverHeaders bool
	clientHeadersKnown, clientHeaders bool
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
	return natsAdmissionWithBudget(w, DefaultParserBudget())
}

func natsAdmissionWithBudget(w []byte, budget ParserBudget) (ProbeResult, string) {
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
		info, err := natsFrameInfoWithBudget(w, budget)
		if err != nil {
			return ProbeResult{Verdict: ProbeReject, Reason: err.Error()}, ""
		}
		if info.length > len(w) {
			return ProbeResult{Verdict: ProbeNeedMore, Protocol: "nats", NeedBytes: info.length - len(w), Reason: "NATS payload is incomplete"}, op
		}
		return probeAccept("nats", "client", 92), op
	}
	if _, _, err := natsLine(w); err != nil {
		return ProbeResult{Verdict: ProbeReject, Reason: err.Error()}, ""
	}
	if len(w) >= budget.MaxMessageBytes {
		return ProbeResult{Verdict: ProbeReject, Reason: "NATS command line exceeds limit"}, ""
	}
	if op, end, ok := natsOperationAt(w); ok {
		if !natsInitialOperation(op) || !natsCandidateLine(op, w) {
			return ProbeResult{Verdict: ProbeReject}, ""
		}
		if end < len(w) && op == "CONNECT" && !natsJSONCommandArgument(op, w) {
			return ProbeResult{Verdict: ProbeReject, Reason: "not a NATS JSON command"}, ""
		}
		return ProbeResult{Verdict: ProbeNeedMore, Protocol: "nats", NeedBytes: 1, Reason: "NATS command line is incomplete"}, op
	}
	for _, op := range []string{"INFO", "CONNECT", "PUB", "HPUB", "MSG", "HMSG"} {
		if len(w) < len(op) && bytes.Equal(w, []byte(op[:len(w)])) {
			return ProbeResult{Verdict: ProbeNeedMore, Protocol: "nats", NeedBytes: 1, Reason: "NATS operation is incomplete"}, op
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
		if len(w) > natsControlLineMax+1 || len(w) == natsControlLineMax+1 && w[len(w)-1] != '\r' {
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

// natsJSONArgument validates JSON without constructing its recursive value tree.
// Each collection and nesting level is charged before reading its children.
func natsJSONArgument(line []byte, op string, tokenEnd int, budget ParserBudget, info *natsFrameInfo) (int, error) {
	malformed := func() error { return protocolError(ErrMalformedMessage, "NATS %s JSON object is malformed", op) }
	if tokenEnd == len(line) || line[tokenEnd] != ' ' && line[tokenEnd] != '\t' {
		return 0, malformed()
	}
	argument := bytes.TrimLeft(line[tokenEnd:], " \t")
	if len(argument) == 0 || argument[0] != '{' {
		return 0, malformed()
	}
	decoder := json.NewDecoder(bytes.NewReader(argument))
	decoder.UseNumber()
	rootKeys := make(map[string]struct{})
	var value func(int) error
	value = func(depth int) error {
		token, err := decoder.Token()
		if err != nil {
			return malformed()
		}
		delimiter, collection := token.(json.Delim)
		if !collection {
			return nil
		}
		if delimiter != '{' && delimiter != '[' {
			return malformed()
		}
		if depth > budget.MaxRecursionDepth {
			return protocolError(ErrResourceExceeded, "NATS JSON nesting exceeds configured depth limit")
		}
		count := 0
		for decoder.More() {
			count++
			if count > budget.MaxCollectionElements {
				return protocolError(ErrResourceExceeded, "NATS JSON collection exceeds configured element limit")
			}
			if delimiter == '{' {
				key, err := decoder.Token()
				if err != nil {
					return malformed()
				}
				name, ok := key.(string)
				if !ok {
					return malformed()
				}
				if depth == 1 {
					rootKeys[name] = struct{}{}
					if name == "headers" {
						option, err := decoder.Token()
						enabled, ok := option.(bool)
						if err != nil || !ok {
							return malformed()
						}
						info.headersOption = &enabled
						continue
					}
				}
			}
			if err := value(depth + 1); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || delimiter == '{' && closing != json.Delim('}') || delimiter == '[' && closing != json.Delim(']') {
			return malformed()
		}
		return nil
	}
	if err := value(1); err != nil {
		return 0, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return 0, malformed()
	}
	return len(rootKeys), nil
}

// natsDecodeHeader follows NATS ADR-4, not HTTP's case-insensitive header map.
// Values are ASCII other than CR/LF (including NUL); each physical field or
// continuation line consumes one collection element before it is retained.
func natsDecodeHeader(header []byte, budget ParserBudget, fields map[string]any) error {
	malformed := func(reason string) error { return protocolError(ErrMalformedMessage, "NATS header %s", reason) }
	readLine := func() ([]byte, error) {
		at := bytes.Index(header, []byte("\r\n"))
		if at < 0 {
			return nil, malformed("line must end in CRLF")
		}
		line := header[:at]
		if bytes.ContainsAny(line, "\r\n") {
			return nil, malformed("line contains a bare CR or LF")
		}
		header = header[at+2:]
		return line, nil
	}
	line, err := readLine()
	if err != nil {
		return err
	}
	version, statusLine, _ := strings.Cut(string(line), " ")
	if !strings.HasPrefix(version, "NATS/") {
		return malformed("section has no NATS version line")
	}
	major, minor, found := strings.Cut(strings.TrimPrefix(version, "NATS/"), ".")
	digits := func(s string) bool {
		if s == "" {
			return false
		}
		for i := range s {
			if s[i] < '0' || s[i] > '9' {
				return false
			}
		}
		return true
	}
	if !found || !digits(major) || !digits(minor) {
		return malformed("version token is malformed")
	}
	if version != "NATS/1.0" {
		return protocolError(ErrUnsupportedVersion, "NATS header version %s is unsupported", version)
	}
	headers := make(map[string]any)
	status, description := "", ""
	if len(line) > len(version) {
		statusLine = strings.Trim(statusLine, " \t")
		if len(statusLine) < 3 || !digits(statusLine[:3]) || len(statusLine) > 3 && statusLine[3] != ' ' && statusLine[3] != '\t' {
			return malformed("status must contain a three-digit code and optional description")
		}
		status = statusLine[:3]
		description = strings.Trim(statusLine[3:], " \t")
		for i := range description {
			if description[i] > 127 || description[i] < 32 && description[i] != '\t' {
				return malformed("status description contains a non-text byte")
			}
		}
	}
	count, lines := 0, 0
	previous := ""
	var current strings.Builder
	flush := func() {
		if previous != "" {
			values, _ := headers[previous].([]string)
			headers[previous] = append(values, current.String())
			current.Reset()
		}
	}
	for {
		line, err := readLine()
		if err != nil {
			return err
		}
		if len(line) == 0 {
			if len(header) != 0 {
				return malformed("length includes bytes after the terminating empty line")
			}
			break
		}
		lines++
		if lines > budget.MaxCollectionElements {
			return protocolError(ErrResourceExceeded, "NATS header lines exceed configured element limit")
		}
		for _, c := range line {
			if c > 127 {
				return malformed("field contains a non-ASCII byte")
			}
		}
		if line[0] == ' ' || line[0] == '\t' {
			// ADR-4 cites RFC 822 unfolding. Preserve case and repeated values,
			// and unfold only the most recently observed value.
			if previous == "" {
				return malformed("continuation has no preceding field")
			}
			current.WriteByte(' ')
			current.WriteString(strings.TrimLeft(string(line), " \t"))
			continue
		}
		colon := bytes.IndexByte(line, ':')
		if colon <= 0 {
			return malformed("field has no name/colon delimiter")
		}
		for _, c := range line[:colon] {
			if c < 33 || c > 126 {
				return malformed("field name contains whitespace or a non-printable byte")
			}
		}
		flush()
		previous = string(line[:colon])
		current.WriteString(strings.TrimLeft(string(line[colon+1:]), " \t"))
		count++
	}
	flush()
	// Match the official clients' representation of inline status without
	// overwriting application fields with the same case-sensitive names.
	if status != "" {
		values, _ := headers["Status"].([]string)
		headers["Status"] = append(values, status)
		fields["Header Status"] = status
	}
	if description != "" {
		values, _ := headers["Description"].([]string)
		headers["Description"] = append(values, description)
		fields["Header Description"] = description
	}
	fields["Headers"] = headers
	fields["Header Version"] = version
	fields["Headers Decoded"] = true
	fields["Header Field Count"] = count
	return nil
}

// natsFrameInfo validates one complete command header and calculates its exact
// message boundary. A short line or body returns length zero without error so
// TCP callers retain it for later input. maxBytes includes the control line,
// payload, and terminal CRLF.
func natsFrameInfoFor(w []byte, maxBytes int) (natsFrameInfo, error) {
	budget := DefaultParserBudget()
	budget.MaxMessageBytes = maxBytes
	return natsFrameInfoWithBudget(w, budget)
}

func natsFrameInfoWithBudget(w []byte, budget ParserBudget) (natsFrameInfo, error) {
	maxBytes := budget.MaxMessageBytes
	line, lineBytes, err := natsLine(w)
	if err != nil || lineBytes == 0 {
		return natsFrameInfo{}, err
	}
	if lineBytes > maxBytes {
		return natsFrameInfo{}, protocolError(ErrResourceExceeded, "NATS control line exceeds configured byte limit")
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
		fieldCount, err := natsJSONArgument(line, op, tokenEnd, budget, &info)
		if err != nil {
			return natsFrameInfo{}, err
		}
		set("JSON Field Count", fieldCount)
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
		if len(w) >= lineBytes+hdr && hdr > 0 {
			if err := natsDecodeHeader(w[lineBytes:lineBytes+hdr], budget, info.fields); err != nil {
				return natsFrameInfo{}, err
			}
		} else if hdr == 0 {
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
			if len(w) >= lineBytes+hdr && hdr > 0 {
				if err := natsDecodeHeader(w[lineBytes:lineBytes+hdr], budget, info.fields); err != nil {
					return natsFrameInfo{}, err
				}
			} else if hdr == 0 {
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

func (f *binNATS) consume(dir int, raw []byte, budget ParserBudget) (map[string]any, error) {
	info, err := natsFrameInfoWithBudget(raw, budget)
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
	switch info.operation {
	case "INFO":
		// Asynchronous INFO updates may omit headers; omission cannot revoke
		// an explicitly observed server capability.
		if info.headersOption != nil {
			f.serverHeadersKnown, f.serverHeaders = true, *info.headersOption
		}
	case "CONNECT":
		f.clientHeadersKnown, f.clientHeaders = true, info.headersOption != nil && *info.headersOption
	}
	negotiation := "unknown"
	if f.serverHeadersKnown && !f.serverHeaders || f.clientHeadersKnown && !f.clientHeaders {
		negotiation = "disabled"
	} else if f.serverHeadersKnown && f.clientHeadersKnown {
		negotiation = "enabled"
	}
	if (info.operation == "HPUB" || info.operation == "HMSG") && negotiation == "disabled" {
		return nil, protocolError(ErrUnsupportedFeature, "NATS header message was observed without mutually enabled header support")
	}
	info.fields["Header Negotiation"] = negotiation
	if f.serverHeadersKnown {
		info.fields["Server Headers Supported"] = f.serverHeaders
	}
	if f.clientHeadersKnown {
		info.fields["Client Headers Enabled"] = f.clientHeaders
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
		info.fields["Body Length"] = info.payloadSize - info.headerSize
	}
	return info.fields, nil
}

// Header negotiation retains only a fixed-size capability snapshot, never a
// subscription table or a previous message's header strings.
func (f *binFlow) consumeNATS(e *ProtocolEvent, dir int) error {
	if f.nats == nil {
		if err := f.reserveSession(64); err != nil {
			return err
		}
		f.nats = &binNATS{clientDir: -1}
	}
	var err error
	e.Session, err = f.nats.consume(dir, e.Raw, f.a.budget)
	return err
}

func (f *binFlow) frameNATS(w []byte) (int, *binSpec, error) {
	info, err := natsFrameInfoWithBudget(w, f.a.budget)
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
