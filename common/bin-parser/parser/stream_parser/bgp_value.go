package stream_parser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
)

var ErrObservedWireBudget = errors.New("observed wire collection budget")
var ErrObservedWireContext = errors.New("observed wire context required")

// BGPWireContext contains observed directional negotiation, not router state.
// ASWidth 0 and AddPath -1 deliberately preserve ambiguous encodings as raw.
type BGPWireContext struct {
	ASWidth int
	AddPath func(afi uint16, safi byte) int
}

func BGPWireLength(w []byte) (int, error) {
	for _, v := range w[:min(16, len(w))] {
		if v != 255 {
			return 0, fmt.Errorf("bgp: marker is not sixteen FF octets")
		}
	}
	if len(w) < 19 {
		return 0, nil
	}
	n := int(binary.BigEndian.Uint16(w[16:18]))
	typ := w[18]
	if n < 19 || typ < 1 || typ > 5 {
		return 0, fmt.Errorf("bgp: invalid length/type")
	}
	if typ == 1 && (n < 29 || n > 4096) || typ == 2 && n < 23 || typ == 3 && n < 21 || typ == 4 && n != 19 || typ == 5 && n != 23 {
		return 0, fmt.Errorf("bgp: message-specific length")
	}
	return n, nil
}

func DecodeBGPMessage(w []byte, ctx BGPWireContext, max int) (map[string]any, error) {
	n, err := BGPWireLength(w)
	if err != nil {
		return nil, err
	}
	if n == 0 || n != len(w) {
		return nil, fmt.Errorf("bgp: complete exact message boundary required")
	}
	if max <= 0 {
		return nil, ErrObservedWireBudget
	}
	typ := w[18]
	out := map[string]any{"Length": n, "Type": typ, "Packet Name": map[byte]string{1: "OPEN", 2: "UPDATE", 3: "NOTIFICATION", 4: "KEEPALIVE", 5: "ROUTE-REFRESH"}[typ], "Authentication Verified": false}
	ranges := map[string][]int{"Length": {16, 18}, "Type": {18, 19}}
	out["Wire Field Ranges"] = ranges
	b := w[19:]
	used := 0
	spend := func() error {
		used++
		if used > max {
			return ErrObservedWireBudget
		}
		return nil
	}
	switch typ {
	case 1:
		if b[0] != 4 {
			return out, fmt.Errorf("bgp: OPEN version is not 4")
		}
		hold := binary.BigEndian.Uint16(b[3:5])
		if hold == 1 || hold == 2 {
			return out, fmt.Errorf("bgp: OPEN hold time must be zero or at least three")
		}
		out["Version"], out["My AS"], out["Hold Time"], out["BGP Identifier"] = b[0], binary.BigEndian.Uint16(b[1:3]), hold, netip.AddrFrom4([4]byte(b[5:9])).String()
		ranges["Version"], ranges["My AS"], ranges["Hold Time"], ranges["BGP Identifier"] = []int{19, 20}, []int{20, 22}, []int{22, 24}, []int{24, 28}
		at, optional := 10, int(b[9])
		out["Optional Length"] = optional
		// RFC9072: FF optional length is followed by type FF and a 16-bit
		// extended optional-parameters length. Individual TLVs remain octets.
		if optional == 255 {
			if len(b) < 13 || b[10] != 255 {
				return out, fmt.Errorf("bgp: extended OPEN optional length header")
			}
			optional = int(binary.BigEndian.Uint16(b[11:13]))
			at = 13
			out["Optional Length"] = optional
			out["Extended Optional Length"] = true
		}
		if optional != len(b)-at {
			return out, fmt.Errorf("bgp: optional parameters length mismatch")
		}
		var params, caps []map[string]any
		for at < len(b) {
			if err = spend(); err != nil {
				return out, err
			}
			if len(b)-at < 2 || int(b[at+1]) > len(b)-at-2 {
				return out, fmt.Errorf("bgp: optional parameter exceeds OPEN")
			}
			t, n := b[at], int(b[at+1])
			v := b[at+2 : at+2+n]
			at += 2 + n
			p := map[string]any{"Type": t, "Length": n, "Value": bytes.Clone(v)}
			params = append(params, p)
			if t != 2 {
				continue
			}
			for len(v) > 0 {
				if err = spend(); err != nil {
					return out, err
				}
				if len(v) < 2 || int(v[1]) > len(v)-2 {
					return out, fmt.Errorf("bgp: capability TLV exceeds parameter")
				}
				code, sz := v[0], int(v[1])
				val := v[2 : 2+sz]
				v = v[2+sz:]
				c := map[string]any{"Code": code, "Length": sz, "Value": bytes.Clone(val)}
				switch code {
				case 1:
					if sz != 4 {
						return out, fmt.Errorf("bgp: multiprotocol capability length")
					}
					c["AFI"], c["Reserved"], c["SAFI"] = binary.BigEndian.Uint16(val), val[2], val[3]
				case 2, 6, 70:
					if sz != 0 {
						return out, fmt.Errorf("bgp: flag capability has data")
					}
				case 65:
					if sz != 4 {
						return out, fmt.Errorf("bgp: four-octet AS capability length")
					}
					c["ASN"] = binary.BigEndian.Uint32(val)
				case 69:
					if sz == 0 || sz%4 != 0 {
						return out, fmt.Errorf("bgp: ADD-PATH capability length")
					}
					var rows []map[string]any
					for p := 0; p < sz; p += 4 {
						if err = spend(); err != nil {
							return out, err
						}
						if val[p+3] < 1 || val[p+3] > 3 {
							return out, fmt.Errorf("bgp: ADD-PATH send/receive mode")
						}
						rows = append(rows, map[string]any{"AFI": binary.BigEndian.Uint16(val[p:]), "SAFI": val[p+2], "Mode": val[p+3]})
					}
					c["Families"] = rows
				case 64:
					if sz < 2 || (sz-2)%4 != 0 {
						return out, fmt.Errorf("bgp: graceful-restart capability length")
					}
					c["Restart Flags and Time"] = binary.BigEndian.Uint16(val)
				}
				caps = append(caps, c)
			}
		}
		out["Optional Parameters"], out["Capabilities"] = params, caps
	case 2:
		wl := int(binary.BigEndian.Uint16(b))
		if wl > len(b)-4 {
			return out, fmt.Errorf("bgp: withdrawn routes exceed UPDATE")
		}
		at := 2 + wl
		al := int(binary.BigEndian.Uint16(b[at:]))
		at += 2
		if al > len(b)-at {
			return out, fmt.Errorf("bgp: attributes exceed UPDATE")
		}
		out["Withdrawn Length"], out["Path Attributes Length"] = wl, al
		ranges["Withdrawn Length"] = []int{19, 21}
		ranges["Path Attributes Length"] = []int{19 + 2 + wl, 19 + 4 + wl}
		decodePrefixes := func(v []byte, afi uint16, safi byte) (map[string]any, error) {
			mode := -1
			if ctx.AddPath != nil {
				mode = ctx.AddPath(afi, safi)
			}
			return decodeBGPPrefixes(v, afi, safi, mode, max, &used)
		}
		withdrawn, e := decodePrefixes(b[2:2+wl], 1, 1)
		if e != nil {
			return out, e
		}
		out["Withdrawn Routes"] = withdrawn
		attrs := b[at : at+al]
		var rows []map[string]any
		seen := map[byte]bool{}
		for off := 0; off < len(attrs); {
			if err = spend(); err != nil {
				return out, err
			}
			if len(attrs)-off < 3 {
				return out, fmt.Errorf("bgp: truncated attribute header")
			}
			start := off
			flags, code := attrs[off], attrs[off+1]
			off += 2
			sz := int(attrs[off])
			off++
			if flags&16 != 0 {
				if len(attrs)-off < 1 {
					return out, fmt.Errorf("bgp: truncated extended attribute length")
				}
				sz = int(binary.BigEndian.Uint16(attrs[off-1 : off+1]))
				off++
			}
			if sz > len(attrs)-off {
				return out, fmt.Errorf("bgp: attribute body exceeds UPDATE")
			}
			v := attrs[off : off+sz]
			off += sz
			a := map[string]any{"Flags": flags, "Type": code, "Length": sz, "Raw": bytes.Clone(v), "Byte Range": []int{19 + at + start, 19 + at + off}}
			rows = append(rows, a)
			if seen[code] {
				return out, fmt.Errorf("bgp: duplicate path attribute %d", code)
			}
			seen[code] = true
			if flags&15 != 0 {
				a["Reserved Flags"] = flags & 15
			} // retain wire observation; routing acceptance is separate
			switch code {
			case 1:
				if sz != 1 || v[0] > 2 {
					return out, fmt.Errorf("bgp: ORIGIN length/value")
				}
				a["Origin"] = v[0]
			case 2, 17:
				width := ctx.ASWidth
				if code == 17 {
					width = 4
				}
				if sz == 0 {
					a["Segments"] = []map[string]any{}
				} else if width == 2 || width == 4 {
					segments, e := decodeBGPASPath(v, width, max, &used)
					if e != nil {
						return out, e
					}
					a["AS Width"], a["Segments"] = width, segments
				} else {
					a["Context Required"] = "AS_PATH width requires observed OPEN capabilities"
				}
			case 3:
				if sz != 4 {
					return out, fmt.Errorf("bgp: NEXT_HOP length")
				}
				a["Next Hop"] = netip.AddrFrom4([4]byte(v)).String()
			case 4, 5:
				if sz != 4 {
					return out, fmt.Errorf("bgp: metric/local preference length")
				}
				key := "MED"
				if code == 5 {
					key = "Local Preference"
				}
				a[key] = binary.BigEndian.Uint32(v)
			case 6:
				if sz != 0 {
					return out, fmt.Errorf("bgp: ATOMIC_AGGREGATE length")
				}
			case 7, 18:
				width := ctx.ASWidth
				if code == 18 {
					width = 4
				}
				if width == 0 {
					a["Context Required"] = "AGGREGATOR AS width requires OPEN context"
				} else {
					if sz != width+4 {
						return out, fmt.Errorf("bgp: AGGREGATOR length")
					}
					asn := uint32(binary.BigEndian.Uint16(v))
					if width == 4 {
						asn = binary.BigEndian.Uint32(v)
					}
					a["ASN"], a["Identifier"] = asn, netip.AddrFrom4([4]byte(v[width:])).String()
				}
			case 8, 16, 32:
				unit := 4
				if code == 16 {
					unit = 8
				}
				if code == 32 {
					unit = 12
				}
				if sz%unit != 0 {
					return out, fmt.Errorf("bgp: community value length")
				}
				var values []map[string]any
				for p := 0; p < sz; p += unit {
					if err = spend(); err != nil {
						return out, err
					}
					row := map[string]any{"Raw": bytes.Clone(v[p : p+unit])}
					if unit == 4 {
						row["Value"] = binary.BigEndian.Uint32(v[p:])
					}
					if unit == 12 {
						row["Global Administrator"], row["Local Data 1"], row["Local Data 2"] = binary.BigEndian.Uint32(v[p:]), binary.BigEndian.Uint32(v[p+4:]), binary.BigEndian.Uint32(v[p+8:])
					}
					values = append(values, row)
				}
				a["Communities"] = values
			case 14, 15:
				if sz < 3 {
					return out, fmt.Errorf("bgp: MP NLRI header")
				}
				afi, safi := binary.BigEndian.Uint16(v), v[2]
				a["AFI"], a["SAFI"] = afi, safi
				p := 3
				if code == 14 {
					if sz < 5 {
						return out, fmt.Errorf("bgp: MP_REACH next hop header")
					}
					nh := int(v[3])
					p = 4 + nh
					if p >= sz {
						return out, fmt.Errorf("bgp: MP_REACH next hop length")
					}
					a["Next Hop Raw"] = bytes.Clone(v[4:p])
					a["Reserved"] = v[p]
					p++
					var hops []string
					if afi == 2 && (nh == 16 || nh == 32) {
						for h := 4; h < 4+nh; h += 16 {
							hops = append(hops, netip.AddrFrom16([16]byte(v[h:h+16])).String())
						}
					} else if afi == 1 && nh == 4 {
						hops = append(hops, netip.AddrFrom4([4]byte(v[4:8])).String())
					}
					if len(hops) > 0 {
						a["Next Hops"] = hops
					}
				}
				prefixes, e := decodePrefixes(v[p:], afi, safi)
				if e != nil {
					return out, e
				}
				a["NLRI"] = prefixes
			}
		}
		out["Path Attributes"] = rows
		nlri, e := decodePrefixes(b[at+al:], 1, 1)
		if e != nil {
			return out, e
		}
		out["NLRI"] = nlri
	case 3:
		out["Error Code"], out["Error Subcode"], out["Data"] = b[0], b[1], bytes.Clone(b[2:])
		ranges["Error Code"], ranges["Error Subcode"] = []int{19, 20}, []int{20, 21}
	case 5:
		out["AFI"], out["Subtype"], out["SAFI"] = binary.BigEndian.Uint16(b), b[2], b[3]
		if b[2] > 2 {
			return out, fmt.Errorf("bgp: ROUTE-REFRESH subtype")
		}
	}
	return out, nil
}

