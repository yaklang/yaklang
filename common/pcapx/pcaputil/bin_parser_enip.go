package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	enipHeaderSize          = 24
	enipCommandRegister     = 0x0065
	enipCommandSendRRData   = 0x006f
	enipCPFNullAddress      = 0x0000
	enipCPFUnconnectedData  = 0x00b2
	enipCIPReadTag          = 0x4c
	enipCIPReadTagReply     = 0xcc
	enipCIPWriteTag         = 0x4d
	enipCIPWriteTagReply    = 0xcd
	enipCIPTypeDINT         = 0x00c4
	enipCIPANSIExtendedPath = 0x91
)

type enipHeader struct {
	command uint16
	length  uint16
	session uint32
	status  uint32
	context [8]byte
	options uint32
}

type enipPending struct {
	service byte
	tag     string
	value   int32
}

type enipPendingKey struct {
	dir     int
	service byte
	tag     string
}

type binENIP struct {
	clientDir           int
	registerRequestSeen bool
	registered          bool
	session             uint32
	pending             map[enipPendingKey]enipPending
	maxPending          int
}

func parseENIPHeader(w []byte) (enipHeader, bool) {
	if len(w) < enipHeaderSize {
		return enipHeader{}, false
	}
	var h enipHeader
	h.command = binary.LittleEndian.Uint16(w[0:2])
	h.length = binary.LittleEndian.Uint16(w[2:4])
	h.session = binary.LittleEndian.Uint32(w[4:8])
	h.status = binary.LittleEndian.Uint32(w[8:12])
	copy(h.context[:], w[12:20])
	h.options = binary.LittleEndian.Uint32(w[20:24])
	return h, true
}

// probeENIP deliberately admits only a complete, structurally valid
// RegisterSession request. A port hint or a 24-byte encapsulation-looking
// prefix alone never establishes a session.
func probeENIP(w []byte) ProbeResult {
	h, ok := parseENIPHeader(w)
	if !ok || h.command != enipCommandRegister || h.length != 4 || h.session != 0 || h.status != 0 || h.options != 0 || len(w) < enipHeaderSize+4 {
		return ProbeResult{Verdict: ProbeReject}
	}
	body := w[enipHeaderSize : enipHeaderSize+4]
	if binary.LittleEndian.Uint16(body) != 1 || binary.LittleEndian.Uint16(body[2:]) != 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("enip", "encapsulation-v1/cip-unconnected", 98)
}

func (f *binFlow) frameENIP(w []byte) (int, *binSpec, error) {
	h, ok := parseENIPHeader(w)
	if !ok {
		return 0, nil, nil
	}
	if h.command != enipCommandRegister && h.command != enipCommandSendRRData {
		return 0, nil, protocolError(ErrUnsupportedFeature, "EtherNet/IP encapsulation command 0x%04x is unsupported", h.command)
	}
	n := enipHeaderSize + int(h.length)
	if n > f.a.config.MaxMessageBytes {
		return n, nil, nil
	}
	if len(w) < n {
		return 0, nil, nil
	}
	if h.command == enipCommandRegister && h.length != 4 {
		return 0, nil, protocolError(ErrMalformedMessage, "EtherNet/IP RegisterSession must carry exactly four bytes")
	}
	return n, &binSpec{}, nil
}

