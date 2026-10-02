package stream_parser

import "fmt"

// nhrpClientRecord describes a CIE without copying its wire values. All offsets
// are relative to the supplied bounded CIE list, not the enclosing packet.
type nhrpClientRecord struct {
	Offset         int
	AddressLengths [3]int
}

// decodeNHRPClientsBody implements only the built-in NHRPClient field layout.
// Message-specific CIE counts/flags remain the enclosing YAML rule's concern.
// Validate the entire list before allocating descriptors; failures return nil.
func decodeNHRPClientsBody(body []byte) ([]nhrpClientRecord, error) {
	if len(body) > 65535 {
		return nil, fmt.Errorf("nhrp: client list exceeds packet size")
	}
	count := 0
	for offset := 0; offset < len(body); {
		if count >= 4096 {
			return nil, fmt.Errorf("nhrp: too many client entries")
		}
		if len(body)-offset < 12 {
			return nil, fmt.Errorf("nhrp: truncated client header")
		}
		if body[offset+8]&128 != 0 || body[offset+9]&128 != 0 {
			return nil, fmt.Errorf("nhrp: reserved client address type bit")
		}
		length := 12 + int(body[offset+8]&63) + int(body[offset+9]&63) + int(body[offset+10])
		if length > len(body)-offset {
			return nil, fmt.Errorf("nhrp: client address exceeds list boundary")
		}
		offset += length
		count++
	}
	records := make([]nhrpClientRecord, count)
	for index, offset := 0, 0; index < count; index++ {
		lengths := [3]int{int(body[offset+8] & 63), int(body[offset+9] & 63), int(body[offset+10])}
		records[index] = nhrpClientRecord{Offset: offset, AddressLengths: lengths}
		offset += 12 + lengths[0] + lengths[1] + lengths[2]
	}
	return records, nil
}
