package parser

import "encoding/binary"

// captureTLSClientHello projects the pinned tls_hello.yaml entry. Admit only
// complete, unambiguous boundaries; the original rule handles truncated or
// overlapping lengths, failed imports, and other handshake types. Unknown
// extension bodies remain the rule's string-valued Octets, not omitted fields.
func captureTLSClientHello(w []byte) (map[string]any, bool) {
	if len(w) < 4 || w[0] != 1 {
		return nil, false
	}
	n := int(w[1])<<16 | int(w[2])<<8 | int(w[3])
	if n != len(w)-4 || n < 38 {
		return nil, false
	}
	data := w[4:]
	fields := make(map[string]any, 10)
	fields["Legacy Version"] = binary.BigEndian.Uint16(data)
	fields["Random"] = append([]byte(nil), data[2:34]...)
	fields["Session ID Length"] = data[34]
	p := 35
	sid := int(data[34])
	if sid > 32 || sid > len(data)-p {
		return nil, false
	}
	if sid > 0 {
		fields["Session ID"] = string(data[p : p+sid])
		p += sid
	}
	if len(data)-p < 2 {
		return nil, false
	}
	cl := int(binary.BigEndian.Uint16(data[p:]))
	p += 2
	fields["Cipher Suites Length"] = uint16(cl)
	if cl > 512 || cl%2 != 0 || cl > len(data)-p {
		return nil, false
	}
	if cl > 0 {
		list := make([]any, cl/2)
		for i := range list {
			list[i] = map[string]any{"Suite": binary.BigEndian.Uint16(data[p+2*i:])}
		}
		fields["Cipher Suites"] = list
		p += cl
	}
	if p == len(data) {
		return nil, false
	}
	compression := int(data[p])
	p++
	fields["Compression Length"] = uint8(compression)
	if compression > 16 || compression > len(data)-p {
		return nil, false
	}
	if compression > 0 {
		list := make([]any, compression)
		for i := range list {
			list[i] = map[string]any{"Method": data[p+i]}
		}
		fields["Compression Methods"] = list
		p += compression
	}
	if p < len(data) {
		if len(data)-p < 2 {
			return nil, false
		}
		el := int(binary.BigEndian.Uint16(data[p:]))
		p += 2
		fields["Extensions Length"] = uint16(el)
		if el != len(data)-p {
			return nil, false
		}
		// Avoid repeated growth for common ClientHello extension lists, without
		// reserving a large array for a single large opaque extension.
		extensions := make([]any, 0, min(el/4, 16))
		for p < len(data) {
			if len(data)-p < 4 {
				return nil, false
			}
			typ, l := binary.BigEndian.Uint16(data[p:]), int(binary.BigEndian.Uint16(data[p+2:]))
			p += 4
			if l > len(data)-p {
				return nil, false
			}
			ext := map[string]any{"Type": typ, "Length": uint16(l)}
			if typ == 0 && l > 0 {
				if l < 5 {
					return nil, false
				}
				nl := int(binary.BigEndian.Uint16(data[p+3:]))
				if nl != l-5 {
					return nil, false
				}
				sni := map[string]any{"List Length": binary.BigEndian.Uint16(data[p:]), "Name Type": data[p+2], "Name Length": uint16(nl)}
				if nl > 0 {
					sni["Host Name"] = string(data[p+5 : p+l])
				}
				ext["SNI"] = sni
			} else if l > 0 {
				ext["Octets"] = string(data[p : p+l])
			}
			extensions = append(extensions, ext)
			p += l
		}
		if len(extensions) > 0 {
			fields["Extensions"] = extensions
		}
	}
	return map[string]any{"Handshake Type": uint8(1), "Length": uint32(n), "ClientHello": fields}, true
}
