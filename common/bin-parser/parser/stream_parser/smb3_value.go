package stream_parser

import (
	"encoding/binary"
	"fmt"
)

// MS-SMB2 revision 87.0, 2.2.1, 2.2.3, 2.2.4, 3.2.5.2 and 3.3.5.4.
// This is a bounded, stateless NEGOTIATE layout profile, not a session or
// algorithm implementation. All offsets are relative to the SMB2 header.
const smb3NegotiateMaxBytes = 65536

type smb3Field struct {
	Name, Type string
	Start, End int
	List       bool
	Children   []smb3Field
}

type smb3Reader struct {
	wire       []byte
	pos, limit int
	err        error
	opaque     int
}

func (r *smb3Reader) fail(format string, args ...any) {
	if r.err == nil {
		r.err = fmt.Errorf("smb3 negotiate: offset %d: %s", r.pos, fmt.Sprintf(format, args...))
	}
}
func (r *smb3Reader) field(out *[]smb3Field, name, typ string, n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > r.limit-r.pos {
		r.fail("truncated %s", name)
		return nil
	}
	start := r.pos
	r.pos += n
	*out = append(*out, smb3Field{Name: name, Type: typ, Start: start * 8, End: r.pos * 8})
	return r.wire[start:r.pos]
}
func (r *smb3Reader) num(out *[]smb3Field, name string, n int) uint64 {
	b := r.field(out, name, fmt.Sprintf("uint%d", n*8), n)
	if r.err != nil {
		return 0
	}
	switch n {
	case 2:
		return uint64(binary.LittleEndian.Uint16(b))
	case 4:
		return uint64(binary.LittleEndian.Uint32(b))
	case 8:
		return binary.LittleEndian.Uint64(b)
	}
	return uint64(b[0])
}
func (r *smb3Reader) array(out *[]smb3Field, name, element string, n int) []uint16 {
	list := smb3Field{Name: name, Start: r.pos * 8, List: true}
	if n > (r.limit-r.pos)/2 {
		r.fail("%s count exceeds remaining bytes", name)
	}
	var values []uint16
	for i := 0; i < n && r.err == nil; i++ {
		values = append(values, uint16(r.num(&list.Children, element, 2)))
	}
	list.End = r.pos * 8
	*out = append(*out, list)
	return values
}
func (r *smb3Reader) gap(out *[]smb3Field, name string, offset int) {
	if offset < r.pos || offset > r.limit {
		r.fail("invalid or overlapping %s offset", name)
		return
	}
	if offset != r.pos {
		r.field(out, name, "raw", offset-r.pos)
	}
}

