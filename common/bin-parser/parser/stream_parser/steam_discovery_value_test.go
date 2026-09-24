package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
)

func steamTestVarint(value uint64) []byte {
	var out [10]byte
	index := 0
	for value >= 0x80 {
		out[index] = byte(value) | 0x80
		value >>= 7
		index++
	}
	out[index] = byte(value)
	return append([]byte(nil), out[:index+1]...)
}

func steamTestTag(number uint32, wire uint8) []byte {
	return steamTestVarint(uint64(number)<<3 | uint64(wire))
}

func steamTestVarintField(number uint32, value uint64) []byte {
	out := steamTestTag(number, 0)
	return append(out, steamTestVarint(value)...)
}

func steamTestFixedField(number uint32, wire uint8, value []byte) []byte {
	out := steamTestTag(number, wire)
	return append(out, value...)
}

func steamTestBytesField(number uint32, value []byte) []byte {
	out := steamTestTag(number, 2)
	out = append(out, steamTestVarint(uint64(len(value)))...)
	return append(out, value...)
}

func steamTestFrame(header, body []byte) []byte {
	out := make([]byte, 12, 16+len(header)+len(body))
	copy(out, steamDiscoveryMagic[:])
	binary.LittleEndian.PutUint32(out[8:], uint32(len(header)))
	out = append(out, header...)
	var length [4]byte
	binary.LittleEndian.PutUint32(length[:], uint32(len(body)))
	out = append(out, length[:]...)
	out = append(out, body...)
	return out
}

func steamTestFrameType(messageType int32, body []byte) []byte {
	return steamTestFrame(steamTestVarintField(2, uint64(uint32(messageType))), body)
}

func steamTestRequireError(t *testing.T, data []byte) {
	t.Helper()
	message, err := decodeSteamDiscovery(data)
	if err == nil {
		t.Fatalf("decodeSteamDiscovery unexpectedly succeeded: %#v", message)
	}
	if message != nil {
		t.Fatalf("failed decode returned a partial message: %#v", message)
	}
}

func steamTestSchemaMessage(schema *steamDiscoverySchema) []byte {
	numbers := make([]int, 0, len(schema.fields))
	for number := range schema.fields {
		numbers = append(numbers, int(number))
	}
	sort.Ints(numbers)
	var out []byte
	for _, rawNumber := range numbers {
		number := uint32(rawNumber)
		field := schema.fields[number]
		switch field.wire {
		case 0:
			out = append(out, steamTestVarintField(number, 0)...)
		case 1:
			out = append(out, steamTestFixedField(number, 1, make([]byte, 8))...)
		case 2:
			value := []byte{0xa5}
			if field.kind == "string" {
				value = []byte("x")
			} else if field.kind == "message" {
				value = steamTestSchemaMessage(field.message)
			}
			out = append(out, steamTestBytesField(number, value)...)
		case 5:
			out = append(out, steamTestFixedField(number, 5, make([]byte, 4))...)
		}
	}
	return out
}

func steamTestCountFields(fields []steamDiscoveryField) int {
	count := 0
	for _, field := range fields {
		count++
		count += steamTestCountFields(field.Children)
	}
	return count
}

func steamTestCheckPartition(t *testing.T, fields []steamDiscoveryField, start, end int) {
	t.Helper()
	offset := start
	for _, field := range fields {
		if field.Start != offset || field.Start > field.ValueStart || field.ValueStart > field.End || field.End > end {
			t.Fatalf("invalid/overlapping field span at %s: got %d..%d..%d, want start %d within end %d", field.Name, field.Start, field.ValueStart, field.End, offset, end)
		}
		if len(field.Children) != 0 {
			steamTestCheckPartition(t, field.Children, field.ValueStart, field.End)
		}
		offset = field.End
	}
	if offset != end {
		t.Fatalf("fields stop at %d, message/group ends at %d", offset, end)
	}
}

