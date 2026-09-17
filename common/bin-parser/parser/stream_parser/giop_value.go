package stream_parser

import (
	"encoding/binary"
	"fmt"
)

const (
	giopMaxBodySize = 1 << 20
	giopMaxItems    = 1024
)

// giopField describes an original wire span, relative to the GIOP body.
// Containers have an empty Type. Alignment octets are separate raw fields,
// rather than being silently included in the value of the following scalar.
type giopField struct {
	Name       string
	Type       string
	Start, End int
	Children   []giopField
	List       bool
}

// decodeGIOPBody decodes the versioned envelopes from CORBA 3.2,
// Interoperability, sections 9.3 and 9.4. Unknown IDL parameters and encapsulated
// profile/context data remain opaque. It does not perform fragment reassembly.
// The caller must independently enforce the fixed header's declared body size.
func decodeGIOPBody(body []byte, major, minor, flags, messageType byte) ([]giopField, error) {
	if major != 1 || minor > 3 {
		return nil, fmt.Errorf("unsupported GIOP version %d.%d", major, minor)
	}
	if len(body) > giopMaxBodySize {
		return nil, fmt.Errorf("GIOP body exceeds %d bytes", giopMaxBodySize)
	}
	if flags&0xfc != 0 || (minor == 0 && flags > 1) {
		return nil, fmt.Errorf("invalid GIOP %d.%d flags %#x", major, minor, flags)
	}
	if messageType > 7 || (minor == 0 && messageType == 7) {
		return nil, fmt.Errorf("invalid GIOP %d.%d message type %d", major, minor, messageType)
	}
	more := flags&2 != 0
	if more {
		allowed := messageType == 0 || messageType == 1 || messageType == 7 || (minor >= 2 && (messageType == 3 || messageType == 4))
		if !allowed {
			return nil, fmt.Errorf("GIOP message type %d cannot be fragmented", messageType)
		}
		if minor >= 2 && (12+len(body))%8 != 0 {
			return nil, fmt.Errorf("non-final GIOP fragment length must be a multiple of eight")
		}
	}
	if len(body) == 0 && (messageType == 0 || messageType == 1 || messageType == 3 || messageType == 4) {
		return nil, fmt.Errorf("zero-sized GIOP message type %d is reserved", messageType)
	}
	var order binary.ByteOrder = binary.BigEndian
	if flags&1 != 0 {
		order = binary.LittleEndian
	}
	r := giopCDRReader{data: body, order: order}
	fields := make([]giopField, 0, 16)
	name := ""
	if more && messageType != 7 {
		// Even the initial message header can straddle fragments. GIOP 1.2+
		// puts its request ID first, so that one field is independently known.
		name = "Fragmented Message"
		if minor >= 2 {
			r.number(&fields, "Request ID", 4)
		}
		r.rest(&fields, "Fragment Data")
	} else {
		switch messageType {
		case 0:
			name = "GIOPRequest"
			r.request(&fields, minor)
		case 1:
			name = "GIOPReply"
			r.reply(&fields, minor)
		case 2:
			name = "CancelRequest"
			r.number(&fields, "Request ID", 4)
		case 3:
			name = "LocateRequest"
			r.number(&fields, "Request ID", 4)
			if minor < 2 {
				r.sequence(&fields, "Key Len", "Object Key", "string")
			} else {
				r.target(&fields)
			}
		case 4:
			name = "LocateReply"
			r.number(&fields, "Request ID", 4)
			status := r.number(&fields, "Locate Status", 4)
			if status > 5 || (minor < 2 && status > 2) {
				r.fail("invalid locate status %d for GIOP 1.%d", status, minor)
			}
			// Unlike Request/Reply 1.2 bodies, LocateReply bodies have no
			// additional eight-octet alignment requirement.
			switch status {
			case 2, 3:
				r.ior(&fields)
			case 4:
				r.systemException(&fields)
			case 5:
				r.disposition(&fields, "Addressing Disposition")
			}
		case 5, 6:
			if len(body) != 0 {
				return nil, fmt.Errorf("GIOP control message type %d must have an empty body", messageType)
			}
			return nil, nil
		case 7:
			name = "Fragment"
			if minor >= 2 {
				r.number(&fields, "Request ID", 4)
			}
			r.rest(&fields, "Fragment Data")
		}
	}
	if r.err == nil && r.pos != len(body) {
		r.fail("unexpected %d trailing body bytes", len(body)-r.pos)
	}
	if r.err != nil {
		return nil, r.err
	}
	return []giopField{{Name: name, Start: 0, End: len(body), Children: fields}}, nil
}