func (p *binENIP) consume(dir int, wire []byte, collectionLimit int) (map[string]any, error) {
	h, ok := parseENIPHeader(wire)
	if !ok || enipHeaderSize+int(h.length) != len(wire) {
		return nil, protocolError(ErrMalformedMessage, "EtherNet/IP encapsulation length does not match the complete TCP message")
	}
	if h.options != 0 {
		return nil, protocolError(ErrMalformedMessage, "EtherNet/IP encapsulation options must be zero")
	}
	fields := map[string]any{
		"Command Code":   h.command,
		"Command":        enipCommandName(h.command),
		"Length":         h.length,
		"Session Handle": h.session,
		"Status":         h.status,
		"Sender Context": bytes.Clone(h.context[:]),
		"Options":        h.options,
		"Encapsulation":  "EtherNet/IP",
	}
	body := wire[enipHeaderSize:]
	switch h.command {
	case enipCommandRegister:
		return p.consumeRegister(dir, h, body, fields)
	case enipCommandSendRRData:
		if !p.registered {
			return nil, sessionContext("SendRRData preceded a successful RegisterSession response")
		}
		if h.status != 0 {
			return nil, protocolError(ErrMalformedMessage, "EtherNet/IP SendRRData status is 0x%08x", h.status)
		}
		if h.session != p.session {
			return nil, protocolError(ErrMalformedMessage, "EtherNet/IP session handle does not match the registered session")
		}
		if p.pending == nil {
			p.pending = make(map[enipPendingKey]enipPending)
		}
		if collectionLimit <= 0 {
			collectionLimit = 1
		}
		if p.maxPending <= 0 {
			p.maxPending = collectionLimit
		}
		cpf, err := parseENIPSendRRData(body)
		if err != nil {
			return nil, err
		}
		fields["Interface Handle"], fields["Timeout"] = cpf.interfaceHandle, cpf.timeout
		cipFields, err := p.consumeCIP(dir, cpf.cip, collectionLimit)
		if err != nil {
			return nil, err
		}
		for key, value := range cipFields {
			fields[key] = value
		}
		return fields, nil
	default:
		return nil, protocolError(ErrUnsupportedFeature, "EtherNet/IP encapsulation command 0x%04x is unsupported", h.command)
	}
}

func (p *binENIP) consumeRegister(dir int, h enipHeader, body []byte, fields map[string]any) (map[string]any, error) {
	if h.status != 0 {
		return nil, protocolError(ErrMalformedMessage, "EtherNet/IP RegisterSession status is 0x%08x", h.status)
	}
	if h.length != 4 || len(body) != 4 || binary.LittleEndian.Uint16(body) != 1 || binary.LittleEndian.Uint16(body[2:]) != 0 {
		return nil, protocolError(ErrMalformedMessage, "EtherNet/IP RegisterSession version/options are invalid")
	}
	fields["Protocol Version"] = binary.LittleEndian.Uint16(body)
	fields["Session Options"] = binary.LittleEndian.Uint16(body[2:])
	if !p.registered {
		if dir == p.clientDir {
			if h.session != 0 || p.registerRequestSeen {
				return nil, sessionContext("RegisterSession request has an unexpected session handle or is duplicated")
			}
			p.registerRequestSeen = true
			fields["Role"] = "RegisterSession Request"
			return fields, nil
		}
		if !p.registerRequestSeen || h.session == 0 {
			return nil, sessionContext("RegisterSession response has no matching request or valid session handle")
		}
		p.session, p.registered = h.session, true
		fields["Role"] = "RegisterSession Response"
		fields["Session Handle"] = p.session
		return fields, nil
	}
	return nil, sessionContext("duplicate RegisterSession exchange")
}

type enipCPF struct {
	interfaceHandle uint32
	timeout         uint16
	cip             []byte
}

func parseENIPSendRRData(body []byte) (enipCPF, error) {
	if len(body) < 8 {
		return enipCPF{}, protocolError(ErrMalformedMessage, "EtherNet/IP SendRRData CPF header is truncated")
	}
	interfaceHandle := binary.LittleEndian.Uint32(body[:4])
	timeout := binary.LittleEndian.Uint16(body[4:6])
	count := binary.LittleEndian.Uint16(body[6:8])
	if interfaceHandle != 0 || count != 2 {
		return enipCPF{}, protocolError(ErrMalformedMessage, "EtherNet/IP SendRRData requires interface handle zero and exactly two CPF items")
	}
	at := 8
	readItem := func() (uint16, []byte, bool) {
		if len(body)-at < 4 {
			return 0, nil, false
		}
		typ, length := binary.LittleEndian.Uint16(body[at:at+2]), int(binary.LittleEndian.Uint16(body[at+2:at+4]))
		at += 4
		if length > len(body)-at {
			return 0, nil, false
		}
		item := body[at : at+length]
		at += length
		return typ, item, true
	}
	addressType, address, ok := readItem()
	if !ok || addressType != enipCPFNullAddress || len(address) != 0 {
		return enipCPF{}, protocolError(ErrMalformedMessage, "EtherNet/IP SendRRData has an invalid null-address CPF item")
	}
	dataType, data, ok := readItem()
	if !ok || dataType != enipCPFUnconnectedData || len(data) == 0 || at != len(body) {
		return enipCPF{}, protocolError(ErrMalformedMessage, "EtherNet/IP SendRRData has an invalid unconnected-data CPF item or trailing bytes")
	}
	return enipCPF{interfaceHandle: interfaceHandle, timeout: timeout, cip: data}, nil
}

