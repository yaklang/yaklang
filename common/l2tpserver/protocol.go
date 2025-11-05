package l2tpserver

import (
	"encoding/binary"
	"fmt"
)

// L2TP Control Message Types (RFC 2661)
const (
	SCCRQ   uint16 = 1  // Start-Control-Connection-Request
	SCCRP   uint16 = 2  // Start-Control-Connection-Reply
	SCCCN   uint16 = 3  // Start-Control-Connection-Connected
	StopCCN uint16 = 4  // Stop-Control-Connection-Notification
	Hello   uint16 = 6  // Hello
	OCRQ    uint16 = 7  // Outgoing-Call-Request
	OCRP    uint16 = 8  // Outgoing-Call-Reply
	OCCN    uint16 = 9  // Outgoing-Call-Connected
	ICRQ    uint16 = 10 // Incoming-Call-Request
	ICRP    uint16 = 11 // Incoming-Call-Reply
	ICCN    uint16 = 12 // Incoming-Call-Connected
	CDN     uint16 = 14 // Call-Disconnect-Notify
)

// L2TP AVP Types (Attribute-Value Pairs)
const (
	AVPMessageType         uint16 = 0
	AVPResultCode          uint16 = 1
	AVPProtocolVersion     uint16 = 2
	AVPFramingCapabilities uint16 = 3
	AVPBearerCapabilities  uint16 = 4
	AVPTieBreaker          uint16 = 5
	AVPFirmwareRevision    uint16 = 6
	AVPHostName            uint16 = 7
	AVPVendorName          uint16 = 8
	AVPAssignedTunnelID    uint16 = 9
	AVPReceiveWindowSize   uint16 = 10
	AVPChallenge           uint16 = 11
	AVPCauseCode           uint16 = 12
	AVPChallengeResponse   uint16 = 13
	AVPAssignedSessionID   uint16 = 14
	AVPCallSerialNumber    uint16 = 15
	AVPMinimumBPS          uint16 = 16
	AVPMaximumBPS          uint16 = 17
	AVPBearerType          uint16 = 18
	AVPFramingType         uint16 = 19
	AVPCalledNumber        uint16 = 20
	AVPCallingNumber       uint16 = 21
	AVPSubAddress          uint16 = 22
	AVPTxConnectSpeed      uint16 = 24
	AVPPhysicalChannelID   uint16 = 25
	AVPPrivateGroupID      uint16 = 37
	AVPSequencingRequired  uint16 = 39
)

// L2TP Header Flags
const (
	FlagType     uint16 = 0x8000 // Type bit (0 = data, 1 = control)
	FlagLength   uint16 = 0x4000 // Length bit
	FlagSequence uint16 = 0x0800 // Sequence bit
	FlagOffset   uint16 = 0x0200 // Offset bit
	FlagPriority uint16 = 0x0100 // Priority bit
	VersionMask  uint16 = 0x000F // Version bits
	L2TPVersion  uint16 = 0x0002 // L2TP Version 2
)

// L2TPHeader represents the L2TP header
type L2TPHeader struct {
	Flags      uint16
	TunnelID   uint16
	SessionID  uint16
	Ns         uint16 // Sequence number (if Sequence bit set)
	Nr         uint16 // Next expected sequence number (if Sequence bit set)
	OffsetSize uint16 // Offset size (if Offset bit set)
}

// ParseL2TPHeader parses L2TP header from bytes
func ParseL2TPHeader(data []byte) (*L2TPHeader, int, error) {
	if len(data) < 6 {
		return nil, 0, fmt.Errorf("data too short for L2TP header")
	}

	header := &L2TPHeader{
		Flags:     binary.BigEndian.Uint16(data[0:2]),
		TunnelID:  binary.BigEndian.Uint16(data[2:4]),
		SessionID: binary.BigEndian.Uint16(data[4:6]),
	}

	offset := 6

	// Check if Length bit is set
	if header.Flags&FlagLength != 0 {
		if len(data) < offset+2 {
			return nil, 0, fmt.Errorf("data too short for length field")
		}
		// Length field exists but we don't store it in header
		offset += 2
	}

	// Check if Sequence bit is set
	if header.Flags&FlagSequence != 0 {
		if len(data) < offset+4 {
			return nil, 0, fmt.Errorf("data too short for sequence fields")
		}
		header.Ns = binary.BigEndian.Uint16(data[offset : offset+2])
		header.Nr = binary.BigEndian.Uint16(data[offset+2 : offset+4])
		offset += 4
	}

	// Check if Offset bit is set
	if header.Flags&FlagOffset != 0 {
		if len(data) < offset+2 {
			return nil, 0, fmt.Errorf("data too short for offset field")
		}
		header.OffsetSize = binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2
		// Skip offset padding
		offset += int(header.OffsetSize)
	}

	return header, offset, nil
}

// IsControl returns true if this is a control message
func (h *L2TPHeader) IsControl() bool {
	return h.Flags&FlagType != 0
}

// HasSequence returns true if sequence numbers are present
func (h *L2TPHeader) HasSequence() bool {
	return h.Flags&FlagSequence != 0
}

