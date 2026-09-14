package stream_parser

import (
	"encoding/binary"
	"fmt"

	"github.com/miekg/dns"
)

// Caller supplies one complete, cleartext, reassembled QUIC stream direction.
// Version selection, stream identity, FIN and transport processing are external.
// RFC 9250 sections 4.2/4.2.1/4.3.3 use uint16 message lengths; the historical
// draft-ietf-dprive-dnsoquic-00 sections 4.1.1/4.2/5.4 doq-i00 mapping does not.
type doqField struct {
	Name, Type string
	Start, End int
	Children   []doqField
	Value      any
	Decoded    bool
	Info       map[string]any
}

func doqRaw(name string, start, end int) doqField {
	return doqField{Name: name, Type: "raw", Start: start, End: end}
}

func doqNumber(name string, start, size int) doqField {
	return doqField{Name: name, Type: fmt.Sprint("uint", size*8), Start: start, End: start + size}
}

type doqDNS struct {
	wire   []byte
	base   int
	pos    int
	labels map[int]bool
	opt    bool
}

func (d *doqDNS) name() (doqField, error) {
	start, pos := d.pos, d.pos
	for {
		if pos >= len(d.wire) {
			return doqField{}, fmt.Errorf("doq: truncated DNS name")
		}
		b := d.wire[pos]
		if b&0xc0 == 0xc0 {
			if pos+2 > len(d.wire) {
				return doqField{}, fmt.Errorf("doq: truncated DNS pointer")
			}
			target := int(binary.BigEndian.Uint16(d.wire[pos:]) & 0x3fff)
			if target >= pos || !d.labels[target] {
				return doqField{}, fmt.Errorf("doq: DNS pointer must reference an earlier decoded label")
			}
			d.labels[pos] = true
			pos += 2
			break
		}
		if b&0xc0 != 0 || pos+1+int(b) > len(d.wire) {
			return doqField{}, fmt.Errorf("doq: invalid or truncated DNS label")
		}
		d.labels[pos] = true
		pos += 1 + int(b)
		if b == 0 {
			break
		}
	}
	// Existing pinned dependency bounds expanded names and pointer chains and
	// gives escaped DNS presentation text, without performing any DNS lookup.
	text, end, err := dns.UnpackDomainName(d.wire, start)
	if err != nil || end != pos {
		return doqField{}, fmt.Errorf("doq: invalid expanded DNS name: %v", err)
	}
	d.pos = pos
	f := doqRaw("Name", d.base+start, d.base+pos)
	f.Decoded, f.Value = true, text
	return f, nil
}

func (d *doqDNS) records(name string, count int, question bool) (doqField, error) {
	list := doqField{Name: name, Start: d.base + d.pos, Info: map[string]any{"Count": count}}
	for i := 0; i < count; i++ {
		entry := doqField{Name: "Record", Start: d.base + d.pos}
		if question {
			entry.Name = "Question"
		}
		owner, err := d.name()
		if err != nil {
			return list, err
		}
		entry.Children = append(entry.Children, owner)
		need := 10
		if question {
			need = 4
		}
		if d.pos+need > len(d.wire) {
			return list, fmt.Errorf("doq: truncated DNS record header")
		}
		typ := binary.BigEndian.Uint16(d.wire[d.pos:])
		entry.Children = append(entry.Children, doqNumber("Type", d.base+d.pos, 2))
		if question {
			entry.Children = append(entry.Children, doqNumber("Class", d.base+d.pos+2, 2))
			d.pos += 4
		} else {
			length := int(binary.BigEndian.Uint16(d.wire[d.pos+8:]))
			if typ == 41 {
				if d.opt || name != "Additional" || owner.Value != "." || owner.End-owner.Start != 1 {
					return list, fmt.Errorf("doq: OPT requires one uncompressed root owner in Additional")
				}
				d.opt = true
				if d.wire[d.pos+5] != 0 {
					return list, fmt.Errorf("doq: EDNS version outside version-zero profile")
				}
				entry.Name = "OPT"
				entry.Children = append(entry.Children, doqNumber("UDP Payload Size", d.base+d.pos+2, 2),
					doqNumber("Extended RCODE", d.base+d.pos+4, 1), doqNumber("EDNS Version", d.base+d.pos+5, 1), doqNumber("EDNS Flags", d.base+d.pos+6, 2))
			} else {
				entry.Children = append(entry.Children, doqNumber("Class", d.base+d.pos+2, 2), doqNumber("TTL", d.base+d.pos+4, 4))
			}
			entry.Children = append(entry.Children, doqNumber("RDLength", d.base+d.pos+8, 2))
			d.pos += 10
			end := d.pos + length
			if end > len(d.wire) {
				return list, fmt.Errorf("doq: truncated DNS RData")
			}
			switch typ {
			case 1:
				if length != 4 {
					return list, fmt.Errorf("doq: A record requires four octets")
				}
				entry.Children = append(entry.Children, doqRaw("Address", d.base+d.pos, d.base+end))
			case 41:
				options := doqField{Name: "EDNS Options", Start: d.base + d.pos, End: d.base + end}
				for count := 0; d.pos < end; count++ {
					if count >= 256 || d.pos+4 > end {
						return list, fmt.Errorf("doq: truncated or excessive EDNS options")
					}
					code := binary.BigEndian.Uint16(d.wire[d.pos:])
					length := int(binary.BigEndian.Uint16(d.wire[d.pos+2:]))
					if code == 11 {
						return list, fmt.Errorf("doq: edns-tcp-keepalive is prohibited on DoQ")
					}
					if d.pos+4+length > end {
						return list, fmt.Errorf("doq: EDNS option length exceeds RData")
					}
					option := doqField{Name: "EDNS Option", Start: d.base + d.pos, End: d.base + d.pos + 4 + length,
						Children: []doqField{doqNumber("Option Code", d.base+d.pos, 2), doqNumber("Option Length", d.base+d.pos+2, 2), doqRaw("Option Data", d.base+d.pos+4, d.base+d.pos+4+length)}}
					options.Children = append(options.Children, option)
					d.pos += 4 + length
				}
				entry.Children = append(entry.Children, options)
			default:
				f := doqRaw("RData", d.base+d.pos, d.base+end)
				f.Info = map[string]any{"Meaning": "Opaque record-specific data; schema not decoded"}
				entry.Children = append(entry.Children, f)
			}
			d.pos = end
		}
		entry.End = d.base + d.pos
		list.Children = append(list.Children, entry)
	}
	list.End = d.base + d.pos
	return list, nil
}