func TestSteamDiscoveryAllPinnedSchemasAndOffsets(t *testing.T) {
	for messageType := int32(0); messageType <= 16; messageType++ {
		schema := steamBodySchemas[messageType]
		body := steamTestSchemaMessage(schema)
		wire := steamTestFrameType(messageType, body)
		message, err := decodeSteamDiscovery(wire)
		if err != nil {
			t.Fatalf("type %d: %v", messageType, err)
		}
		if message.MessageType != messageType {
			t.Fatalf("type %d decoded as %d", messageType, message.MessageType)
		}
		if message.HeaderStart != 12 || message.HeaderEnd != 14 || message.BodyStart != 18 || message.BodyEnd != len(wire) {
			t.Fatalf("type %d has wrong envelope offsets: %#v", messageType, message)
		}
		if len(message.Body) != len(schema.fields) {
			t.Fatalf("type %d decoded %d top fields, schema has %d", messageType, len(message.Body), len(schema.fields))
		}
		for _, field := range message.Body {
			if strings.Contains(field.Name, "unknown") || field.Name != schema.fields[field.Number].name {
				t.Fatalf("type %d field %d not schema-decoded: %#v", messageType, field.Number, field)
			}
		}
		steamTestCheckPartition(t, message.Header, message.HeaderStart, message.HeaderEnd)
		steamTestCheckPartition(t, message.Body, message.BodyStart, message.BodyEnd)
		if got := steamTestCountFields(message.Header) + steamTestCountFields(message.Body); got != message.Fields {
			t.Fatalf("type %d field budget=%d, AST nodes=%d", messageType, message.Fields, got)
		}
	}

	// Absence of the optional msg_type applies the pinned proto2 default 0.
	message, err := decodeSteamDiscovery(steamTestFrame(nil, nil))
	if err != nil || message.MessageType != 0 || len(message.Header) != 0 || len(message.Body) != 0 {
		t.Fatalf("missing type default: message=%#v err=%v", message, err)
	}
}

func steamTestEnumSignature(values map[int32]struct{}) string {
	if len(values) == 0 {
		return "-"
	}
	sorted := make([]int, 0, len(values))
	for value := range values {
		sorted = append(sorted, int(value))
	}
	sort.Ints(sorted)
	parts := make([]string, len(sorted))
	contiguous := true
	for index, value := range sorted {
		parts[index] = fmt.Sprint(value)
		if index != 0 && value != sorted[index-1]+1 {
			contiguous = false
		}
	}
	if contiguous && len(sorted) > 1 {
		return fmt.Sprintf("%d..%d", sorted[0], sorted[len(sorted)-1])
	}
	return strings.Join(parts, ",")
}

func steamTestSchemaSignature(label string, schema *steamDiscoverySchema) string {
	required := append([]uint32(nil), schema.required...)
	sort.Slice(required, func(i, j int) bool { return required[i] < required[j] })
	requiredText := "-"
	if len(required) != 0 {
		parts := make([]string, len(required))
		for index, number := range required {
			parts[index] = fmt.Sprint(number)
		}
		requiredText = strings.Join(parts, ",")
	}
	numbers := make([]int, 0, len(schema.fields))
	for number := range schema.fields {
		numbers = append(numbers, int(number))
	}
	sort.Ints(numbers)
	fields := make([]string, 0, len(numbers))
	for _, rawNumber := range numbers {
		number := uint32(rawNumber)
		field := schema.fields[number]
		cardinality := "o"
		if field.required {
			cardinality = "r"
		} else if field.repeated {
			cardinality = "l"
		}
		packed := "-"
		if field.packable {
			packed = "p"
		}
		target := "-"
		if field.message != nil {
			target = field.message.name
		}
		fields = append(fields, fmt.Sprintf("%d:%s/%s/w%d/%s/%s/e%s/m%s", number, field.name, field.kind, field.wire, cardinality, packed, steamTestEnumSignature(field.enumValues), target))
	}
	return fmt.Sprintf("%s=%s|required=%s|fields=%s", label, schema.name, requiredText, strings.Join(fields, ";"))
}

