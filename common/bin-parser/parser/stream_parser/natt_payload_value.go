package stream_parser

import (
	"encoding/binary"
	"fmt"
)

// These indices describe the built-in NATTIKEPayload scalar prototype, not a
// public protocol registry. Unknown types retain their generic header and raw
// bytes; no received message is considered valid for an IKE session here.
const (
	nattNextPayload = iota
	nattPayloadFlags
	nattPayloadLength
	nattFragmentNumber
	nattTotalFragments
	nattDomainOfInterpretation
	nattProtocolID
	nattSPISize
	nattNotifyType
	nattNotificationSPI
	nattNotificationData
	nattDHGroup
	nattKEReserved
	nattKeyExchangeData
	nattNonceData
	nattVendorID
	nattEncryptedPayloadData
	nattPayloadData
)

var nattPayloadFieldNames = [...]string{
	"Next Payload", "Payload Flags", "Payload Length", "Fragment Number", "Total Fragments",
	"Domain of Interpretation", "Protocol ID", "SPI Size", "Notify Type", "Notification SPI",
	"Notification Data", "DH Group", "KE Reserved", "Key Exchange Data", "Nonce Data", "Vendor ID",
	"Encrypted Payload Data", "Payload Data",
}

// Offsets are relative to the bounded payload list. The descriptor owns no
// wire data and never exposes a partially validated list on failure.
type nattPayloadRecord struct {
	Offset, Length int
	Type, Next     int
	Flags          int
	DataField      int
	DataOffset     int
	SPILength      int
}

func nattPayloadRecordAt(body []byte, offset, major, typ, index int) (nattPayloadRecord, int, error) {
	r := nattPayloadRecord{Offset: offset, Type: typ, DataField: nattPayloadData, DataOffset: 4}
	if len(body)-offset < 4 {
		return r, 0, fmt.Errorf("nat-t: truncated IKE payload header")
	}
	b := body[offset:]
	r.Next, r.Flags, r.Length = int(b[0]), int(b[1]), int(binary.BigEndian.Uint16(b[2:4]))
	if r.Length < 4 || r.Length > len(b) {
		return r, 0, fmt.Errorf("nat-t: IKE payload length exceeds its boundary or is below four")
	}
	b = b[:r.Length]
	next := r.Next
	switch {
	case major == 2 && (typ == 46 || typ == 53):
		if typ == 53 {
			if r.Length < 9 {
				return r, 0, fmt.Errorf("nat-t: encrypted fragment needs counters and nonempty data")
			}
			fragment, fragments := int(binary.BigEndian.Uint16(b[4:6])), int(binary.BigEndian.Uint16(b[6:8]))
			if fragment == 0 || fragments == 0 || fragment > fragments {
				return r, 0, fmt.Errorf("nat-t: invalid IKE fragment counters")
			}
			if fragment > 1 && next != 0 {
				return r, 0, fmt.Errorf("nat-t: later IKE fragments must have next payload zero")
			}
			if fragment > 1 && index > 0 {
				return r, 0, fmt.Errorf("nat-t: only first IKE fragment can have preceding payloads")
			}
			r.DataOffset = 8
		} else if r.Length == 4 {
			return r, 0, fmt.Errorf("nat-t: encrypted payload is empty")
		}
		r.DataField = nattEncryptedPayloadData
		next = 0
	case major == 2 && typ == 41 || major == 1 && typ == 11:
		r.DataOffset = 8
		if major == 1 {
			r.DataOffset = 12
		}
		if r.Length < r.DataOffset {
			return r, 0, fmt.Errorf("nat-t: truncated notification fields")
		}
		r.SPILength = int(b[r.DataOffset-3])
		if r.SPILength > r.Length-r.DataOffset {
			return r, 0, fmt.Errorf("nat-t: notification SPI exceeds payload")
		}
		r.DataOffset += r.SPILength
		r.DataField = nattNotificationData
	case major == 2 && typ == 34:
		if r.Length < 9 {
			return r, 0, fmt.Errorf("nat-t: key exchange requires group, reserved bytes and data")
		}
		r.DataOffset = 8
		r.DataField = nattKeyExchangeData
	case major == 2 && typ == 40 || major == 1 && typ == 10:
		if major == 2 && (r.Length-4 < 16 || r.Length-4 > 256) {
			return r, 0, fmt.Errorf("nat-t: IKEv2 nonce data must contain 16..256 bytes")
		}
		r.DataField = nattNonceData
	case major == 2 && typ == 43 || major == 1 && typ == 13:
		r.DataField = nattVendorID
	}
	return r, next, nil
}

// Two linear passes bound both work and descriptor allocation. Parsing the
// whole chain before publishing descriptors prevents partial-success output.
func decodeNATTPayloads(body []byte, major, firstType int) ([]nattPayloadRecord, error) {
	if major != 1 && major != 2 {
		return nil, fmt.Errorf("nat-t: IKE major version must be one or two")
	}
	if firstType < 0 || firstType > 255 || len(body) > 65527 {
		return nil, fmt.Errorf("nat-t: invalid payload-list boundary or first type")
	}
	count := 0
	for offset, typ := 0, firstType; ; {
		if offset == len(body) {
			if typ != 0 {
				return nil, fmt.Errorf("nat-t: IKE payload chain ends before its next payload")
			}
			break
		}
		if typ == 0 {
			// RFC 2408 permits only the message's final 1..3 alignment
			// octets here, never an extra payload or IKEv2 trailing bytes.
			if major == 1 && count > 0 && len(body)-offset <= 3 && len(body)%4 == 0 {
				break
			}
			return nil, fmt.Errorf("nat-t: bytes remain after the final IKE payload")
		}
		if count >= 4096 {
			return nil, fmt.Errorf("nat-t: IKE payload count exceeds 4096")
		}
		r, next, err := nattPayloadRecordAt(body, offset, major, typ, count)
		if err != nil {
			return nil, err
		}
		offset += r.Length
		typ = next
		count++
	}
	records := make([]nattPayloadRecord, count)
	for index, offset, typ := 0, 0, firstType; index < count; index++ {
		r, next, err := nattPayloadRecordAt(body, offset, major, typ, index)
		if err != nil {
			return nil, err
		}
		records[index] = r
		offset += r.Length
		typ = next
	}
	return records, nil
}