func (p *binENIP) consumeCIP(dir int, cip []byte, collectionLimit int) (map[string]any, error) {
	if len(cip) < 2 {
		return nil, protocolError(ErrMalformedMessage, "CIP service/path header is truncated")
	}
	service := cip[0]
	switch service {
	case enipCIPReadTag, enipCIPWriteTag:
		if dir != p.clientDir {
			return nil, sessionContext("CIP request was observed in the registered originator's peer direction")
		}
		if len(cip) < 2 {
			return nil, protocolError(ErrMalformedMessage, "CIP request path size is missing")
		}
		pathBytes := int(cip[1]) * 2
		if pathBytes == 0 || pathBytes > len(cip)-2 {
			return nil, protocolError(ErrMalformedMessage, "CIP request path size exceeds the service data")
		}
		tag, err := parseENIPTagPath(cip[2 : 2+pathBytes])
		if err != nil {
			return nil, err
		}
		data := cip[2+pathBytes:]
		fields := map[string]any{"CIP Service Code": service, "CIP Service": enipCIPServiceName(service), "Tag": tag}
		if service == enipCIPReadTag {
			if len(data) != 2 || binary.LittleEndian.Uint16(data) != 1 {
				return nil, protocolError(ErrUnsupportedFeature, "CIP Read Tag supports exactly one explicitly requested element")
			}
			fields["Element Count"] = uint16(1)
			if err := p.addPending(enipPending{service: service, tag: tag}, collectionLimit); err != nil {
				return nil, err
			}
			return fields, nil
		}
		if len(data) != 8 || binary.LittleEndian.Uint16(data[:2]) != enipCIPTypeDINT || binary.LittleEndian.Uint16(data[2:4]) != 1 {
			return nil, protocolError(ErrUnsupportedFeature, "CIP Write Tag supports one DINT element")
		}
		value := int32(binary.LittleEndian.Uint32(data[4:8]))
		fields["Data Type"], fields["Data Type Code"], fields["Element Count"], fields["Write Value"] = "DINT", uint16(enipCIPTypeDINT), uint16(1), value
		if err := p.addPending(enipPending{service: service, tag: tag, value: value}, collectionLimit); err != nil {
			return nil, err
		}
		return fields, nil
	case enipCIPReadTagReply, enipCIPWriteTagReply:
		if dir == p.clientDir {
			return nil, sessionContext("CIP response was observed in the registered originator's direction")
		}
		if len(cip) < 4 || cip[1] != 0 {
			return nil, protocolError(ErrMalformedMessage, "CIP response reserved byte is invalid")
		}
		generalStatus, additionalWords := cip[2], int(cip[3])
		if 4+additionalWords*2 > len(cip) {
			return nil, protocolError(ErrMalformedMessage, "CIP additional-status length exceeds the response")
		}
		if generalStatus != 0 || additionalWords != 0 {
			return nil, protocolError(ErrUnsupportedFeature, "CIP response status 0x%02x with %d additional words is unsupported", generalStatus, additionalWords)
		}
		if service == enipCIPReadTagReply {
			if len(cip) != 10 || binary.LittleEndian.Uint16(cip[4:6]) != enipCIPTypeDINT {
				return nil, protocolError(ErrUnsupportedFeature, "CIP Read Tag response must contain one DINT")
			}
			pending, err := p.takePending(enipCIPReadTag)
			if err != nil {
				return nil, err
			}
			value := int32(binary.LittleEndian.Uint32(cip[6:10]))
			return map[string]any{"CIP Service Code": service, "CIP Service": enipCIPServiceName(service), "Tag": pending.tag, "Data Type": "DINT", "Data Type Code": uint16(enipCIPTypeDINT), "Element Count": uint16(1), "Read Value": value}, nil
		}
		if len(cip) != 4 {
			return nil, protocolError(ErrMalformedMessage, "CIP Write Tag response must not contain service data")
		}
		pending, err := p.takePending(enipCIPWriteTag)
		if err != nil {
			return nil, err
		}
		return map[string]any{"CIP Service Code": service, "CIP Service": enipCIPServiceName(service), "Tag": pending.tag, "Data Type": "DINT", "Data Type Code": uint16(enipCIPTypeDINT), "Element Count": uint16(1), "Write Value": pending.value, "Write Confirmed": true}, nil
	default:
		return nil, protocolError(ErrUnsupportedFeature, "CIP service 0x%02x is unsupported by this profile", service)
	}
}

