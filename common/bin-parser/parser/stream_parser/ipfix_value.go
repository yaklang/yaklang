package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net/netip"
)

// IPFIX schemas are not NetFlow v9 schemas: enterprise namespaces, variable
// lengths, scope counts, withdrawals and data-record sequences differ.
type IPFIXField struct {
	Type, Length uint16
	Enterprise   bool
	PEN          uint32
	Scope        bool
}
type IPFIXTemplate struct {
	ID                                 uint16
	Options                            bool
	Quarantined                        bool // TCP redefinition without an observed withdrawal
	Fields                             []IPFIXField
	MinimumLength                      int
	PDUSHA256, DefinitionSHA256        string
	DefinitionOffset, DefinitionLength int
	ExportTime                         uint32
}
type IPFIXResult struct {
	Fields map[string]any
	// Non-nil on a complete syntactically valid message, even when a Data Set
	// lacks a schema. Such data cannot be retrospectively decoded by a later Set.
	Templates map[uint16]IPFIXTemplate
}

func CloneIPFIXTemplates(in map[uint16]IPFIXTemplate) map[uint16]IPFIXTemplate {
	out := make(map[uint16]IPFIXTemplate, len(in))
	for id, t := range in {
		t.Fields = append([]IPFIXField(nil), t.Fields...)
		out[id] = t
	}
	return out
}
func IPFIXWireLength(w []byte) (int, error) {
	if len(w) < 4 {
		return 0, nil
	}
	if binary.BigEndian.Uint16(w) != 10 || binary.BigEndian.Uint16(w[2:]) < 16 {
		return 0, fmt.Errorf("IPFIX: invalid version/message length")
	}
	return int(binary.BigEndian.Uint16(w[2:])), nil
}