// This oracle is transcribed directly from the pinned 266-line proto. Unlike
// steamTestSchemaMessage, it does not discover expected fields by walking the
// production descriptors, so a mutually-consistent descriptor/decoder typo is
// still caught. encrypted_request is intentionally raw: the nested escrow
// ticket declaration is encrypted payload, not a reachable message field.
func TestSteamDiscoveryPinnedSchemaSignatureOracle(t *testing.T) {
	schemas := []struct {
		label  string
		schema *steamDiscoverySchema
	}{
		{"header", steamHeaderSchema},
		{"body0", steamBodySchemas[0]}, {"body1", steamBodySchemas[1]}, {"body2", steamBodySchemas[2]},
		{"body3", steamBodySchemas[3]}, {"body4", steamBodySchemas[4]}, {"body5", steamBodySchemas[5]},
		{"body6", steamBodySchemas[6]}, {"body7", steamBodySchemas[7]}, {"body8", steamBodySchemas[8]},
		{"body9", steamBodySchemas[9]}, {"body10", steamBodySchemas[10]}, {"body11", steamBodySchemas[11]},
		{"body12", steamBodySchemas[12]}, {"body13", steamBodySchemas[13]}, {"body14", steamBodySchemas[14]},
		{"body15", steamBodySchemas[15]}, {"body16", steamBodySchemas[16]},
		{"nested.user", steamStatusUserSchema}, {"nested.gamepad", steamReservedGamepadSchema},
	}
	actual := make([]string, len(schemas))
	for index, schema := range schemas {
		actual[index] = steamTestSchemaSignature(schema.label, schema.schema)
	}
	const expected = `header=CMsgRemoteClientBroadcastHeader|required=-|fields=1:client_id/u64/w0/o/-/e-/m-;2:msg_type/i32/w0/o/-/e0..16/m-;3:instance_id/u64/w0/o/-/e-/m-;4:device_id_OBSOLETE/u64/w0/o/-/e-/m-;5:device_token/raw/w2/o/-/e-/m-
body0=CMsgRemoteClientBroadcastDiscovery|required=-|fields=1:seq_num/u32/w0/o/-/e-/m-;2:client_ids/u64/w0/l/p/e-/m-
body1=CMsgRemoteClientBroadcastStatus|required=-|fields=1:version/i32/w0/o/-/e-/m-;2:min_version/i32/w0/o/-/e-/m-;3:connect_port/u32/w0/o/-/e-/m-;4:hostname/string/w2/o/-/e-/m-;6:enabled_services/u32/w0/o/-/e-/m-;7:ostype/i32/w0/o/-/e-/m-;8:is64bit/bool/w0/o/-/e-/m-;9:users/message/w2/l/-/e-/mCMsgRemoteClientBroadcastStatus.User;11:euniverse/i32/w0/o/-/e-/m-;12:timestamp/u32/w0/o/-/e-/m-;13:screen_locked/bool/w0/o/-/e-/m-;14:games_running/bool/w0/o/-/e-/m-;15:mac_addresses/string/w2/l/-/e-/m-;16:download_lan_peer_group/u32/w0/o/-/e-/m-;17:broadcasting_active/bool/w0/o/-/e-/m-;18:vr_active/bool/w0/o/-/e-/m-;19:content_cache_port/u32/w0/o/-/e-/m-;20:ip_addresses/string/w2/l/-/e-/m-;21:public_ip_address/string/w2/o/-/e-/m-;22:remoteplay_active/bool/w0/o/-/e-/m-;23:supported_services/u32/w0/o/-/e-/m-;24:steam_deck/bool/w0/o/-/e-/m-;25:steam_version/u64/w0/o/-/e-/m-;26:vr_link_caps/i32/w0/o/-/e0..3/m-;27:vr_link_invite_client_id/fixed64/w1/o/-/e-/m-;28:connected_paired_network_hash/fixed64/w1/o/-/e-/m-;29:wifi_dongle_present/bool/w0/o/-/e-/m-;30:is_low_spec_hardware/bool/w0/o/-/e-/m-;31:gaming_device_type/u32/w0/o/-/e-/m-
body2=CMsgRemoteClientBroadcastOffline|required=-|fields=
body3=CMsgRemoteDeviceAuthorizationRequest|required=1,3|fields=1:device_token/raw/w2/r/-/e-/m-;2:device_name/string/w2/o/-/e-/m-;3:encrypted_request/raw/w2/r/-/e-/m-;4:auth_key/raw/w2/o/-/e-/m-;5:request_id/u32/w0/o/-/e-/m-
body4=CMsgRemoteDeviceAuthorizationResponse|required=1|fields=1:result/i32/w0/r/-/e0..8/m-;2:steamid/fixed64/w1/o/-/e-/m-;3:auth_key/raw/w2/o/-/e-/m-;4:device_token/raw/w2/o/-/e-/m-
body5=CMsgRemoteDeviceStreamingRequest|required=1|fields=1:request_id/u32/w0/r/-/e-/m-;2:maximum_resolution_x/i32/w0/o/-/e-/m-;3:maximum_resolution_y/i32/w0/o/-/e-/m-;4:audio_channel_count/i32/w0/o/-/e-/m-;5:device_version/string/w2/o/-/e-/m-;6:stream_desktop/bool/w0/o/-/e-/m-;7:device_token/raw/w2/o/-/e-/m-;8:pin/raw/w2/o/-/e-/m-;9:enable_video_streaming/bool/w0/o/-/e-/m-;10:enable_audio_streaming/bool/w0/o/-/e-/m-;11:enable_input_streaming/bool/w0/o/-/e-/m-;12:network_test/bool/w0/o/-/e-/m-;13:client_id/u64/w0/o/-/e-/m-;14:supported_transport/i32/w0/l/p/e0..6/m-;15:restricted/bool/w0/o/-/e-/m-;16:form_factor/i32/w0/o/-/e0..5/m-;17:gamepad_count/i32/w0/o/-/e-/m-;18:gamepads/message/w2/l/-/e-/mCMsgRemoteDeviceStreamingRequest.ReservedGamepad;19:gameid/u64/w0/o/-/e-/m-;20:stream_interface/i32/w0/o/-/e0..4/m-;21:maximum_framerate_numerator/i32/w0/o/-/e-/m-;22:maximum_framerate_denominator/i32/w0/o/-/e-/m-;23:display_hdr/bool/w0/o/-/e-/m-
body6=CMsgRemoteDeviceStreamingResponse|required=1,2|fields=1:request_id/u32/w0/r/-/e-/m-;2:result/i32/w0/r/-/e0..15/m-;3:port/u32/w0/o/-/e-/m-;4:encrypted_session_key/raw/w2/o/-/e-/m-;6:transport/i32/w0/o/-/e0..6/m-;7:relay_server/string/w2/o/-/e-/m-;8:cert/string/w2/o/-/e-/m-
body7=CMsgRemoteDeviceProofRequest|required=1|fields=1:challenge/raw/w2/r/-/e-/m-;2:request_id/u32/w0/o/-/e-/m-;3:update_secret/bool/w0/o/-/e-/m-
body8=CMsgRemoteDeviceProofResponse|required=1|fields=1:response/raw/w2/r/-/e-/m-;2:request_id/u32/w0/o/-/e-/m-;3:updated_secret/bool/w0/o/-/e-/m-
body9=CMsgRemoteDeviceAuthorizationCancelRequest|required=-|fields=
body10=CMsgRemoteDeviceStreamingCancelRequest|required=1|fields=1:request_id/u32/w0/r/-/e-/m-
body11=CMsgRemoteClientBroadcastClientIDDeconflict|required=-|fields=2:client_ids/u64/w0/l/p/e-/m-
body12=CMsgRemoteDeviceStreamTransportSignal|required=-|fields=1:token/raw/w2/o/-/e-/m-;2:payload/raw/w2/o/-/e-/m-
body13=CMsgRemoteDeviceStreamingProgress|required=1|fields=1:request_id/u32/w0/r/-/e-/m-;2:progress/float32/w5/o/-/e-/m-
body14=CMsgRemoteDeviceAuthorizationConfirmed|required=1|fields=1:result/i32/w0/r/-/e0..8/m-
body15=CMsgRemoteClientBroadcastClientPairingState|required=-|fields=1:my_paired_network_hash/fixed64/w1/o/-/e-/m-;2:my_pairing_time/u32/w0/o/-/e-/m-
body16=CMsgRemoteClientBroadcastClientPairingExclusivity|required=-|fields=1:if_paired_network_hash_is/fixed64/w1/o/-/e-/m-;2:unpair_unless_you_are_client_id/fixed64/w1/o/-/e-/m-;3:last_known_pairing_time/u32/w0/o/-/e-/m-
nested.user=CMsgRemoteClientBroadcastStatus.User|required=-|fields=1:steamid/fixed64/w1/o/-/e-/m-;2:auth_key_id/u32/w0/o/-/e-/m-
nested.gamepad=CMsgRemoteDeviceStreamingRequest.ReservedGamepad|required=-|fields=1:controller_type/u32/w0/o/-/e-/m-;2:controller_subtype/u32/w0/o/-/e-/m-`
	if got := strings.Join(actual, "\n"); got != expected {
		t.Fatalf("pinned schema signature drift:\n--- got ---\n%s\n--- want ---\n%s", got, expected)
	}
}