func parseENIPTagPath(path []byte) (string, error) {
	if len(path) < 2 || path[0] != enipCIPANSIExtendedPath {
		return "", protocolError(ErrUnsupportedFeature, "CIP path must be one ANSI extended-symbol segment")
	}
	size := int(path[1])
	padded := size + size%2
	if size == 0 || len(path) != 2+padded {
		return "", protocolError(ErrMalformedMessage, "CIP ANSI symbol length does not match the path size")
	}
	if size%2 == 1 && path[len(path)-1] != 0 {
		return "", protocolError(ErrMalformedMessage, "CIP ANSI symbol pad byte must be zero")
	}
	symbol := path[2 : 2+size]
	if !isENIPTagSymbol(symbol) {
		return "", protocolError(ErrUnsupportedFeature, "CIP symbol is outside the supported simple-tag profile")
	}
	return string(symbol), nil
}

func isENIPTagSymbol(symbol []byte) bool {
	if len(symbol) == 0 || !((symbol[0] >= 'A' && symbol[0] <= 'Z') || (symbol[0] >= 'a' && symbol[0] <= 'z') || symbol[0] == '_') {
		return false
	}
	for _, c := range symbol[1:] {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

func (p *binENIP) addPending(request enipPending, limit int) error {
	key := enipPendingKey{dir: p.clientDir, service: request.service, tag: request.tag}
	for candidate := range p.pending {
		if candidate.service == request.service {
			return sessionContext("CIP response correlation is ambiguous while another request of the same service is outstanding")
		}
	}
	if p.maxPending <= 0 {
		p.maxPending = limit
	}
	if len(p.pending) >= p.maxPending {
		return protocolError(ErrResourceExceeded, "EtherNet/IP pending CIP request budget exceeded")
	}
	p.pending[key] = request
	return nil
}

func (p *binENIP) takePending(service byte) (enipPending, error) {
	var match enipPendingKey
	found := false
	for candidate, request := range p.pending {
		if candidate.dir == p.clientDir && candidate.service == service && request.service == service {
			if found {
				return enipPending{}, sessionContext("CIP response matches multiple outstanding requests")
			}
			match, found = candidate, true
		}
	}
	if !found {
		return enipPending{}, sessionContext("CIP response has no matching outstanding request")
	}
	request := p.pending[match]
	delete(p.pending, match)
	return request, nil
}

func enipCommandName(command uint16) string {
	switch command {
	case enipCommandRegister:
		return "RegisterSession"
	case enipCommandSendRRData:
		return "SendRRData"
	default:
		return fmt.Sprintf("0x%04x", command)
	}
}

func enipCIPServiceName(service byte) string {
	switch service {
	case enipCIPReadTag:
		return "Read Tag Request"
	case enipCIPReadTagReply:
		return "Read Tag Response"
	case enipCIPWriteTag:
		return "Write Tag Request"
	case enipCIPWriteTagReply:
		return "Write Tag Response"
	default:
		return fmt.Sprintf("0x%02x", service)
	}
}