func decodeBGPASPath(w []byte, width, max int, used *int) ([]map[string]any, error) {
	var rows []map[string]any
	for len(w) > 0 {
		if len(w) < 2 || w[0] < 1 || w[0] > 4 || w[1] == 0 {
			return nil, fmt.Errorf("bgp: AS_PATH segment header")
		}
		typ, n := w[0], int(w[1])
		w = w[2:]
		if n > (len(w) / width) {
			return nil, fmt.Errorf("bgp: AS_PATH segment length")
		}
		*used += n + 1
		if *used > max {
			return nil, ErrObservedWireBudget
		}
		asns := make([]uint32, n)
		for i := range asns {
			if width == 2 {
				asns[i] = uint32(binary.BigEndian.Uint16(w))
			} else {
				asns[i] = binary.BigEndian.Uint32(w)
			}
			w = w[width:]
		}
		rows = append(rows, map[string]any{"Type": typ, "Count": n, "ASNs": asns})
	}
	return rows, nil
}

func decodeBGPPrefixes(w []byte, afi uint16, safi byte, mode, max int, used *int) (map[string]any, error) {
	out := map[string]any{"AFI": afi, "SAFI": safi}
	if (afi != 1 && afi != 2) || (safi != 1 && safi != 2) {
		out["Raw"] = bytes.Clone(w)
		out["Context Required"] = "unselected AFI/SAFI NLRI profile"
		return out, nil
	}
	parse := func(add bool) ([]map[string]any, error) {
		var rows []map[string]any
		v := w
		for len(v) > 0 {
			if len(rows) >= max-*used {
				return nil, ErrObservedWireBudget
			}
			r := map[string]any{}
			if add {
				if len(v) < 5 {
					return nil, fmt.Errorf("bgp: ADD-PATH prefix header")
				}
				r["Path ID"] = binary.BigEndian.Uint32(v)
				v = v[4:]
			}
			bits := int(v[0])
			v = v[1:]
			bound := 32
			if afi == 2 {
				bound = 128
			}
			n := (bits + 7) / 8
			if bits > bound || n > len(v) {
				return nil, fmt.Errorf("bgp: NLRI prefix boundary")
			}
			r["Prefix Length"] = bits
			r["Raw"] = bytes.Clone(v[:n])
			if afi == 1 {
				var ip [4]byte
				copy(ip[:], v[:n])
				r["Prefix"] = netip.PrefixFrom(netip.AddrFrom4(ip), bits).Masked().String()
			} else {
				var ip [16]byte
				copy(ip[:], v[:n])
				r["Prefix"] = netip.PrefixFrom(netip.AddrFrom16(ip), bits).Masked().String()
			}
			v = v[n:]
			rows = append(rows, r)
		}
		return rows, nil
	}
	var rows []map[string]any
	var err error
	if mode < 0 && len(w) > 0 {
		a, ea := parse(false)
		b, eb := parse(true)
		if errors.Is(ea, ErrObservedWireBudget) || errors.Is(eb, ErrObservedWireBudget) {
			return out, ErrObservedWireBudget
		}
		if ea == nil && eb == nil {
			out["Raw"] = bytes.Clone(w)
			out["Context Required"] = "ambiguous ADD-PATH encoding without observed capabilities"
			return out, nil
		}
		if ea != nil && eb != nil {
			return out, fmt.Errorf("bgp: no complete NLRI encoding")
		}
		rows = a
		if ea != nil {
			rows = b
			mode = 1
		} else {
			mode = 0
		}
		out["Encoding Evidence"] = "structurally unique; capability negotiation unavailable"
	} else {
		rows, err = parse(mode == 1)
		if err != nil {
			return out, err
		}
	}
	*used += len(rows)
	out["Prefixes"], out["Add Path"] = rows, mode == 1
	return out, nil
}