func TestSteamDiscoveryStrictFramingPrefixesAndOverflow(t *testing.T) {
	valid := steamTestFrameType(5, steamTestVarintField(1, 7))
	for cut := 0; cut < len(valid); cut++ {
		steamTestRequireError(t, valid[:cut])
	}
	if _, err := decodeSteamDiscovery(valid); err != nil {
		t.Fatalf("complete frame: %v", err)
	}

	bad := append([]byte(nil), valid...)
	bad[0] = 0
	steamTestRequireError(t, bad)
	steamTestRequireError(t, append(append([]byte(nil), valid...), 0))
	steamTestRequireError(t, make([]byte, steamDiscoveryMaxSize+1))

	bad = append([]byte(nil), valid...)
	binary.LittleEndian.PutUint32(bad[8:12], math.MaxUint32)
	steamTestRequireError(t, bad)
	bad = append([]byte(nil), valid...)
	binary.LittleEndian.PutUint32(bad[14:18], math.MaxUint32)
	steamTestRequireError(t, bad)

	varintOverflow := bytes.Repeat([]byte{0x80}, 10)
	steamTestRequireError(t, steamTestFrameType(0, append(steamTestTag(1, 0), varintOverflow...)))
	lastOverflow := append(bytes.Repeat([]byte{0x80}, 9), 0x02)
	steamTestRequireError(t, steamTestFrameType(0, append(steamTestTag(1, 0), lastOverflow...)))
	steamTestRequireError(t, steamTestFrameType(0, append([]byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80}, 0x02)))
	steamTestRequireError(t, steamTestFrameType(0, append(steamTestTag(3, 2), lastOverflow...)))
	steamTestRequireError(t, steamTestFrameType(0, steamTestVarintField(1, uint64(math.MaxUint32)+1)))

	for _, body := range [][]byte{
		{0},    // field number zero
		{0x0e}, // reserved wire type 6
		{0x0c}, // top-level end group
		append(steamTestTag(40, 1), make([]byte, 7)...),
		append(steamTestTag(40, 5), make([]byte, 3)...),
		append(steamTestTag(40, 2), 2, 1),
	} {
		steamTestRequireError(t, steamTestFrameType(0, body))
	}
}