type giopCDRReader struct {
	data  []byte
	order binary.ByteOrder
	pos   int
	err   error
}

func (r *giopCDRReader) fail(format string, args ...interface{}) {
	if r.err == nil {
		r.err = fmt.Errorf("GIOP body offset %d: %s", r.pos, fmt.Sprintf(format, args...))
	}
}

func (r *giopCDRReader) raw(fields *[]giopField, name, typ string, n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > len(r.data)-r.pos {
		r.fail("truncated %s: need %d bytes, have %d", name, n, len(r.data)-r.pos)
		return nil
	}
	start := r.pos
	r.pos += n
	*fields = append(*fields, giopField{Name: name, Type: typ, Start: start, End: r.pos})
	return r.data[start:r.pos]
}

func (r *giopCDRReader) align(fields *[]giopField, n int, name string) {
	padding := (-(12 + r.pos)) & (n - 1)
	if padding != 0 {
		r.raw(fields, name, "raw", padding)
	}
}

func (r *giopCDRReader) number(fields *[]giopField, name string, size int) uint32 {
	r.align(fields, size, name+" Padding")
	var typ string
	switch size {
	case 1:
		typ = "uint8"
	case 2:
		typ = "uint16"
	case 4:
		typ = "uint32"
	}
	b := r.raw(fields, name, typ, size)
	if r.err != nil {
		return 0
	}
	switch size {
	case 1:
		return uint32(b[0])
	case 2:
		return uint32(r.order.Uint16(b))
	default:
		return r.order.Uint32(b)
	}
}

func (r *giopCDRReader) sequence(fields *[]giopField, lengthName, dataName, typ string) []byte {
	n := r.number(fields, lengthName, 4)
	if uint64(n) > uint64(len(r.data)-r.pos) {
		r.fail("%s length %d exceeds remaining body", dataName, n)
		return nil
	}
	return r.raw(fields, dataName, typ, int(n))
}

func (r *giopCDRReader) cdrString(fields *[]giopField, lengthName, name string) {
	b := r.sequence(fields, lengthName, name, "string")
	if r.err == nil && (len(b) == 0 || b[len(b)-1] != 0) {
		r.fail("%s must have a nonzero CDR length and terminal NUL", name)
	}
}

func (r *giopCDRReader) rest(fields *[]giopField, name string) {
	r.raw(fields, name, "raw", len(r.data)-r.pos)
}

func (r *giopCDRReader) serviceContexts(fields *[]giopField) {
	n := r.number(fields, "Service Context Count", 4)
	if n > giopMaxItems {
		r.fail("service context count %d exceeds %d", n, giopMaxItems)
	}
	if r.err != nil {
		return
	}
	list := giopField{Name: "Service Contexts", Start: r.pos, List: true}
	for i := uint32(0); i < n && r.err == nil; i++ {
		entry := giopField{Name: "Service Context", Start: r.pos}
		r.number(&entry.Children, "Context ID", 4)
		r.sequence(&entry.Children, "Context Data Length", "Context Data", "raw")
		if i+1 < n {
			r.align(&entry.Children, 4, "Context Padding")
		}
		entry.End = r.pos
		list.Children = append(list.Children, entry)
	}
	list.End = r.pos
	*fields = append(*fields, list)
}

func (r *giopCDRReader) disposition(fields *[]giopField, name string) uint32 {
	v := r.number(fields, name, 2)
	if v > 2 {
		r.fail("invalid addressing disposition %d", v)
	}
	return v
}

func (r *giopCDRReader) target(fields *[]giopField) {
	disc := r.disposition(fields, "Addr Disc")
	r.align(fields, 4, "Addr Pad")
	switch disc {
	case 0:
		// Object keys are octet sequences, not CDR strings. The legacy
		// display type/name is retained without imposing NUL semantics.
		r.sequence(fields, "Key Len", "Object Key", "string")
	case 1:
		r.taggedProfile(fields)
	case 2:
		index := r.number(fields, "Selected Profile Index", 4)
		count := r.ior(fields)
		if index >= count {
			r.fail("selected profile index %d is outside profile count %d", index, count)
		}
	}
}

