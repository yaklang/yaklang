package stream_parser

import (
	"encoding/binary"
	"fmt"
)

// EtherSBusMessage describes a datagram; no operation is executed. A response
// does not carry a command or count. Those fields are interpreted only when
// the caller supplies the original request from the same peer exchange.
type EtherSBusMessage struct {
	Length                 uint32
	Version                uint8
	Protocol               uint8
	Sequence               uint16
	Attribute              uint8
	Destination            uint8
	Command                uint8
	CommandSource          string // wire, request, or empty
	BodyKind               string
	BodyDecoded            bool
	RequestContextRequired bool
	EncodedCount           uint8
	Count                  uint16
	Address                uint32
	Words                  []uint32
	Bytes                  []byte
	CPUType                string
	FirmwareVersion        string
	FirmwareSuffix         uint8
	CPUStatus              uint8
	ACKCode                uint16
	Checksum               uint16
}

func etherSBusCRC(data []byte) uint16 {
	var crc uint16
	for _, octet := range data {
		crc ^= uint16(octet) << 8
		for bit := 0; bit < 8; bit++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// decodeEtherSBusDatagram validates a complete version-0/1, protocol-0 UDP
// datagram. Unknown commands retain their bodies explicitly as opaque bytes.
// Request correlation is caller-owned: select by both endpoints, sequence and
// exchange lifetime, never by sequence alone across conversations. The helper
// also validates the supplied request's framing, CRC and sequence. It has no
// mutable session state, network access or cross-message cache.
func decodeEtherSBusDatagram(wire, request []byte) (*EtherSBusMessage, error) {
	if len(wire) < 12 || len(wire) > 65507 {
		return nil, fmt.Errorf("ether-s-bus: invalid datagram size")
	}
	m := &EtherSBusMessage{
		Length: binary.BigEndian.Uint32(wire), Version: wire[4], Protocol: wire[5],
		Sequence: binary.BigEndian.Uint16(wire[6:]), Attribute: wire[8],
		Checksum: binary.BigEndian.Uint16(wire[len(wire)-2:]),
	}
	if uint64(m.Length) != uint64(len(wire)) {
		return nil, fmt.Errorf("ether-s-bus: declared length differs from datagram boundary")
	}
	if m.Version > 1 || m.Protocol != 0 {
		return nil, fmt.Errorf("ether-s-bus: unsupported version or protocol type")
	}
	if m.Attribute > 2 {
		return nil, fmt.Errorf("ether-s-bus: invalid attribute")
	}
	if etherSBusCRC(wire[:len(wire)-2]) != m.Checksum {
		return nil, fmt.Errorf("ether-s-bus: CRC mismatch")
	}
	body := wire[9 : len(wire)-2]
	if m.Attribute == 0 {
		if len(request) != 0 {
			return nil, fmt.Errorf("ether-s-bus: response context supplied to a request")
		}
		if len(body) < 2 {
			return nil, fmt.Errorf("ether-s-bus: truncated request header")
		}
		m.Destination, m.Command, m.CommandSource = body[0], body[1], "wire"
		if err := etherSBusRequestBody(m, body[2:]); err != nil {
			return nil, err
		}
		return m, nil
	}
	var req *EtherSBusMessage
	if len(request) != 0 {
		var err error
		req, err = decodeEtherSBusDatagram(request, nil)
		if err != nil {
			return nil, fmt.Errorf("ether-s-bus: invalid request context: %w", err)
		}
		if req.Attribute != 0 || req.Sequence != m.Sequence {
			return nil, fmt.Errorf("ether-s-bus: request context attribute or sequence mismatch")
		}
		m.Command, m.CommandSource = req.Command, "request"
	}
	if m.Attribute == 2 {
		if len(body) != 2 {
			return nil, fmt.Errorf("ether-s-bus: ACK/NAK body must be two bytes")
		}
		m.ACKCode, m.BodyKind, m.BodyDecoded = binary.BigEndian.Uint16(body), "ack-nak", true
		return m, nil
	}
	if req == nil {
		m.RequestContextRequired = true
		m.BodyKind = "opaque-response"
		m.Bytes = append([]byte(nil), body...)
		return m, nil
	}
	if err := etherSBusResponseBody(m, req, body); err != nil {
		return nil, err
	}
	return m, nil
}

func etherSBusRequestBody(m *EtherSBusMessage, body []byte) error {
	m.BodyDecoded = true
	switch {
	case m.Command == 0x20 || m.Command >= 0x14 && m.Command <= 0x1b:
		if len(body) != 0 {
			return fmt.Errorf("ether-s-bus: unexpected arguments for version/status request")
		}
		m.BodyKind = "version-request"
		if m.Command != 0x20 {
			m.BodyKind = "status-request"
		}
	case m.Command == 0x1e || m.Command == 0x1f || m.Command == 0x47:
		if len(body) != 4 {
			return fmt.Errorf("ether-s-bus: read request requires count and 24-bit address")
		}
		m.EncodedCount, m.Count = body[0], uint16(body[0])+1
		m.Address = uint32(body[1])<<16 | uint32(body[2])<<8 | uint32(body[3])
		m.BodyKind = "word-read-request"
		if m.Command == 0x47 {
			m.BodyKind = "byte-read-request"
		}
	case m.Command == 0x51:
		if len(body) < 5 || body[0] < 3 || len(body) != int(body[0])+2 {
			return fmt.Errorf("ether-s-bus: byte-write count does not match body")
		}
		m.EncodedCount, m.Count = body[0], uint16(body[0])-2
		m.Address = uint32(body[1])<<16 | uint32(body[2])<<8 | uint32(body[3])
		m.Bytes = append([]byte(nil), body[4:]...)
		m.BodyKind = "byte-write-request"
	default:
		m.BodyDecoded, m.BodyKind = false, "opaque-request"
		m.Bytes = append([]byte(nil), body...)
	}
	return nil
}

func etherSBusResponseBody(m, req *EtherSBusMessage, body []byte) error {
	m.BodyDecoded = true
	switch {
	case req.Command == 0x20:
		if len(body) != 9 {
			return fmt.Errorf("ether-s-bus: firmware response must contain nine bytes")
		}
		m.BodyKind = "version-response"
		m.CPUType, m.FirmwareVersion, m.FirmwareSuffix = string(body[:5]), string(body[5:8]), body[8]
	case req.Command >= 0x14 && req.Command <= 0x1b:
		if len(body) != 1 {
			return fmt.Errorf("ether-s-bus: status response must contain one byte")
		}
		m.BodyKind, m.CPUStatus = "status-response", body[0]
	case req.Command == 0x1e || req.Command == 0x1f:
		if len(body) != int(req.Count)*4 {
			return fmt.Errorf("ether-s-bus: word response length differs from request count")
		}
		m.BodyKind, m.Count, m.Address = "word-read-response", req.Count, req.Address
		m.Words = make([]uint32, int(req.Count))
		for i := range m.Words {
			m.Words[i] = binary.BigEndian.Uint32(body[i*4:])
		}
	case req.Command == 0x47:
		if len(body) != int(req.Count) {
			return fmt.Errorf("ether-s-bus: byte response length differs from request count")
		}
		m.BodyKind, m.Count, m.Address = "byte-read-response", req.Count, req.Address
		m.Bytes = append([]byte(nil), body...)
	case req.Command == 0x51:
		return fmt.Errorf("ether-s-bus: byte-write response requires ACK/NAK attribute")
	default:
		m.BodyDecoded, m.BodyKind = false, "opaque-response"
		m.Bytes = append([]byte(nil), body...)
	}
	return nil
}
