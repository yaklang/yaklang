package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
)

// RFC1002 name-service datagrams share a DNS-style carrier but type 0x21 is
// NBSTAT, not DNS SRV. Names, every record boundary, and the entire datagram
// must validate before an NBNS message is accepted. No ownership is inferred.
func decodeNBNS(w []byte, limit int) (map[string]any, error) {
	if len(w) < 12 {
		return nil, fmt.Errorf("nbns: incomplete header")
	}
	if limit <= 0 {
		limit = 4096
	}
	flags := binary.BigEndian.Uint16(w[2:4])
	opcode := (flags >> 11) & 15
	if opcode != 0 && opcode != 5 && opcode != 6 && opcode != 7 && opcode != 8 {
		return nil, fmt.Errorf("nbns: unsupported opcode")
	}
	info := map[string]any{"Transaction ID": binary.BigEndian.Uint16(w), "Flags": flags, "Opcode": opcode, "Response": flags&0x8000 != 0, "RCODE": flags & 15, "Observation": "unverified name service message"}
	pos, count := 12, 0
	for section, key := range []string{"Questions", "Answers", "Authority", "Additional"} {
		n := int(binary.BigEndian.Uint16(w[4+2*section:]))
		count += n
		if count > limit {
			return nil, protocolError(ErrResourceExceeded, "NBNS record budget")
		}
		var rows []map[string]any
		for i := 0; i < n; i++ {
			name, next, err := dnsParseName(w, pos)
			if err != nil {
				return nil, err
			}
			decoded, err := decodeNBNSName(name)
			if err != nil {
				return nil, err
			}
			pos = next
			need := 4
			if section > 0 {
				need = 10
			}
			if pos > len(w) || need > len(w)-pos {
				return nil, fmt.Errorf("nbns: truncated record")
			}
			typ, class := binary.BigEndian.Uint16(w[pos:]), binary.BigEndian.Uint16(w[pos+2:])
			if class != 1 || typ != 0x20 && typ != 0x21 {
				return nil, fmt.Errorf("nbns: unsupported name record type/class")
			}
			decoded["Type"], decoded["Class"] = typ, class
			if section == 0 {
				pos += 4
			} else {
				length := int(binary.BigEndian.Uint16(w[pos+8:]))
				decoded["TTL"] = binary.BigEndian.Uint32(w[pos+4:])
				pos += 10
				if length > len(w)-pos {
					return nil, fmt.Errorf("nbns: RDATA outside datagram")
				}
				data := w[pos : pos+length]
				pos += length
				if opcode == 7 {
					if len(data) != 2 {
						return nil, fmt.Errorf("nbns: WACK RDATA length")
					}
					decoded["Wait Flags"] = binary.BigEndian.Uint16(data)
				} else if typ == 0x20 {
					if len(data) == 0 || len(data)%6 != 0 {
						return nil, fmt.Errorf("nbns: NB address RDATA length")
					}
					if count+len(data)/6 > limit {
						return nil, protocolError(ErrResourceExceeded, "NBNS address budget")
					}
					count += len(data) / 6
					var addrs []map[string]any
					for at := 0; at < len(data); at += 6 {
						addrs = append(addrs, map[string]any{"Flags": binary.BigEndian.Uint16(data[at:]), "Address": net.IP(data[at+2 : at+6]).String()})
					}
					decoded["Addresses"] = addrs
				} else {
					if len(data) < 1 {
						return nil, fmt.Errorf("nbns: NBSTAT name count")
					}
					names := int(data[0])
					if count+names > limit {
						return nil, protocolError(ErrResourceExceeded, "NBSTAT name budget")
					}
					count += names
					if len(data) != 1+18*names+46 {
						return nil, fmt.Errorf("nbns: NBSTAT names/statistics boundary")
					}
					var entries []map[string]any
					for at := 1; at < 1+18*names; at += 18 {
						entries = append(entries, map[string]any{"Name Bytes": bytes.Clone(data[at : at+15]), "Suffix": data[at+15], "Flags": binary.BigEndian.Uint16(data[at+16:])})
					}
					decoded["Node Names"] = entries
					decoded["Statistics"] = bytes.Clone(data[1+18*names:])
				}
			}
			rows = append(rows, decoded)
		}
		info[key] = rows
	}
	if count == 0 || pos != len(w) {
		return nil, fmt.Errorf("nbns: empty message or trailing bytes")
	}
	info["Packet Name"] = "Query"
	if flags&0x8000 != 0 {
		info["Packet Name"] = "Response"
	}
	info["Association"] = "datagram observation; transaction not correlated"
	return info, nil
}

func decodeNBNSName(name string) (map[string]any, error) {
	parts := strings.SplitN(name, ".", 2)
	if len(parts[0]) != 32 {
		return nil, fmt.Errorf("nbns: first-level name must encode 16 octets")
	}
	w := make([]byte, 16)
	for i := range w {
		a, b := parts[0][2*i], parts[0][2*i+1]
		if a < 'A' || a > 'P' || b < 'A' || b > 'P' {
			return nil, fmt.Errorf("nbns: invalid first-level name alphabet")
		}
		w[i] = (a-'A')<<4 | (b - 'A')
	}
	display := string(bytes.TrimRight(w[:15], " \x00"))
	for _, b := range []byte(display) {
		if b < 32 || b > 126 {
			display = fmt.Sprintf("OEM bytes %x", w[:15])
			break
		}
	}
	out := map[string]any{"Name": display, "Name Bytes": w, "Suffix": w[15]}
	if len(parts) > 1 {
		out["Scope"] = parts[1]
	}
	return out, nil
}