func TestSteamDiscoveryRequiredWrongWireAndUnknownEnums(t *testing.T) {
	missing := map[int32][]byte{
		3: nil, 4: nil, 5: nil, 6: steamTestVarintField(1, 0), 7: nil,
		8: nil, 10: nil, 13: nil, 14: nil,
	}
	for messageType, body := range missing {
		steamTestRequireError(t, steamTestFrameType(messageType, body))
	}

	// Correct field numbers with the wrong wire stay opaque and cannot satisfy
	// proto2 required presence.
	steamTestRequireError(t, steamTestFrameType(5, steamTestBytesField(1, nil)))
	steamTestRequireError(t, steamTestFrameType(3, append(steamTestBytesField(1, nil), steamTestVarintField(3, 0)...)))
	// Unknown proto2 enum numerics are annotated but do not initialize a
	// required enum field.
	steamTestRequireError(t, steamTestFrameType(4, steamTestVarintField(1, 99)))
	steamTestRequireError(t, steamTestFrameType(14, steamTestVarintField(1, math.MaxUint64)))

	// Duplicate singular/required fields are retained in wire order and any
	// recognized enum occurrence satisfies required presence.
	body := append(steamTestVarintField(1, 99), steamTestVarintField(1, 2)...)
	body = append(body, steamTestVarintField(1, 1)...)
	message, err := decodeSteamDiscovery(steamTestFrameType(4, body))
	if err != nil {
		t.Fatal(err)
	}
	if len(message.Body) != 3 || !strings.Contains(message.Body[0].Name, "unknown enum") || message.Body[1].Name != "result" || message.Body[2].Name != "result" {
		t.Fatalf("enum repeats not retained/annotated: %#v", message.Body)
	}
}

