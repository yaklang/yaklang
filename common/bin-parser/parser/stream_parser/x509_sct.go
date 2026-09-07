package stream_parser

import (
	"encoding/binary"
	"fmt"
)

// RFC 6962 sections 3.2/3.3: DER OCTET STRING wrapping a TLS vector.
// Version 0 has a known layout; later versions retain opaque entry bytes.
// Neither timestamps nor log identifiers establish inclusion or validity.
func (r *x509DERReader) sctList(n *x509DERElement, info map[string]any) error {
	if n.tag != 4 || n.end-n.content < 3 {
		return x509ExtensionError()
	}
	start, end := n.content, n.end
	if int(binary.BigEndian.Uint16(r.wire[start:start+2])) != end-start-2 {
		return fmt.Errorf("SCT list length differs from exact boundary")
	}
	fields := []tlsCertificateField{tlsCertificateLeaf("SCT List Length", "uint16", start, start+2)}
	list := tlsCertificateField{Name: "SCT Entries", Start: start + 2, End: end, List: true}
	known, opaque := 0, 0
	for at := start + 2; at < end; {
		if len(list.Children) >= 1024 || r.nodes+r.payloadNodes+12 > x509DERMaxNodes {
			return fmt.Errorf("SCT entry/node resource limit exceeded")
		}
		r.payloadNodes += 12
		if end-at < 3 {
			return fmt.Errorf("truncated SCT entry length or version")
		}
		length := int(binary.BigEndian.Uint16(r.wire[at : at+2]))
		if length < 1 || length > end-at-2 {
			return fmt.Errorf("SCT entry exceeds enclosing list")
		}
		limit := at + 2 + length
		body := at + 2
		entry := tlsCertificateField{Name: "SCT", Start: at, End: limit, Children: []tlsCertificateField{
			tlsCertificateLeaf("SCT Length", "uint16", at, body), tlsCertificateLeaf("SCT Version", "uint8", body, body+1),
		}}
		if r.wire[body] != 0 {
			entry.Children = append(entry.Children, tlsCertificateLeaf("Unknown SCT Version Data", "raw", body+1, limit))
			opaque++
		} else {
			if limit-body < 47 {
				return fmt.Errorf("truncated v1 SCT")
			}
			entry.Children = append(entry.Children,
				tlsCertificateLeaf("SCT Log ID", "raw", body+1, body+33),
				tlsCertificateLeaf("SCT Timestamp Milliseconds", "uint64", body+33, body+41),
				tlsCertificateLeaf("SCT Extensions Length", "uint16", body+41, body+43))
			extensions := int(binary.BigEndian.Uint16(r.wire[body+41 : body+43]))
			position := body + 43
			if extensions > limit-position-4 {
				return fmt.Errorf("SCT extensions exceed entry")
			}
			entry.Children = append(entry.Children, tlsCertificateLeaf("SCT Extensions", "raw", position, position+extensions))
			position += extensions
			entry.Children = append(entry.Children, tlsCertificateLeaf("SCT Hash Algorithm", "uint8", position, position+1), tlsCertificateLeaf("SCT Signature Algorithm", "uint8", position+1, position+2), tlsCertificateLeaf("SCT Signature Length", "uint16", position+2, position+4))
			signature := int(binary.BigEndian.Uint16(r.wire[position+2 : position+4]))
			position += 4
			if signature != limit-position {
				return fmt.Errorf("SCT signature length differs from entry boundary")
			}
			entry.Children = append(entry.Children, tlsCertificateLeaf("SCT Signature", "raw", position, limit))
			known++
		}
		list.Children = append(list.Children, entry)
		at = limit
	}
	if len(list.Children) == 0 {
		return fmt.Errorf("empty SCT list")
	}
	n.payloadFields = append(fields, list)
	info["SCT Count"], info["SCT V1 Count"], info["Opaque SCT Count"] = len(list.Children), known, opaque
	info["Log Inclusion Validated"], info["SCT Signatures Verified"] = false, false
	return nil
}