// Serialize converts header to bytes
func (h *L2TPHeader) Serialize() []byte {
	buf := make([]byte, 0, 14)

	// Flags and Version
	buf = binary.BigEndian.AppendUint16(buf, h.Flags)
	// Tunnel ID
	buf = binary.BigEndian.AppendUint16(buf, h.TunnelID)
	// Session ID
	buf = binary.BigEndian.AppendUint16(buf, h.SessionID)

	// Length (if Length bit is set)
	// Note: actual length will be filled by caller
	if h.Flags&FlagLength != 0 {
		buf = binary.BigEndian.AppendUint16(buf, 0) // Placeholder, will be updated
	}

	// Ns and Nr (if sequence bit is set)
	if h.Flags&FlagSequence != 0 {
		buf = binary.BigEndian.AppendUint16(buf, h.Ns)
		buf = binary.BigEndian.AppendUint16(buf, h.Nr)
	}

	// Offset (if Offset bit is set)
	if h.Flags&FlagOffset != 0 {
		buf = binary.BigEndian.AppendUint16(buf, h.OffsetSize)
	}

	return buf
}

// AVP represents an Attribute-Value Pair
type AVP struct {
	Mandatory bool
	Hidden    bool
	VendorID  uint16
	Type      uint16
	Value     []byte
}

// ParseAVP parses an AVP from bytes
func ParseAVP(data []byte) (*AVP, int, error) {
	if len(data) < 6 {
		return nil, 0, fmt.Errorf("data too short for AVP")
	}

	avp := &AVP{}
	flags := binary.BigEndian.Uint16(data[0:2])
	length := flags & 0x03FF // Lower 10 bits are length

	if length < 6 {
		return nil, 0, fmt.Errorf("AVP length %d too small (minimum 6)", length)
	}

	avp.Mandatory = (flags & 0x8000) != 0
	avp.Hidden = (flags & 0x4000) != 0

	if int(length) > len(data) {
		return nil, 0, fmt.Errorf("AVP length %d exceeds data length %d", length, len(data))
	}

	// Check if Vendor ID is present (bit 13)
	if flags&0x2000 != 0 {
		if length < 8 {
			return nil, 0, fmt.Errorf("AVP too short for vendor ID")
		}
		avp.VendorID = binary.BigEndian.Uint16(data[2:4])
		avp.Type = binary.BigEndian.Uint16(data[4:6])
		avp.Value = make([]byte, int(length)-6)
		copy(avp.Value, data[6:length])
	} else {
		avp.VendorID = 0
		avp.Type = binary.BigEndian.Uint16(data[2:4])
		avp.Value = make([]byte, int(length)-4)
		copy(avp.Value, data[4:length])
	}

	return avp, int(length), nil
}

// Serialize converts AVP to bytes
func (a *AVP) Serialize() []byte {
	valueLen := len(a.Value)
	var buf []byte
	var length uint16

	if a.VendorID != 0 {
		length = uint16(8 + valueLen)
		buf = make([]byte, length)

		flags := length & 0x03FF
		if a.Mandatory {
			flags |= 0x8000
		}
		if a.Hidden {
			flags |= 0x4000
		}
		flags |= 0x2000 // Vendor ID present

		binary.BigEndian.PutUint16(buf[0:2], flags)
		binary.BigEndian.PutUint16(buf[2:4], a.VendorID)
		binary.BigEndian.PutUint16(buf[4:6], a.Type)
		copy(buf[6:], a.Value)
	} else {
		length = uint16(6 + valueLen)
		buf = make([]byte, length)

		flags := length & 0x03FF
		if a.Mandatory {
			flags |= 0x8000
		}
		if a.Hidden {
			flags |= 0x4000
		}

		binary.BigEndian.PutUint16(buf[0:2], flags)
		binary.BigEndian.PutUint16(buf[2:4], a.Type)
		copy(buf[4:], a.Value)
	}

	return buf
}

// ParseAVPs parses all AVPs from payload
func ParseAVPs(payload []byte) ([]AVP, error) {
	var avps []AVP
	offset := 0

	for offset < len(payload) {
		avp, size, err := ParseAVP(payload[offset:])
		if err != nil {
			return avps, err
		}
		avps = append(avps, *avp)
		offset += size
	}

	return avps, nil
}

// CreateAVP creates a new AVP with the given type and value
func CreateAVP(avpType uint16, value []byte, mandatory bool) AVP {
	return AVP{
		Mandatory: mandatory,
		Hidden:    false,
		VendorID:  0,
		Type:      avpType,
		Value:     value,
	}
}

// CreateUint16AVP creates an AVP with a uint16 value
func CreateUint16AVP(avpType uint16, value uint16, mandatory bool) AVP {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, value)
	return CreateAVP(avpType, buf, mandatory)
}

// CreateUint32AVP creates an AVP with a uint32 value
func CreateUint32AVP(avpType uint16, value uint32, mandatory bool) AVP {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, value)
	return CreateAVP(avpType, buf, mandatory)
}

// CreateStringAVP creates an AVP with a string value
func CreateStringAVP(avpType uint16, value string, mandatory bool) AVP {
	return CreateAVP(avpType, []byte(value), mandatory)
}

// GetUint16 extracts a uint16 value from AVP
func (a *AVP) GetUint16() (uint16, error) {
	if len(a.Value) < 2 {
		return 0, fmt.Errorf("AVP value too short for uint16")
	}
	return binary.BigEndian.Uint16(a.Value), nil
}

// GetUint32 extracts a uint32 value from AVP
func (a *AVP) GetUint32() (uint32, error) {
	if len(a.Value) < 4 {
		return 0, fmt.Errorf("AVP value too short for uint32")
	}
	return binary.BigEndian.Uint32(a.Value), nil
}

// GetString extracts a string value from AVP
func (a *AVP) GetString() string {
	return string(a.Value)
}