func TestSteamDiscoveryPackedUnpackedRepeatsAndLastType(t *testing.T) {
	header := steamTestVarintField(2, 1)
	header = append(header, steamTestBytesField(2, []byte{0})...) // wrong wire: opaque
	header = append(header, steamTestVarintField(2, 0)...)
	body := steamTestVarintField(1, 7)
	body = append(body, steamTestVarintField(1, 8)...)
	body = append(body, steamTestVarintField(2, 11)...)
	body = append(body, steamTestBytesField(2, append(steamTestVarint(12), steamTestVarint(13)...))...)
	body = append(body, steamTestVarintField(2, 14)...)
	message, err := decodeSteamDiscovery(steamTestFrame(header, body))
	if err != nil {
		t.Fatal(err)
	}
	if message.MessageType != 0 || len(message.Header) != 3 || message.Header[1].Kind != "raw" || message.Header[1].Name != "unknown_field_2" {
		t.Fatalf("last valid type/wrong-wire handling: %#v", message)
	}
	wantKinds := []string{"u32", "u32", "u64", "packed", "u64"}
	if len(message.Body) != len(wantKinds) {
		t.Fatalf("body=%#v", message.Body)
	}
	for index, want := range wantKinds {
		if message.Body[index].Kind != want {
			t.Fatalf("body[%d] kind=%s want %s", index, message.Body[index].Kind, want)
		}
	}
	packed := message.Body[3]
	if len(packed.Children) != 2 || packed.Children[0].Start != packed.ValueStart || packed.Children[0].Start != packed.Children[0].ValueStart || packed.Children[1].Start != packed.Children[0].End {
		t.Fatalf("packed offsets/children: %#v", packed)
	}
	for index, want := range []uint64{12, 13} {
		got, scalarErr := decodeSteamScalar(body[packed.Children[index].ValueStart-message.BodyStart:packed.Children[index].End-message.BodyStart], "u64")
		if scalarErr != nil || got != want {
			t.Fatalf("packed[%d]=%v err=%v want=%d", index, got, scalarErr, want)
		}
	}

	// Repeated enums accept both encodings; unknown packed elements stay
	// explicit instead of being dropped.
	streamBody := steamTestVarintField(1, 1)
	streamBody = append(streamBody, steamTestVarintField(14, 6)...)
	streamBody = append(streamBody, steamTestBytesField(14, []byte{0, 99})...)
	message, err = decodeSteamDiscovery(steamTestFrameType(5, streamBody))
	if err != nil {
		t.Fatal(err)
	}
	if len(message.Body) != 3 || message.Body[1].Kind != "i32" || message.Body[2].Kind != "packed" || len(message.Body[2].Children) != 2 || !strings.Contains(message.Body[2].Children[1].Name, "unknown enum") {
		t.Fatalf("enum packed/unpacked handling: %#v", message.Body)
	}
}