// DecodeIPFIXWire decodes exactly one complete message. The receiver must scope
// previous schemas by transport generation, direction and Observation Domain.
// UDP withdrawals are ignored; TCP withdrawals take effect in Set order.
// Unknown IEs retain literal bytes, never a guessed standard/enterprise alias.
func DecodeIPFIXWire(w []byte, previous map[uint16]IPFIXTemplate, transport string, limit int) (out IPFIXResult, err error) {
	if len(w) < 16 || len(w) > 65535 {
		return out, fmt.Errorf("IPFIX: complete bounded header required")
	}
	n, e := IPFIXWireLength(w)
	if e != nil {
		return out, e
	}
	if n != len(w) {
		return out, fmt.Errorf("IPFIX: declared message length differs from complete PDU")
	}
	if transport != "udp" && transport != "tcp" {
		return out, fmt.Errorf("%w: explicit IPFIX transport required", ErrObservedWireContext)
	}
	if limit <= 0 {
		return out, ErrObservedWireBudget
	}
	limit = min(limit, 4096)
	r := &controlReader{w: w, info: map[string]any{"Protocol Name": "IPFIX", "Packet Name": "Flow Export", "Authentication Verified": false, "Observation Scope": "reported exporter records; not captured connections, delivery or authenticated identity", "Wire Framing Complete": false, "Record Count Verified": false, "Known Set Namespace": true}}
	out.Fields = r.info
	r.num("Version", 2, false)
	r.num("Message Length", 2, false)
	export := uint32(r.num("Export Time", 4, false))
	r.num("Sequence Number", 4, false)
	r.num("Observation Domain ID", 4, false)
	r.info["Fixed Header Complete"] = true
	defer func() { r.info["Wire Field Ranges"] = routingFieldRanges(r.fields) }()
	// Admit only complete Set framing, including on unknown-template messages.
	for at := 16; at < len(w); {
		if len(w)-at < 4 {
			return out, fmt.Errorf("IPFIX: partial Set header")
		}
		size := int(binary.BigEndian.Uint16(w[at+2:]))
		if size < 4 || size > len(w)-at {
			return out, fmt.Errorf("IPFIX: Set length exceeds message")
		}
		at += size
	}
	r.info["Wire Framing Complete"] = true
	used := 0
	charge := func(n int) bool {
		if n < 0 || n > limit-used {
			r.err = ErrObservedWireBudget
			return false
		}
		used += n
		return true
	}
	if len(previous) > limit {
		return out, ErrObservedWireBudget
	}
	for id, t := range previous {
		if !charge(len(t.Fields) + 1) {
			return out, r.err
		}
		minimum := 0
		valid := id >= 256 && t.ID == id && len(t.Fields) > 0
		normal := false
		for _, f := range t.Fields {
			valid = valid && (!f.Scope || t.Options && !normal) && (!f.Enterprise && f.PEN == 0 || f.Enterprise)
			normal = normal || !f.Scope
			if f.Length == 65535 {
				minimum++
			} else {
				minimum += int(f.Length)
			}
		}
		if !valid || minimum != t.MinimumLength || minimum == 0 {
			return out, fmt.Errorf("%w: invalid caller IPFIX schema", ErrObservedWireContext)
		}
	}
	working := CloneIPFIXTemplates(previous)
	pduHash := netflowV9Hash(w)
	var sets []map[string]any
	var contextErr error
	count, definitions := 0, 0
	for r.at < len(w) && r.err == nil {
		start := r.at
		end := start + int(binary.BigEndian.Uint16(w[start+2:]))
		if !charge(1) {
			break
		}
		row := r.child("Set", end, func(c *controlReader) {
			id := uint16(c.num("FlowSet ID", 2, false))
			c.num("FlowSet Length", 2, false)
			if id == 2 || id == 3 {
				c.need(len(c.w)-c.at >= 4, "IPFIX Template Set requires at least one record")
				var templates []map[string]any
				for c.at < len(c.w) && c.err == nil {
					if len(c.w)-c.at < 4 {
						b := c.rest("Padding")
						c.need(bytes.Equal(b, make([]byte, len(b))), "IPFIX nonzero template padding")
						break
					}
					if !charge(1) {
						c.err = r.err
						break
					}
					at := c.at
					t := IPFIXTemplate{Options: id == 3, PDUSHA256: pduHash, ExportTime: export, DefinitionOffset: at}
					x := &controlReader{w: c.w, at: at, info: map[string]any{}}
					t.ID = uint16(x.num("Template ID", 2, false))
					nf := int(x.num("Field Count", 2, false))
					if nf == 0 {
						x.need(t.ID >= 256 || t.ID == id, "IPFIX withdrawal ID/kind")
						x.info["Withdrawal"] = true
						if transport == "udp" {
							x.info["Withdrawal Applied"] = false
							x.info["Notice"] = "UDP template withdrawals MUST be ignored"
						} else {
							x.info["Withdrawal Applied"] = true
							if t.ID == id {
								for k, v := range working {
									if v.Options == t.Options {
										delete(working, k)
									}
								}
							} else if old, ok := working[t.ID]; ok && old.Options == t.Options {
								delete(working, t.ID)
							}
						}
					} else {
						x.need(t.ID >= 256, "IPFIX template ID below256")
						scope := 0
						if t.Options {
							scope = int(x.num("Scope Field Count", 2, false))
							x.need(scope > 0 && scope <= nf, "IPFIX options scope count")
						}
						if !charge(nf) {
							x.err = r.err
						}
						var specs []map[string]any
						for i := 0; i < nf && x.err == nil; i++ {
							s := &controlReader{w: x.w, at: x.at, info: map[string]any{}}
							literal := uint16(s.num("Field Type", 2, false))
							f := IPFIXField{Type: literal & 32767, Enterprise: literal&32768 != 0, Scope: i < scope}
							f.Length = uint16(s.num("Field Length", 2, false))
							if f.Enterprise {
								f.PEN = uint32(s.num("Enterprise Number", 4, false))
							}
							s.info["Field Type"] = uint64(f.Type)
							s.info["Enterprise"] = f.Enterprise
							s.info["Scope"] = f.Scope
							s.info["Wire Field Ranges"] = routingFieldRanges(s.fields)
							x.at, x.err = s.at, s.err
							specs = append(specs, s.info)
							t.Fields = append(t.Fields, f)
							if f.Length == 65535 {
								t.MinimumLength++
							} else {
								t.MinimumLength += int(f.Length)
							}
						}
						if t.MinimumLength == 0 && x.err == nil {
							x.err = fmt.Errorf("%w: zero-width IPFIX records have no observable boundaries", ErrObservedWireContext)
						}
						x.info["Fields"] = specs
						x.info["Options"] = t.Options
						if x.err == nil {
							t.DefinitionLength = x.at - at
							t.DefinitionSHA256 = netflowV9Hash(w[at:x.at])
							if old, exists := working[t.ID]; transport == "tcp" && exists && (old.Quarantined || old.DefinitionSHA256 != t.DefinitionSHA256) {
								t.Quarantined = true
								x.info["Template Quarantined"] = true
								contextErr = fmt.Errorf("%w: IPFIX TCP template redefinition without withdrawal; schema quarantined", ErrObservedWireContext)
							}
							working[t.ID] = t
							definitions++
						}
					}
					x.info["Wire Field Ranges"] = routingFieldRanges(x.fields)
					c.at, c.err = x.at, x.err
					templates = append(templates, x.info)
				}
				c.info["Templates"] = templates
			} else if id >= 256 {
				c.need(c.at < len(c.w), "IPFIX Data Set requires at least one record")
				t, ok := working[id]
				if !ok || t.Quarantined || export < t.ExportTime {
					contextErr = fmt.Errorf("%w: IPFIX template %d unavailable at this export time", ErrObservedWireContext, id)
					c.rest("Undecoded Data Set")
					c.info["Template Available"] = false
				} else {
					c.info["Template Available"] = true
					c.info["Template PDU SHA256"] = t.PDUSHA256
					c.info["Template Definition SHA256"] = t.DefinitionSHA256
					c.info["Template Definition Offset"] = uint64(t.DefinitionOffset)
					c.info["Template Definition Length"] = uint64(t.DefinitionLength)
					c.info["Options"] = t.Options
					var records []map[string]any
					for c.at < len(c.w) && c.err == nil {
						if len(c.w)-c.at < t.MinimumLength {
							b := c.rest("Padding")
							c.need(bytes.Equal(b, make([]byte, len(b))), "IPFIX nonzero data padding")
							break
						}
						if !charge(len(t.Fields) + 1) {
							c.err = r.err
							break
						}
						record := map[string]any{"Record Index": uint64(len(records) + 1)}
						var values []map[string]any
						for _, f := range t.Fields {
							s := &controlReader{w: c.w, at: c.at, info: map[string]any{"Field Type": uint64(f.Type), "Declared Length": uint64(f.Length), "Enterprise": f.Enterprise, "Enterprise Number": uint64(f.PEN), "Scope": f.Scope}}
							length := int(f.Length)
							if length == 65535 {
								length = int(s.num("Variable Length", 1, false))
								if length == 255 {
									length = int(s.num("Variable Extended Length", 2, false))
								}
							}
							b := s.take("Value", length)
							if s.err == nil {
								v, kind, name := ipfixValue(f, b)
								s.info["Value"], s.info["Value Kind"], s.info["Field Name"] = v, kind, name
								if kind == "opaque" {
									r.info["Unselected Field Semantics"] = true
								}
							}
							s.info["Wire Field Ranges"] = routingFieldRanges(s.fields)
							c.at, c.err = s.at, s.err
							values = append(values, s.info)
							if s.err != nil {
								break
							}
						}
						if c.err == nil {
							record["Fields"] = values
							records = append(records, record)
							count++
						}
					}
					c.info["Records"] = records
					c.need(len(records) > 0, "IPFIX Data Set contains padding but no complete record")
				}
			} else {
				r.info["Known Set Namespace"] = false
				contextErr = fmt.Errorf("%w: reserved IPFIX Set namespace", ErrObservedWireContext)
				c.rest("Reserved Set Bytes")
			}
			c.info["Wire Field Ranges"] = routingFieldRanges(c.fields)
		})
		if row != nil {
			sets = append(sets, row)
		}
	}
	r.info["FlowSets"] = sets
	r.info["Template Record Count"] = uint64(definitions)
	r.info["Data Record Count"] = uint64(count)
	if r.err != nil {
		return out, r.err
	}
	out.Templates = working
	if contextErr != nil {
		return out, contextErr
	}
	r.info["Record Count Verified"] = true
	return out, nil
}

