package stream_parser

import (
	"encoding/binary"
	"fmt"
	"math"
	"unicode/utf8"
)

const (
	steamDiscoveryMinSize   = 16
	steamDiscoveryMaxSize   = 65535
	steamDiscoveryMaxFields = 4096
	steamDiscoveryMaxDepth  = 16
)

var steamDiscoveryMagic = [8]byte{0xff, 0xff, 0xff, 0xff, 0x21, 0x4c, 0x5f, 0xa0}

// steamDiscoveryField is a byte-exact description of one protobuf field.
// Offsets are absolute within the containing discovery datagram. Start includes
// the tag, ValueStart excludes the tag and any length prefix, and End is
// exclusive. Descriptors never retain or copy input bytes.
type steamDiscoveryField struct {
	Name                   string
	Kind                   string
	Number                 uint32
	Wire                   uint8
	Start, ValueStart, End int
	Children               []steamDiscoveryField
}

type steamDiscoveryMessage struct {
	HeaderStart, HeaderEnd int
	BodyStart, BodyEnd     int
	MessageType            int32
	Fields                 int
	Header, Body           []steamDiscoveryField
}

type steamDiscoveryFieldSchema struct {
	name       string
	kind       string
	wire       uint8
	repeated   bool
	packable   bool
	required   bool
	message    *steamDiscoverySchema
	enumValues map[int32]struct{}
}

type steamDiscoverySchema struct {
	name     string
	fields   map[uint32]steamDiscoveryFieldSchema
	required []uint32
}