func TestSteamDiscoveryUnknownFieldsMessagesAndGroups(t *testing.T) {
	body := steamTestVarintField(40, 1)
	body = append(body, steamTestFixedField(41, 1, make([]byte, 8))...)
	body = append(body, steamTestBytesField(42, []byte{1, 2})...)
	body = append(body, steamTestTag(43, 3)...)
	body = append(body, steamTestVarintField(1, 3)...)
	body = append(body, steamTestTag(44, 3)...)
	body = append(body, steamTestTag(44, 4)...)
	body = append(body, steamTestTag(43, 4)...)
	body = append(body, steamTestFixedField(45, 5, make([]byte, 4))...)
	message, err := decodeSteamDiscovery(steamTestFrameType(17, body))
	if err != nil {
		t.Fatal(err)
	}
	if message.MessageType != 17 || len(message.Body) != 5 {
		t.Fatalf("unknown message: %#v", message)
	}
	for _, field := range message.Body {
		if !strings.Contains(field.Name, "unknown message type 17") {
			t.Fatalf("unknown body field lacks message annotation: %#v", field)
		}
	}
	group := message.Body[3]
	if group.Kind != "group" || len(group.Children) != 3 || group.Children[2].Wire != 4 || group.Children[2].End != group.End {
		t.Fatalf("group/end tag AST: %#v", group)
	}
	steamTestCheckPartition(t, message.Body, message.BodyStart, message.BodyEnd)
	if got := steamTestCountFields(message.Header) + steamTestCountFields(message.Body); got != message.Fields {
		t.Fatalf("group field count=%d AST=%d", message.Fields, got)
	}

	for _, invalid := range [][]byte{
		append(steamTestTag(1, 3), steamTestTag(2, 4)...),
		steamTestTag(1, 3),
		steamTestTag(1, 4),
	} {
		steamTestRequireError(t, steamTestFrameType(0, invalid))
	}
}

func steamTestNestedGroups(depth int) []byte {
	var out []byte
	for level := 1; level <= depth; level++ {
		out = append(out, steamTestTag(uint32(level), 3)...)
	}
	for level := depth; level >= 1; level-- {
		out = append(out, steamTestTag(uint32(level), 4)...)
	}
	return out
}

func TestSteamDiscoveryRecursionAndSharedBudget(t *testing.T) {
	message, err := decodeSteamDiscovery(steamTestFrameType(0, steamTestNestedGroups(steamDiscoveryMaxDepth)))
	if err != nil {
		t.Fatalf("exact depth limit: %v", err)
	}
	if message.Fields != 1+2*steamDiscoveryMaxDepth {
		t.Fatalf("depth field count=%d", message.Fields)
	}
	steamTestRequireError(t, steamTestFrameType(0, steamTestNestedGroups(steamDiscoveryMaxDepth+1)))

	twoByteField := []byte{0x08, 0x00}
	for _, tc := range []struct {
		headerFields int
		bodyFields   int
		ok           bool
	}{
		{0, 4096, true},
		{1, 4095, true},
		{1, 4096, false},
	} {
		header := bytes.Repeat(twoByteField, tc.headerFields)
		body := bytes.Repeat(twoByteField, tc.bodyFields)
		decoded, decodeErr := decodeSteamDiscovery(steamTestFrame(header, body))
		if tc.ok {
			if decodeErr != nil || decoded.Fields != steamDiscoveryMaxFields {
				t.Fatalf("header=%d body=%d: message=%#v err=%v", tc.headerFields, tc.bodyFields, decoded, decodeErr)
			}
		} else if decodeErr == nil || decoded != nil {
			t.Fatalf("header=%d body=%d exceeded shared budget", tc.headerFields, tc.bodyFields)
		}
	}

	packed := bytes.Repeat([]byte{0}, 4095)
	message, err = decodeSteamDiscovery(steamTestFrame(nil, steamTestBytesField(2, packed)))
	if err != nil || message.Fields != 4096 {
		t.Fatalf("exact packed budget: message=%#v err=%v", message, err)
	}
	steamTestRequireError(t, steamTestFrame(nil, steamTestBytesField(2, append(packed, 0))))
}