func decodeDoQDNS(wire []byte, base int, response bool) (doqField, error) {
	root := doqField{Name: "DNS Message", Start: base, End: base + len(wire)}
	if len(wire) < 12 || len(wire) > 65535 {
		return root, fmt.Errorf("doq: DNS message length must be 12..65535 bytes")
	}
	if binary.BigEndian.Uint16(wire) != 0 {
		return root, fmt.Errorf("doq: DNS Message ID must be zero")
	}
	flags := binary.BigEndian.Uint16(wire[2:])
	if (flags&0x8000 != 0) != response {
		return root, fmt.Errorf("doq: DNS QR does not match caller-selected direction")
	}
	if (flags>>11)&15 != 0 {
		return root, fmt.Errorf("doq: DNS opcode outside QUERY profile")
	}
	if flags&0x0040 != 0 {
		return root, fmt.Errorf("doq: DNS reserved Z bit must be zero")
	}
	counts := []int{}
	total := 0
	for off := 4; off < 12; off += 2 {
		n := int(binary.BigEndian.Uint16(wire[off:]))
		counts = append(counts, n)
		total += n
	}
	if total > 1024 {
		return root, fmt.Errorf("doq: DNS record count limit")
	}
	header := doqField{Name: "Header", Start: base, End: base + 12}
	for i, name := range []string{"ID", "Flags", "Questions", "Answer RRs", "Authority RRs", "Additional RRs"} {
		header.Children = append(header.Children, doqNumber(name, base+i*2, 2))
	}
	header.Children[1].Info = map[string]any{"Response": response, "Opcode": uint64((flags >> 11) & 15), "RCODE": uint64(flags & 15)}
	root.Children = append(root.Children, header)
	d := doqDNS{wire: wire, base: base, pos: 12, labels: map[int]bool{}}
	for i, name := range []string{"Questions", "Answers", "Authority", "Additional"} {
		part, err := d.records(name, counts[i], i == 0)
		if err != nil {
			return root, err
		}
		root.Children = append(root.Children, part)
	}
	if d.pos != len(wire) {
		return root, fmt.Errorf("doq: trailing bytes after complete DNS message")
	}
	return root, nil
}

func decodeDoQStream(wire []byte, mode string, response bool) ([]doqField, map[string]any, error) {
	if mode != "rfc9250" && mode != "draft-i00" {
		return nil, nil, fmt.Errorf("doq: explicit known mapping version required")
	}
	if len(wire) == 0 || len(wire) > 1<<20 {
		return nil, nil, fmt.Errorf("doq: clear stream size outside 1..1048576 bytes")
	}
	info := map[string]any{"Mapping": mode, "Response": response,
		"Input":                   "Caller-provided complete decrypted and reassembled stream direction",
		"Transport Preconditions": "Caller verifies client-initiated bidirectional stream identity, FIN, offsets and mapping version",
		"Transport":               "No QUIC parsing, decryption or reassembly performed here",
		"Correlation":             "No query/response or zone-transfer semantic correlation",
		"DNS Profile":             "QUERY header/questions/RR envelopes, compressed names, A and EDNS(0); other RData opaque"}
	if mode == "draft-i00" {
		message, err := decodeDoQDNS(wire, 0, response)
		if err != nil {
			return nil, nil, err
		}
		info["Message Count"] = 1
		return []doqField{message}, info, nil
	}
	fields := []doqField{}
	for pos, count := 0, 0; pos < len(wire); count++ {
		if count >= 256 || !response && count != 0 {
			return nil, nil, fmt.Errorf("doq: stream message count outside selected direction profile")
		}
		if pos+2 > len(wire) {
			return nil, nil, fmt.Errorf("doq: truncated two-octet message length")
		}
		length := int(binary.BigEndian.Uint16(wire[pos:]))
		if length < 12 {
			return nil, nil, fmt.Errorf("doq: RFC 9250 message length must be at least 12 bytes")
		}
		if pos+2+length > len(wire) {
			return nil, nil, fmt.Errorf("doq: incomplete length-prefixed message at stream end")
		}
		entry := doqField{Name: "DoQ Message", Start: pos, End: pos + 2 + length}
		entry.Children = append(entry.Children, doqNumber("Message Length", pos, 2))
		message, err := decodeDoQDNS(wire[pos+2:pos+2+length], pos+2, response)
		if err != nil {
			return nil, nil, err
		}
		entry.Children = append(entry.Children, message)
		fields = append(fields, entry)
		pos = entry.End
	}
	info["Message Count"] = len(fields)
	return fields, info, nil
}