func (r *giopCDRReader) taggedProfile(fields *[]giopField) {
	profile := giopField{Name: "Profile", Start: r.pos}
	r.number(&profile.Children, "Profile ID", 4)
	r.sequence(&profile.Children, "Profile Data Length", "Profile Data", "raw")
	profile.End = r.pos
	*fields = append(*fields, profile)
}

func (r *giopCDRReader) ior(fields *[]giopField) uint32 {
	ior := giopField{Name: "IOR", Start: r.pos}
	r.cdrString(&ior.Children, "Type ID Length", "Type ID")
	n := r.number(&ior.Children, "Profile Count", 4)
	if n > giopMaxItems {
		r.fail("profile count %d exceeds %d", n, giopMaxItems)
	}
	if r.err != nil {
		return 0
	}
	profiles := giopField{Name: "Profiles", Start: r.pos, List: true}
	for i := uint32(0); i < n && r.err == nil; i++ {
		r.taggedProfile(&profiles.Children)
	}
	profiles.End = r.pos
	ior.Children = append(ior.Children, profiles)
	ior.End = r.pos
	*fields = append(*fields, ior)
	return n
}

func (r *giopCDRReader) systemException(fields *[]giopField) {
	exception := giopField{Name: "System Exception", Start: r.pos}
	r.cdrString(&exception.Children, "Exception ID Length", "Exception ID")
	r.number(&exception.Children, "Minor Code", 4)
	completion := r.number(&exception.Children, "Completion Status", 4)
	if completion > 2 {
		r.fail("invalid completion status %d", completion)
	}
	exception.End = r.pos
	*fields = append(*fields, exception)
}

func (r *giopCDRReader) bodyAlignment(fields *[]giopField, minor byte) {
	if minor >= 2 && r.pos < len(r.data) && r.err == nil {
		r.align(fields, 8, "Body Padding")
		if r.err == nil && r.pos == len(r.data) {
			r.fail("padding without a message body")
		}
	}
}

func (r *giopCDRReader) request(fields *[]giopField, minor byte) {
	if minor < 2 {
		r.serviceContexts(fields)
	}
	r.number(fields, "Request ID", 4)
	response := r.number(fields, "Response Flags", 1)
	if (minor < 2 && response > 1) || (minor >= 2 && response != 0 && response != 1 && response != 3) {
		r.fail("invalid response flags %d for GIOP 1.%d", response, minor)
	}
	if minor == 0 {
		r.align(fields, 4, "Request Padding")
	} else {
		reserved := r.raw(fields, "Reserved", "raw", 3)
		if minor == 1 && len(reserved) == 3 && (reserved[0]|reserved[1]|reserved[2]) != 0 {
			r.fail("GIOP 1.1 request reserved octets must be zero")
		}
	}
	if minor < 2 {
		r.sequence(fields, "Key Len", "Object Key", "string")
	} else {
		r.target(fields)
	}
	r.cdrString(fields, "Op Len", "Operation")
	if minor < 2 {
		r.sequence(fields, "Principal Length", "Principal", "raw")
	} else {
		r.serviceContexts(fields)
	}
	r.bodyAlignment(fields, minor)
	r.rest(fields, "Stub Data")
}

func (r *giopCDRReader) reply(fields *[]giopField, minor byte) {
	if minor < 2 {
		r.serviceContexts(fields)
	}
	r.number(fields, "Request ID", 4)
	status := r.number(fields, "Reply Status", 4)
	if status > 5 || (minor < 2 && status > 3) {
		r.fail("invalid reply status %d for GIOP 1.%d", status, minor)
	}
	if minor >= 2 {
		r.serviceContexts(fields)
	}
	r.bodyAlignment(fields, minor)
	switch status {
	case 0:
		r.rest(fields, "Stub Data")
	case 1:
		r.cdrString(fields, "Exception ID Length", "Exception ID")
		r.rest(fields, "Stub Data")
	case 2:
		r.systemException(fields)
	case 3, 4:
		r.ior(fields)
	case 5:
		r.disposition(fields, "Addressing Disposition")
	}
}