func TestSteamDiscoveryScalarConversionsAndNegativeInt32(t *testing.T) {
	negativeValue := int64(-123)
	negative := steamTestVarint(uint64(negativeValue))
	body := append(steamTestTag(1, 0), negative...)
	message, err := decodeSteamDiscovery(steamTestFrameType(1, body))
	if err != nil {
		t.Fatal(err)
	}
	field := message.Body[0]
	value, err := decodeSteamScalar(steamTestFrameType(1, body)[field.ValueStart:field.End], "i32")
	if err != nil || value != int32(-123) || field.End-field.ValueStart != 10 {
		t.Fatalf("negative int32=%v width=%d err=%v", value, field.End-field.ValueStart, err)
	}

	for _, tc := range []struct {
		data []byte
		kind string
		want any
	}{
		{steamTestVarint(math.MaxUint64), "u64", uint64(math.MaxUint64)},
		{steamTestVarint(math.MaxUint32), "u32", uint32(math.MaxUint32)},
		{[]byte{2}, "bool", true},
		{[]byte{0}, "bool", false},
		{[]byte{1, 2, 3, 4, 5, 6, 7, 8}, "fixed64", uint64(0x0807060504030201)},
		{[]byte{0, 0, 0x80, 0x3f}, "float32", float32(1)},
		{[]byte("hello"), "string", "hello"},
	} {
		got, scalarErr := decodeSteamScalar(tc.data, tc.kind)
		if scalarErr != nil || got != tc.want {
			t.Fatalf("%s(%x)=%#v err=%v want=%#v", tc.kind, tc.data, got, scalarErr, tc.want)
		}
	}
	raw := []byte{1, 2}
	copyValue, err := decodeSteamScalar(raw, "raw")
	if err != nil {
		t.Fatal(err)
	}
	raw[0] = 9
	if !bytes.Equal(copyValue.([]byte), []byte{1, 2}) {
		t.Fatal("raw scalar retained caller storage")
	}
	for _, tc := range []struct {
		data []byte
		kind string
	}{
		{append(steamTestVarint(1), 0), "u64"},
		{steamTestVarint(uint64(math.MaxUint32) + 1), "u32"},
		{[]byte{1}, "fixed64"},
		{[]byte{1}, "float32"},
		{[]byte{0xff}, "string"},
		{[]byte{0}, "message"},
	} {
		if got, scalarErr := decodeSteamScalar(tc.data, tc.kind); scalarErr == nil {
			t.Fatalf("%s(%x) unexpectedly decoded as %#v", tc.kind, tc.data, got)
		}
	}
}

var steamDiscoveryBenchmarkSink *steamDiscoveryMessage

func BenchmarkDecodeSteamDiscovery(b *testing.B) {
	body := steamTestSchemaMessage(steamStreamingRequestSchema)
	wire := steamTestFrameType(5, body)
	b.ReportAllocs()
	b.SetBytes(int64(len(wire)))
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		message, err := decodeSteamDiscovery(wire)
		if err != nil {
			b.Fatal(err)
		}
		steamDiscoveryBenchmarkSink = message
	}
}

func BenchmarkDecodeSteamDiscoveryFieldLimit(b *testing.B) {
	wire := steamTestFrame(nil, bytes.Repeat([]byte{0x08, 0}, steamDiscoveryMaxFields))
	b.ReportAllocs()
	b.SetBytes(int64(len(wire)))
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		message, err := decodeSteamDiscovery(wire)
		if err != nil {
			b.Fatal(err)
		}
		steamDiscoveryBenchmarkSink = message
	}
}