func decodeSMB3Negotiate(wire []byte) ([]smb3Field, map[string]any, error) {
	if len(wire) < 100 || len(wire) > smb3NegotiateMaxBytes {
		return nil, nil, fmt.Errorf("smb3 negotiate: explicit record must be 100..65536 bytes")
	}
	r := smb3Reader{wire: wire, limit: len(wire)}
	var fields []smb3Field
	if r.num(&fields, "ProtocolId", 4) != 0x424d53fe {
		r.fail("ProtocolId must be FE SMB")
	}
	if r.num(&fields, "Header StructureSize", 2) != 64 {
		r.fail("header StructureSize must be 64")
	}
	r.num(&fields, "CreditCharge", 2)
	reply := binary.LittleEndian.Uint32(wire[16:20])&1 != 0
	if reply {
		if r.num(&fields, "Status", 4) != 0 {
			r.fail("non-success response outside NEGOTIATE profile")
		}
	} else {
		r.num(&fields, "ChannelSequence", 2)
		r.num(&fields, "Header Reserved", 2)
	}
	if r.num(&fields, "Command", 2) != 0 {
		r.fail("only NEGOTIATE command is supported")
	}
	creditName := "CreditRequest"
	if reply {
		creditName = "CreditResponse"
	}
	r.num(&fields, creditName, 2)
	flags := r.num(&fields, "Flags", 4)
	if flags&2 != 0 {
		r.fail("asynchronous header outside NEGOTIATE profile")
	}
	if flags&4 != 0 {
		r.fail("related compound operation outside single NEGOTIATE profile")
	}
	if r.num(&fields, "NextCommand", 4) != 0 {
		r.fail("compound message outside single NEGOTIATE profile")
	}
	r.num(&fields, "MessageId", 8)
	r.num(&fields, "Sync Reserved", 4)
	r.num(&fields, "TreeId", 4)
	if r.num(&fields, "SessionId", 8) != 0 {
		r.fail("NEGOTIATE SessionId must be zero")
	}
	r.field(&fields, "Signature", "raw", 16)
	body := smb3Field{Name: "Negotiate Request", Start: 64 * 8}
	if reply {
		body.Name = "Negotiate Response"
	}
	bf := &body.Children
	version311 := false
	hasSMB3 := false
	contextOffset, contextCount := 0, 0
	if !reply {
		if r.num(bf, "StructureSize", 2) != 36 {
			r.fail("request StructureSize must be 36")
		}
		count := int(r.num(bf, "DialectCount", 2))
		if count == 0 {
			r.fail("DialectCount must be nonzero")
		}
		r.num(bf, "SecurityMode", 2)
		r.num(bf, "Reserved", 2)
		r.num(bf, "Capabilities", 4)
		r.field(bf, "ClientGuid", "raw", 16)
		if count > (len(wire)-100)/2 {
			r.fail("DialectCount exceeds record boundary")
		}
		if r.err == nil {
			for i := 0; i < count; i++ {
				d := binary.LittleEndian.Uint16(wire[100+i*2:])
				version311 = version311 || d == 0x0311
				hasSMB3 = hasSMB3 || d == 0x0300 || d == 0x0302 || d == 0x0311
			}
		}
		if version311 {
			contextOffset = int(r.num(bf, "NegotiateContextOffset", 4))
			contextCount = int(r.num(bf, "NegotiateContextCount", 2))
			r.num(bf, "Reserved2", 2)
		} else {
			r.field(bf, "ClientStartTime Reserved", "raw", 8)
		}
		r.array(bf, "Dialects", "Dialect", count)
	} else {
		if r.num(bf, "StructureSize", 2) != 65 {
			r.fail("response StructureSize must be 65")
		}
		r.num(bf, "SecurityMode", 2)
		d := r.num(bf, "DialectRevision", 2)
		version311 = d == 0x0311
		hasSMB3 = d == 0x0300 || d == 0x0302 || version311
		name := "Reserved"
		if version311 {
			name = "NegotiateContextCount"
		}
		contextCount = int(r.num(bf, name, 2))
		r.field(bf, "ServerGuid", "raw", 16)
		for _, n := range []string{"Capabilities", "MaxTransactSize", "MaxReadSize", "MaxWriteSize"} {
			r.num(bf, n, 4)
		}
		r.num(bf, "SystemTime", 8)
		r.num(bf, "ServerStartTime", 8)
		bufferOffset := int(r.num(bf, "SecurityBufferOffset", 2))
		bufferLength := int(r.num(bf, "SecurityBufferLength", 2))
		name = "Reserved2"
		if version311 {
			name = "NegotiateContextOffset"
		}
		contextOffset = int(r.num(bf, name, 4))
		if bufferLength > 0 {
			r.gap(bf, "Security Buffer Padding", bufferOffset)
			r.field(bf, "Security Buffer", "raw", bufferLength)
		}
	}
	if !hasSMB3 {
		r.fail("no SMB 3.0, 3.0.2 or 3.1.1 dialect in profile")
	}
	if version311 && r.err == nil {
		if contextCount == 0 {
			r.fail("SMB 3.1.1 requires exactly one PREAUTH context")
		}
		if contextOffset%8 != 0 {
			r.fail("NegotiateContextOffset must be 8-byte aligned")
		}
		r.gap(bf, "Context Padding", contextOffset)
		list := smb3Field{Name: "Negotiate Contexts", Start: r.pos * 8, List: true}
		seen := map[uint16]int{}
		for i := 0; i < contextCount && r.err == nil; i++ {
			ctx := smb3Field{Name: "Context", Start: r.pos * 8}
			if i > 0 {
				r.gap(&ctx.Children, "Context Alignment Padding", (r.pos+7)&^7)
			}
			typ := uint16(r.num(&ctx.Children, "ContextType", 2))
			n := int(r.num(&ctx.Children, "DataLength", 2))
			r.num(&ctx.Children, "Context Reserved", 4)
			seen[typ]++
			if seen[typ] > 1 && (typ == 1 || typ == 2 || typ == 3 || typ == 7 || typ == 8 || reply && typ == 6) {
				r.fail("duplicate context type 0x%04x", typ)
			}
			if n > r.limit-r.pos {
				r.fail("context DataLength exceeds record boundary")
			}
			if r.err == nil {
				limit := r.limit
				r.limit = r.pos + n
				r.context(&ctx.Children, typ, reply)
				if r.err == nil && r.pos < r.limit {
					r.opaque += r.limit - r.pos
					r.field(&ctx.Children, "Uninterpreted Context Tail", "raw", r.limit-r.pos)
				}
				r.limit = limit
			}
			ctx.End = r.pos * 8
			list.Children = append(list.Children, ctx)
		}
		list.End = r.pos * 8
		*bf = append(*bf, list)
		if seen[1] != 1 {
			r.fail("SMB 3.1.1 requires exactly one PREAUTH context")
		}
	}
	// No field specifies a length for trailing bytes. Preserve them explicitly,
	// as the receive path hashes all bytes; never label them another context.
	if r.err == nil && r.pos < r.limit {
		r.opaque += r.limit - r.pos
		r.field(bf, "Uninterpreted Record Tail", "raw", r.limit-r.pos)
	}
	body.End = r.pos * 8
	fields = append(fields, body)
	if r.err != nil {
		return nil, nil, r.err
	}
	info := map[string]any{"Profile": "SMB 3.x single NEGOTIATE structural fields", "Maximum Record Bytes": smb3NegotiateMaxBytes, "Response": reply, "SMB 3.1.1 Contexts": version311, "Uninterpreted Bytes": r.opaque, "Session State Validated": false, "Algorithm Negotiation Validated": false, "Signature Verified": false, "Security Buffer Decoded": false, "Transform Payload Decoded": false, "Transport Reassembled": false}
	return fields, info, nil
}