func ipfixValue(f IPFIXField, b []byte) (any, string, string) {
	// This deliberately bounded standard subset follows IANA IPFIX types.
	// tcpControlBits is16-bit and flowLabelIPv6 is32-bit, unlike v9 layouts.
	name := fmt.Sprintf("Reported IE %d (enterprise=%t PEN=%d scope=%t)", f.Type, f.Enterprise, f.PEN, f.Scope)
	if f.Enterprise {
		return bytes.Clone(b), "opaque", name
	}
	s, ok := netflowV9Values[f.Type]
	width := s.width
	if width == 0 {
		width = 8
	}
	switch f.Type {
	case 6:
		width = 2
	case 10, 14, 16, 17, 31:
		width = 4
	case 152, 153:
		if len(b) == 8 {
			return binary.BigEndian.Uint64(b), "dateTimeMilliseconds", map[bool]string{true: "Reported Flow Start Milliseconds", false: "Reported Flow End Milliseconds"}[f.Type == 152]
		}
		return bytes.Clone(b), "opaque", name
	}
	if !ok {
		return bytes.Clone(b), "opaque", name
	}
	if s.kind == "IPv4" && len(b) == 4 {
		return netip.AddrFrom4([4]byte(b)).String(), "IPv4", s.name
	}
	if s.kind == "IPv6" && len(b) == 16 {
		return netip.AddrFrom16([16]byte(b)).String(), "IPv6", s.name
	}
	if s.kind != "unsigned" && s.kind != "AS" || len(b) == 0 || len(b) > width {
		return bytes.Clone(b), "opaque", name
	}
	var n uint64
	for _, c := range b {
		n = n<<8 | uint64(c)
	}
	return n, "unsigned", s.name
}
