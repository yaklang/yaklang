package stream_parser

import (
	"bytes"
	"fmt"
)

const (
	smtpReplyMaxLines = 1024
	smtpReplyMaxBytes = smtpReplyMaxLines * 512
)

type smtpReplyLine struct {
	Start, End, TextStart, TextEnd int
	Separator                      byte // Zero means the code-only final form.
}

// decodeSMTPReply implements the RFC 5321 receiver-compatible Reply-line layout,
// including the section 4.2 allowance for an empty space-terminated final text.
// It does not implement the Greeting
// or command-specific grammar, session state, TLS or message-delivery semantics.
// One caller-bounded reply is required; every offset refers to original bytes.
func decodeSMTPReply(wire []byte) ([]smtpReplyLine, map[string]any, error) {
	fail := func(s string) ([]smtpReplyLine, map[string]any, error) {
		return nil, nil, fmt.Errorf("smtp-reply: %s", s)
	}
	if len(wire) < 5 || len(wire) > smtpReplyMaxBytes {
		return fail("complete reply size outside 5..524288 byte profile")
	}
	var lines []smtpReplyLine
	for at := 0; at < len(wire); {
		if len(lines) == smtpReplyMaxLines {
			return fail("reply exceeds 1024 line profile")
		}
		end := bytes.Index(wire[at:], []byte("\r\n"))
		if end < 0 {
			return fail("reply line lacks CRLF")
		}
		end += at
		if end-at < 3 || end-at+2 > 512 {
			return fail("reply line size outside 5..512 octets")
		}
		if wire[at] < '2' || wire[at] > '5' || wire[at+1] < '0' || wire[at+1] > '5' || wire[at+2] < '0' || wire[at+2] > '9' {
			return fail("reply code requires [2-5][0-5][0-9]")
		}
		if !bytes.Equal(wire[at:at+3], wire[:3]) {
			return fail("multiline reply codes differ")
		}
		line := smtpReplyLine{Start: at, End: end + 2, TextStart: at + 3, TextEnd: end}
		if end > at+3 {
			line.Separator = wire[at+3]
			if line.Separator != ' ' && line.Separator != '-' {
				return fail("reply code separator must be space or hyphen")
			}
			line.TextStart++
		}
		for _, b := range wire[line.TextStart:line.TextEnd] {
			if b != '\t' && (b < 32 || b > 126) {
				return fail("reply text requires printable ASCII or HT")
			}
		}
		lines = append(lines, line)
		if line.Separator != '-' {
			if line.End != len(wire) {
				return fail("bytes follow terminal reply line")
			}
			code := int(wire[0]-'0')*100 + int(wire[1]-'0')*10 + int(wire[2]-'0')
			return lines, map[string]any{
				"Profile": "Explicit complete RFC5321 ASCII receiver-compatible reply layout", "Reply Code": code,
				"Reply Category": int(wire[0] - '0'), "Reply Subject": int(wire[1] - '0'), "Line Count": len(lines),
				"Multiline": len(lines) > 1, "Code Only Final Line": line.Separator == 0,
				"Receiver Empty Final Text": line.Separator == ' ' && line.TextStart == line.TextEnd, "Sender Conformance Validated": false,
				"Line Length Validated": true, "Connection State Validated": false, "TLS Handshake Validated": false,
				"Endpoint Identity Proven": false, "Command Semantics Decoded": false, "Greeting Grammar Validated": false,
				"TCP Reassembly Performed": false, "Message Delivery Proven": false,
			}, nil
		}
		at = line.End
	}
	return fail("multiline reply lacks terminal line")
}