func (r *smb3Reader) context(out *[]smb3Field, typ uint16, reply bool) {
	array := func(countName, listName, elementName string, prefix int, single bool) []uint16 {
		count := int(r.num(out, countName, 2))
		if count == 0 || single && count != 1 {
			r.fail("invalid %s", countName)
		}
		if prefix == 8 {
			r.num(out, "Algorithm Reserved1", 2)
			r.num(out, "Algorithm Reserved2", 4)
		}
		return r.array(out, listName, elementName, count)
	}
	switch typ {
	case 1:
		count := int(r.num(out, "HashAlgorithmCount", 2))
		salt := int(r.num(out, "SaltLength", 2))
		if count == 0 || reply && count != 1 {
			r.fail("invalid HashAlgorithmCount")
		}
		r.array(out, "HashAlgorithms", "HashAlgorithm", count)
		r.field(out, "Salt", "raw", salt)
	case 2:
		array("CipherCount", "Ciphers", "Cipher", 2, reply)
	case 3:
		count := int(r.num(out, "CompressionAlgorithmCount", 2))
		r.num(out, "Compression Padding", 2)
		r.num(out, "Compression Flags", 4)
		if count == 0 {
			r.fail("invalid CompressionAlgorithmCount")
		}
		values := r.array(out, "CompressionAlgorithms", "CompressionAlgorithm", count)
		if reply {
			seen := map[uint16]bool{}
			for _, v := range values {
				if v >= 32 || seen[v] {
					r.fail("invalid or duplicate response compression algorithm")
				}
				seen[v] = true
			}
		}
	case 5:
		// Both receiving peers ignore this context. Preserve odd-length input
		// too; valid UTF-16LE gets code-unit fields, not a guessed DNS identity.
		if reply || (r.limit-r.pos)%2 != 0 {
			r.opaque += r.limit - r.pos
			r.field(out, "Uninterpreted NetName", "raw", r.limit-r.pos)
		} else {
			r.array(out, "NetName UTF16LE", "Code Unit", (r.limit-r.pos)/2)
		}
	case 6:
		r.num(out, "Transport Flags", 4)
	case 7:
		array("TransformCount", "RDMATransformIds", "RDMATransformId", 8, false)
	case 8:
		array("SigningAlgorithmCount", "SigningAlgorithms", "SigningAlgorithm", 2, reply)
	default:
		r.opaque += r.limit - r.pos
		r.field(out, "Uninterpreted Context Data", "raw", r.limit-r.pos)
	}
}