func steamEnum(values ...int32) map[int32]struct{} {
	out := make(map[int32]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func steamSchema(name string, fields ...struct {
	number uint32
	field  steamDiscoveryFieldSchema
}) *steamDiscoverySchema {
	schema := &steamDiscoverySchema{name: name, fields: make(map[uint32]steamDiscoveryFieldSchema, len(fields))}
	for _, entry := range fields {
		schema.fields[entry.number] = entry.field
		if entry.field.required {
			schema.required = append(schema.required, entry.number)
		}
	}
	return schema
}

func steamField(number uint32, name, kind string, wire uint8) struct {
	number uint32
	field  steamDiscoveryFieldSchema
} {
	return struct {
		number uint32
		field  steamDiscoveryFieldSchema
	}{number: number, field: steamDiscoveryFieldSchema{name: name, kind: kind, wire: wire}}
}

func steamRequired(number uint32, name, kind string, wire uint8) struct {
	number uint32
	field  steamDiscoveryFieldSchema
} {
	entry := steamField(number, name, kind, wire)
	entry.field.required = true
	return entry
}

func steamRepeated(number uint32, name, kind string, wire uint8, packable bool) struct {
	number uint32
	field  steamDiscoveryFieldSchema
} {
	entry := steamField(number, name, kind, wire)
	entry.field.repeated, entry.field.packable = true, packable
	return entry
}

func steamMessageField(number uint32, name string, message *steamDiscoverySchema, repeated bool) struct {
	number uint32
	field  steamDiscoveryFieldSchema
} {
	entry := steamField(number, name, "message", 2)
	entry.field.message, entry.field.repeated = message, repeated
	return entry
}

func steamEnumField(number uint32, name string, values map[int32]struct{}, required, repeated bool) struct {
	number uint32
	field  steamDiscoveryFieldSchema
} {
	entry := steamField(number, name, "i32", 0)
	entry.field.enumValues = values
	entry.field.required, entry.field.repeated, entry.field.packable = required, repeated, repeated
	return entry
}

var (
	steamBroadcastMsgEnum = steamEnum(0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16)
	steamVRLinkCapsEnum   = steamEnum(0, 1, 2, 3)
	steamAuthResultEnum   = steamEnum(0, 1, 2, 3, 4, 5, 6, 7, 8)
	steamFormFactorEnum   = steamEnum(0, 1, 2, 3, 4, 5)
	steamTransportEnum    = steamEnum(0, 1, 2, 3, 4, 5, 6)
	steamInterfaceEnum    = steamEnum(0, 1, 2, 3, 4)
	steamStreamResultEnum = steamEnum(0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15)
)

var steamStatusUserSchema = steamSchema("CMsgRemoteClientBroadcastStatus.User",
	steamField(1, "steamid", "fixed64", 1),
	steamField(2, "auth_key_id", "u32", 0),
)

var steamReservedGamepadSchema = steamSchema("CMsgRemoteDeviceStreamingRequest.ReservedGamepad",
	steamField(1, "controller_type", "u32", 0),
	steamField(2, "controller_subtype", "u32", 0),
)

var steamHeaderSchema = steamSchema("CMsgRemoteClientBroadcastHeader",
	steamField(1, "client_id", "u64", 0),
	steamEnumField(2, "msg_type", steamBroadcastMsgEnum, false, false),
	steamField(3, "instance_id", "u64", 0),
	steamField(4, "device_id_OBSOLETE", "u64", 0),
	steamField(5, "device_token", "raw", 2),
)

var steamDiscoveryBodySchema = steamSchema("CMsgRemoteClientBroadcastDiscovery",
	steamField(1, "seq_num", "u32", 0),
	steamRepeated(2, "client_ids", "u64", 0, true),
)

var steamStatusBodySchema = steamSchema("CMsgRemoteClientBroadcastStatus",
	steamField(1, "version", "i32", 0),
	steamField(2, "min_version", "i32", 0),
	steamField(3, "connect_port", "u32", 0),
	steamField(4, "hostname", "string", 2),
	steamField(6, "enabled_services", "u32", 0),
	steamField(7, "ostype", "i32", 0),
	steamField(8, "is64bit", "bool", 0),
	steamMessageField(9, "users", steamStatusUserSchema, true),
	steamField(11, "euniverse", "i32", 0),
	steamField(12, "timestamp", "u32", 0),
	steamField(13, "screen_locked", "bool", 0),
	steamField(14, "games_running", "bool", 0),
	steamRepeated(15, "mac_addresses", "string", 2, false),
	steamField(16, "download_lan_peer_group", "u32", 0),
	steamField(17, "broadcasting_active", "bool", 0),
	steamField(18, "vr_active", "bool", 0),
	steamField(19, "content_cache_port", "u32", 0),
	steamRepeated(20, "ip_addresses", "string", 2, false),
	steamField(21, "public_ip_address", "string", 2),
	steamField(22, "remoteplay_active", "bool", 0),
	steamField(23, "supported_services", "u32", 0),
	steamField(24, "steam_deck", "bool", 0),
	steamField(25, "steam_version", "u64", 0),
	steamEnumField(26, "vr_link_caps", steamVRLinkCapsEnum, false, false),
	steamField(27, "vr_link_invite_client_id", "fixed64", 1),
	steamField(28, "connected_paired_network_hash", "fixed64", 1),
	steamField(29, "wifi_dongle_present", "bool", 0),
	steamField(30, "is_low_spec_hardware", "bool", 0),
	steamField(31, "gaming_device_type", "u32", 0),
)

var steamAuthorizationRequestSchema = steamSchema("CMsgRemoteDeviceAuthorizationRequest",
	steamRequired(1, "device_token", "raw", 2),
	steamField(2, "device_name", "string", 2),
	steamRequired(3, "encrypted_request", "raw", 2),
	steamField(4, "auth_key", "raw", 2),
	steamField(5, "request_id", "u32", 0),
)

var steamAuthorizationResponseSchema = steamSchema("CMsgRemoteDeviceAuthorizationResponse",
	steamEnumField(1, "result", steamAuthResultEnum, true, false),
	steamField(2, "steamid", "fixed64", 1),
	steamField(3, "auth_key", "raw", 2),
	steamField(4, "device_token", "raw", 2),
)

var steamStreamingRequestSchema = steamSchema("CMsgRemoteDeviceStreamingRequest",
	steamRequired(1, "request_id", "u32", 0),
	steamField(2, "maximum_resolution_x", "i32", 0),
	steamField(3, "maximum_resolution_y", "i32", 0),
	steamField(4, "audio_channel_count", "i32", 0),
	steamField(5, "device_version", "string", 2),
	steamField(6, "stream_desktop", "bool", 0),
	steamField(7, "device_token", "raw", 2),
	steamField(8, "pin", "raw", 2),
	steamField(9, "enable_video_streaming", "bool", 0),
	steamField(10, "enable_audio_streaming", "bool", 0),
	steamField(11, "enable_input_streaming", "bool", 0),
	steamField(12, "network_test", "bool", 0),
	steamField(13, "client_id", "u64", 0),
	steamEnumField(14, "supported_transport", steamTransportEnum, false, true),
	steamField(15, "restricted", "bool", 0),
	steamEnumField(16, "form_factor", steamFormFactorEnum, false, false),
	steamField(17, "gamepad_count", "i32", 0),
	steamMessageField(18, "gamepads", steamReservedGamepadSchema, true),
	steamField(19, "gameid", "u64", 0),
	steamEnumField(20, "stream_interface", steamInterfaceEnum, false, false),
	steamField(21, "maximum_framerate_numerator", "i32", 0),
	steamField(22, "maximum_framerate_denominator", "i32", 0),
	steamField(23, "display_hdr", "bool", 0),
)

var steamStreamingResponseSchema = steamSchema("CMsgRemoteDeviceStreamingResponse",
	steamRequired(1, "request_id", "u32", 0),
	steamEnumField(2, "result", steamStreamResultEnum, true, false),
	steamField(3, "port", "u32", 0),
	steamField(4, "encrypted_session_key", "raw", 2),
	steamEnumField(6, "transport", steamTransportEnum, false, false),
	steamField(7, "relay_server", "string", 2),
	steamField(8, "cert", "string", 2),
)

var steamProofRequestSchema = steamSchema("CMsgRemoteDeviceProofRequest",
	steamRequired(1, "challenge", "raw", 2),
	steamField(2, "request_id", "u32", 0),
	steamField(3, "update_secret", "bool", 0),
)

var steamProofResponseSchema = steamSchema("CMsgRemoteDeviceProofResponse",
	steamRequired(1, "response", "raw", 2),
	steamField(2, "request_id", "u32", 0),
	steamField(3, "updated_secret", "bool", 0),
)

var steamStreamingCancelSchema = steamSchema("CMsgRemoteDeviceStreamingCancelRequest",
	steamRequired(1, "request_id", "u32", 0),
)

var steamClientIDDeconflictSchema = steamSchema("CMsgRemoteClientBroadcastClientIDDeconflict",
	steamRepeated(2, "client_ids", "u64", 0, true),
)

var steamTransportSignalSchema = steamSchema("CMsgRemoteDeviceStreamTransportSignal",
	steamField(1, "token", "raw", 2),
	steamField(2, "payload", "raw", 2),
)

var steamStreamingProgressSchema = steamSchema("CMsgRemoteDeviceStreamingProgress",
	steamRequired(1, "request_id", "u32", 0),
	steamField(2, "progress", "float32", 5),
)

var steamAuthorizationConfirmedSchema = steamSchema("CMsgRemoteDeviceAuthorizationConfirmed",
	steamEnumField(1, "result", steamAuthResultEnum, true, false),
)

var steamPairingStateSchema = steamSchema("CMsgRemoteClientBroadcastClientPairingState",
	steamField(1, "my_paired_network_hash", "fixed64", 1),
	steamField(2, "my_pairing_time", "u32", 0),
)

var steamPairingExclusivitySchema = steamSchema("CMsgRemoteClientBroadcastClientPairingExclusivity",
	steamField(1, "if_paired_network_hash_is", "fixed64", 1),
	steamField(2, "unpair_unless_you_are_client_id", "fixed64", 1),
	steamField(3, "last_known_pairing_time", "u32", 0),
)

var steamEmptyOfflineSchema = steamSchema("CMsgRemoteClientBroadcastOffline")
var steamEmptyAuthorizationCancelSchema = steamSchema("CMsgRemoteDeviceAuthorizationCancelRequest")

var steamBodySchemas = map[int32]*steamDiscoverySchema{
	0:  steamDiscoveryBodySchema,
	1:  steamStatusBodySchema,
	2:  steamEmptyOfflineSchema,
	3:  steamAuthorizationRequestSchema,
	4:  steamAuthorizationResponseSchema,
	5:  steamStreamingRequestSchema,
	6:  steamStreamingResponseSchema,
	7:  steamProofRequestSchema,
	8:  steamProofResponseSchema,
	9:  steamEmptyAuthorizationCancelSchema,
	10: steamStreamingCancelSchema,
	11: steamClientIDDeconflictSchema,
	12: steamTransportSignalSchema,
	13: steamStreamingProgressSchema,
	14: steamAuthorizationConfirmedSchema,
	15: steamPairingStateSchema,
	16: steamPairingExclusivitySchema,
}

type steamDiscoveryBudget struct{ fields int }

func (budget *steamDiscoveryBudget) take() error {
	if budget.fields >= steamDiscoveryMaxFields {
		return fmt.Errorf("steam discovery: protobuf field/element count exceeds %d limit", steamDiscoveryMaxFields)
	}
	budget.fields++
	return nil
}

func steamReadVarint(data []byte, offset, end int) (uint64, int, error) {
	var value uint64
	for index := 0; index < 10; index++ {
		if offset+index >= end {
			return 0, offset, fmt.Errorf("steam discovery: truncated protobuf varint")
		}
		octet := data[offset+index]
		if index == 9 && octet > 1 {
			return 0, offset, fmt.Errorf("steam discovery: protobuf varint overflow")
		}
		value |= uint64(octet&0x7f) << (7 * index)
		if octet&0x80 == 0 {
			return value, offset + index + 1, nil
		}
	}
	return 0, offset, fmt.Errorf("steam discovery: protobuf varint overflow")
}

func steamUnknownName(number uint32, context string) string {
	if context != "" {
		return fmt.Sprintf("unknown message %s field %d", context, number)
	}
	return fmt.Sprintf("unknown_field_%d", number)
}

func steamLengthBounds(data []byte, offset, end int) (int, int, error) {
	length, valueStart, err := steamReadVarint(data, offset, end)
	if err != nil {
		return 0, 0, err
	}
	if length > uint64(end-valueStart) {
		return 0, 0, fmt.Errorf("steam discovery: length-delimited field exceeds containing message")
	}
	return valueStart, valueStart + int(length), nil
}

func steamEnumName(data []byte, field steamDiscoveryFieldSchema, start, end int) (string, bool, error) {
	if field.enumValues == nil {
		return field.name, true, nil
	}
	value, next, err := steamReadVarint(data, start, end)
	if err != nil || next != end {
		if err == nil {
			err = fmt.Errorf("steam discovery: enum contains trailing bytes")
		}
		return "", false, err
	}
	decoded := int32(value)
	// All declared values in the pinned schema are non-negative. Do not let an
	// out-of-domain uint64 wrap into a declared int32 enumerator.
	_, known := field.enumValues[decoded]
	if !known || value > math.MaxInt32 {
		return fmt.Sprintf("%s (unknown enum %d)", field.name, decoded), false, nil
	}
	return field.name, true, nil
}

func steamValidateKnownScalar(data []byte, field steamDiscoveryFieldSchema, start, end int) (string, bool, error) {
	name := field.name
	switch field.wire {
	case 0:
		value, next, err := steamReadVarint(data, start, end)
		if err != nil {
			return "", false, err
		}
		if next != end {
			return "", false, fmt.Errorf("steam discovery: scalar contains trailing bytes")
		}
		if field.kind == "u32" && value > math.MaxUint32 {
			return "", false, fmt.Errorf("steam discovery: uint32 field overflows")
		}
		if field.enumValues != nil {
			return steamEnumName(data, field, start, end)
		}
	case 1:
		if end-start != 8 {
			return "", false, fmt.Errorf("steam discovery: fixed64 field has invalid length")
		}
	case 2:
		if field.kind == "string" && !utf8.Valid(data[start:end]) {
			return "", false, fmt.Errorf("steam discovery: string field is not valid UTF-8")
		}
	case 5:
		if end-start != 4 {
			return "", false, fmt.Errorf("steam discovery: fixed32 field has invalid length")
		}
	default:
		return "", false, fmt.Errorf("steam discovery: invalid known wire type %d", field.wire)
	}
	return name, true, nil
}

func steamParsePacked(data []byte, start, end int, field steamDiscoveryFieldSchema, number uint32, budget *steamDiscoveryBudget) ([]steamDiscoveryField, error) {
	children := make([]steamDiscoveryField, 0)
	for offset := start; offset < end; {
		if err := budget.take(); err != nil {
			return nil, err
		}
		next := offset
		switch field.wire {
		case 0:
			_, varintEnd, err := steamReadVarint(data, offset, end)
			if err != nil {
				return nil, err
			}
			next = varintEnd
		case 1:
			if end-offset < 8 {
				return nil, fmt.Errorf("steam discovery: truncated packed fixed64 value")
			}
			next += 8
		case 5:
			if end-offset < 4 {
				return nil, fmt.Errorf("steam discovery: truncated packed fixed32 value")
			}
			next += 4
		default:
			return nil, fmt.Errorf("steam discovery: non-packable field encoded as packed")
		}
		name, _, err := steamValidateKnownScalar(data, field, offset, next)
		if err != nil {
			return nil, err
		}
		children = append(children, steamDiscoveryField{
			Name: name, Kind: field.kind, Number: number, Wire: field.wire,
			Start: offset, ValueStart: offset, End: next,
		})
		offset = next
	}
	return children, nil
}

// steamParseFields validates exactly [start,end). groupEnd is zero for a
// length-bounded protobuf message and otherwise names the start-group whose
// matching end tag must terminate this range. Closing tags become children so
// every byte in a group remains represented in the AST.
func steamParseFields(data []byte, start, end int, schema *steamDiscoverySchema, budget *steamDiscoveryBudget, depth int, groupEnd uint32, unknownContext string) ([]steamDiscoveryField, int, error) {
	fields := make([]steamDiscoveryField, 0)
	var seen uint64
	for offset := start; offset < end; {
		tagStart := offset
		key, afterTag, err := steamReadVarint(data, offset, end)
		if err != nil {
			return nil, offset, err
		}
		number64, wire := key>>3, uint8(key&7)
		if number64 == 0 || number64 > (1<<29)-1 {
			return nil, offset, fmt.Errorf("steam discovery: invalid protobuf field number %d", number64)
		}
		if wire > 5 {
			return nil, offset, fmt.Errorf("steam discovery: invalid protobuf wire type %d", wire)
		}
		number := uint32(number64)
		if wire == 4 {
			if groupEnd == 0 {
				return nil, offset, fmt.Errorf("steam discovery: unexpected end-group tag")
			}
			if number != groupEnd {
				return nil, offset, fmt.Errorf("steam discovery: mismatched end-group tag %d for group %d", number, groupEnd)
			}
			if err := budget.take(); err != nil {
				return nil, offset, err
			}
			fields = append(fields, steamDiscoveryField{
				Name: fmt.Sprintf("end_group_%d", number), Kind: "group", Number: number, Wire: wire,
				Start: tagStart, ValueStart: afterTag, End: afterTag,
			})
			return fields, afterTag, nil
		}
		if err := budget.take(); err != nil {
			return nil, offset, err
		}

		field, known := steamDiscoveryFieldSchema{}, false
		if schema != nil {
			field, known = schema.fields[number]
		}
		packed := known && field.repeated && field.packable && wire == 2 && field.wire != 2
		correctWire := known && (wire == field.wire || packed)
		name, kind := field.name, field.kind
		if !correctWire {
			name, kind = steamUnknownName(number, unknownContext), "raw"
		}
		valueStart, fieldEnd := afterTag, afterTag
		var children []steamDiscoveryField
		switch wire {
		case 0:
			_, fieldEnd, err = steamReadVarint(data, afterTag, end)
		case 1:
			if end-afterTag < 8 {
				err = fmt.Errorf("steam discovery: truncated fixed64 field")
			} else {
				fieldEnd = afterTag + 8
			}
		case 2:
			valueStart, fieldEnd, err = steamLengthBounds(data, afterTag, end)
		case 3:
			if depth >= steamDiscoveryMaxDepth {
				err = fmt.Errorf("steam discovery: protobuf nesting exceeds %d levels", steamDiscoveryMaxDepth)
				break
			}
			children, fieldEnd, err = steamParseFields(data, afterTag, end, nil, budget, depth+1, number, unknownContext)
			valueStart, kind = afterTag, "group"
		case 5:
			if end-afterTag < 4 {
				err = fmt.Errorf("steam discovery: truncated fixed32 field")
			} else {
				fieldEnd = afterTag + 4
			}
		}
		if err != nil {
			return nil, offset, err
		}

		if correctWire {
			validPresence := true
			switch {
			case packed:
				kind = "packed"
				children, err = steamParsePacked(data, valueStart, fieldEnd, field, number, budget)
			case field.kind == "message":
				if depth >= steamDiscoveryMaxDepth {
					err = fmt.Errorf("steam discovery: protobuf nesting exceeds %d levels", steamDiscoveryMaxDepth)
					break
				}
				children, _, err = steamParseFields(data, valueStart, fieldEnd, field.message, budget, depth+1, 0, "")
			case wire != 3:
				name, validPresence, err = steamValidateKnownScalar(data, field, valueStart, fieldEnd)
			}
			if err != nil {
				return nil, offset, err
			}
			if validPresence && number < 64 {
				seen |= uint64(1) << number
			}
		}
		fields = append(fields, steamDiscoveryField{
			Name: name, Kind: kind, Number: number, Wire: wire,
			Start: tagStart, ValueStart: valueStart, End: fieldEnd, Children: children,
		})
		offset = fieldEnd
	}
	if groupEnd != 0 {
		return nil, end, fmt.Errorf("steam discovery: unterminated protobuf group %d", groupEnd)
	}
	if schema != nil {
		for _, number := range schema.required {
			if number >= 64 || seen&(uint64(1)<<number) == 0 {
				return nil, end, fmt.Errorf("steam discovery: required field %s missing from %s", schema.fields[number].name, schema.name)
			}
		}
	}
	return fields, end, nil
}

// decodeSteamDiscovery validates a complete, explicitly bounded datagram. It
// only decodes the public framing and protobuf schemas: encrypted fields remain
// opaque and no network, session, decompression, or execution behavior occurs.
func decodeSteamDiscovery(data []byte) (*steamDiscoveryMessage, error) {
	if len(data) < steamDiscoveryMinSize || len(data) > steamDiscoveryMaxSize {
		return nil, fmt.Errorf("steam discovery: datagram outside %d..%d bytes", steamDiscoveryMinSize, steamDiscoveryMaxSize)
	}
	for index, octet := range steamDiscoveryMagic {
		if data[index] != octet {
			return nil, fmt.Errorf("steam discovery: invalid signature")
		}
	}
	headerLength := binary.LittleEndian.Uint32(data[8:12])
	if uint64(headerLength)+16 > uint64(len(data)) {
		return nil, fmt.Errorf("steam discovery: header length exceeds datagram")
	}
	headerStart, headerEnd := 12, 12+int(headerLength)
	bodyLength := binary.LittleEndian.Uint32(data[headerEnd : headerEnd+4])
	bodyStart := headerEnd + 4
	if uint64(bodyLength) != uint64(len(data)-bodyStart) {
		return nil, fmt.Errorf("steam discovery: body length does not consume datagram")
	}
	bodyEnd := bodyStart + int(bodyLength)

	budget := &steamDiscoveryBudget{}
	header, _, err := steamParseFields(data, headerStart, headerEnd, steamHeaderSchema, budget, 0, 0, "")
	if err != nil {
		return nil, err
	}
	messageType := int32(0)
	typeRaw := uint64(0)
	for _, field := range header {
		if field.Number == 2 && field.Wire == 0 && field.Kind == "i32" {
			value, _, readErr := steamReadVarint(data, field.ValueStart, field.End)
			if readErr != nil {
				return nil, readErr
			}
			typeRaw, messageType = value, int32(value)
		}
	}
	bodySchema := steamBodySchemas[messageType]
	unknownContext := ""
	if typeRaw > 16 || bodySchema == nil {
		bodySchema = nil
		unknownContext = fmt.Sprintf("type %d", messageType)
	}
	body, _, err := steamParseFields(data, bodyStart, bodyEnd, bodySchema, budget, 0, 0, unknownContext)
	if err != nil {
		return nil, err
	}
	return &steamDiscoveryMessage{
		HeaderStart: headerStart, HeaderEnd: headerEnd, BodyStart: bodyStart, BodyEnd: bodyEnd,
		MessageType: messageType, Fields: budget.fields, Header: header, Body: body,
	}, nil
}

// decodeSteamScalar is a pure conversion helper for validated scalar spans.
// Varint input must contain exactly one complete value; fixed-width inputs must
// have their exact width. It neither mutates nor retains data.
func decodeSteamScalar(data []byte, kind string) (any, error) {
	switch kind {
	case "u64", "u32", "i32", "bool":
		value, end, err := steamReadVarint(data, 0, len(data))
		if err != nil {
			return nil, err
		}
		if end != len(data) {
			return nil, fmt.Errorf("steam discovery: scalar contains trailing bytes")
		}
		switch kind {
		case "u64":
			return value, nil
		case "u32":
			if value > math.MaxUint32 {
				return nil, fmt.Errorf("steam discovery: uint32 field overflows")
			}
			return uint32(value), nil
		case "i32":
			return int32(value), nil
		default:
			return value != 0, nil
		}
	case "fixed64":
		if len(data) != 8 {
			return nil, fmt.Errorf("steam discovery: fixed64 scalar requires 8 bytes")
		}
		return binary.LittleEndian.Uint64(data), nil
	case "float32":
		if len(data) != 4 {
			return nil, fmt.Errorf("steam discovery: float32 scalar requires 4 bytes")
		}
		return math.Float32frombits(binary.LittleEndian.Uint32(data)), nil
	case "string":
		if !utf8.Valid(data) {
			return nil, fmt.Errorf("steam discovery: string scalar is not valid UTF-8")
		}
		return string(data), nil
	case "raw":
		return append([]byte(nil), data...), nil
	default:
		return nil, fmt.Errorf("steam discovery: unsupported scalar kind %q", kind)
	}
}
