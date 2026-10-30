package sharkcli

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func smbDetailTestClone(value any) any {
	switch value := value.(type) {
	case map[string]any:
		if value == nil {
			return map[string]any(nil)
		}
		out := make(map[string]any, len(value))
		for key, child := range value {
			out[key] = smbDetailTestClone(child)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(value))
		for i, child := range value {
			out[i] = smbDetailTestClone(child).(map[string]any)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, child := range value {
			out[i] = smbDetailTestClone(child)
		}
		return out
	case []byte:
		return bytes.Clone(value)
	default:
		return value
	}
}

func smbDetailTestLiteral(owner map[string]any, name string, value any, start, end uint64) {
	ranges, _ := owner["Wire Field Ranges"].(map[string]any)
	if ranges == nil {
		ranges = map[string]any{}
		owner["Wire Field Ranges"] = ranges
	}
	owner[name] = value
	ranges[name] = []any{start, end}
}

// This fixture checks projection contracts independently of protocol admission.
// Native session fixtures below separately verify real parser output and ranges.
func smbDetailTestEvent() *pcaputil.ProtocolEvent {
	raw := make([]byte, 156)
	raw[3] = byte(len(raw) - 4)
	copy(raw[4:8], []byte{0xfe, 'S', 'M', 'B'})
	header, body := map[string]any{}, map[string]any{}
	smbDetailTestLiteral(header, "Protocol ID", uint32(0x424d53fe), 4, 8)
	smbDetailTestLiteral(header, "Structure Size", uint16(64), 8, 10)
	smbDetailTestLiteral(header, "Command", uint16(5), 16, 18)
	smbDetailTestLiteral(header, "Tree ID", uint32(7), 40, 44)
	smbDetailTestLiteral(header, "Session ID", uint64(17), 44, 52)
	smbDetailTestLiteral(header, "Signature", make([]byte, 16), 52, 68)
	smbDetailTestLiteral(body, "Structure Size", uint16(89), 68, 70)
	smbDetailTestLiteral(body, "FileId", strings.Repeat("12", 16), 132, 148)
	fields := smbDetailTestClone(header).(map[string]any)
	fields["Header"], fields["Body"] = header, body
	fields["Packet Name"], fields["Response"] = "CREATE", true
	smbDetailTestLiteral(fields, "FileId", body["FileId"], 132, 148)
	return &pcaputil.ProtocolEvent{ID: 1, Protocol: "smb2", Transport: "tcp", Status: "decoded", Raw: raw, Length: len(raw), Fields: fields, SourceBytes: pcaputil.ByteSource{Kind: "reassembled"}}
}

func smbDetailTestLeafCount(fields []field, name string) int {
	count := 0
	for _, item := range fields {
		if !item.branch && strings.HasPrefix(item.text, name+":") {
			count++
		}
	}
	return count
}

func smbDetailTestBranchCount(fields []field, name string) int {
	count := 0
	for _, item := range fields {
		if item.branch && (item.text == name || strings.HasPrefix(item.text, name+" · ")) {
			count++
		}
	}
	return count
}

func TestSMBDisplayAliasDedupRequiresExactTypeValueAndRange(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*pcaputil.ProtocolEvent)
	}{
		{"equal", func(*pcaputil.ProtocolEvent) {}},
		{"integer-width", func(e *pcaputil.ProtocolEvent) { e.Fields["Command"] = uint32(5) }},
		{"integer-float", func(e *pcaputil.ProtocolEvent) { e.Fields["Command"] = float64(5) }},
		{"value", func(e *pcaputil.ProtocolEvent) { e.Fields["Command"] = uint16(6) }},
		{"range-position", func(e *pcaputil.ProtocolEvent) {
			e.Fields["Wire Field Ranges"].(map[string]any)["Command"] = []any{uint64(18), uint64(20)}
		}},
		{"range-integer-width", func(e *pcaputil.ProtocolEvent) {
			e.Fields["Wire Field Ranges"].(map[string]any)["Command"] = []any{uint32(16), uint64(18)}
		}},
		{"missing-range", func(e *pcaputil.ProtocolEvent) { delete(e.Fields["Wire Field Ranges"].(map[string]any), "Command") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := smbDetailTestEvent()
			tc.mutate(e)
			beforeFields := smbDetailTestClone(e.Fields)
			beforeRaw := bytes.Clone(e.Raw)
			projected := protocolDisplayEvent(e)
			_, retained := projected.Fields["Command"]
			require.Equal(t, tc.name != "equal", retained)
			if retained {
				require.True(t, reflect.DeepEqual(e.Fields["Command"], projected.Fields["Command"]))
				require.True(t, reflect.DeepEqual(smbDisplayRange(e.Fields, "Command"), smbDisplayRange(projected.Fields, "Command")))
			}
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			want := 2
			if tc.name == "equal" {
				want = 1
			}
			require.Equal(t, want, smbDetailTestLeafCount(fields, "Command"))
			require.Len(t, sources, 1)
			for _, stream := range []bool{false, true} {
				got := workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, stream)
				require.Equal(t, want, smbDetailTestLeafCount(got.fields, "Command"), "JSON number folding cannot authorize a second equality decision")
				require.Equal(t, e.Raw, got.sources[0].Data)
			}
			require.True(t, reflect.DeepEqual(beforeFields, e.Fields), "presentation must not mutate original fields or range types")
			require.Equal(t, beforeRaw, e.Raw)
		})
	}
}

func TestSMBDisplayOctetAliasesDoNotEqualTextOrDifferentTails(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"equal", make([]byte, 16)},
		{"base64-text", "AAAAAAAAAAAAAAAAAAAAAA=="},
		{"different-tail", append(make([]byte, 15), byte(1))},
		{"nil", []byte(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := smbDetailTestEvent()
			e.Fields["Signature"] = tc.value
			projected := protocolDisplayEvent(e)
			_, retained := projected.Fields["Signature"]
			require.Equal(t, tc.name != "equal", retained)
			for _, stream := range []bool{false, true} {
				got := workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, stream)
				want := 2
				if tc.name == "equal" {
					want = 1
				}
				require.Equal(t, want, smbDetailTestLeafCount(got.fields, "Signature"))
			}
		})
	}
}

func TestSMBDisplayFieldsAndSessionCopiesNeedSameTypedOwner(t *testing.T) {
	for _, distinct := range []string{"equal", "integer-width", "value", "range-position", "range-integer-width", "outer-range-only", "missing-owner"} {
		t.Run(distinct, func(t *testing.T) {
			e := smbDetailTestEvent()
			e.Session = smbDetailTestClone(e.Fields).(map[string]any)
			header := e.Session["Header"].(map[string]any)
			switch distinct {
			case "integer-width":
				header["Command"] = uint32(5)
			case "value":
				header["Command"] = uint16(6)
			case "range-position":
				header["Wire Field Ranges"].(map[string]any)["Command"] = []any{uint64(18), uint64(20)}
			case "range-integer-width":
				header["Wire Field Ranges"].(map[string]any)["Command"] = []any{uint32(16), uint64(18)}
			case "outer-range-only":
				e.Session["Wire Field Ranges"].(map[string]any)["Header"] = []any{uint64(4), uint64(68)}
			case "missing-owner":
				delete(header["Wire Field Ranges"].(map[string]any), "Protocol ID")
			}
			beforeFields, beforeSession := smbDetailTestClone(e.Fields), smbDetailTestClone(e.Session)
			want := 2
			if distinct == "equal" {
				want = 1
			}
			fields, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			require.Equal(t, want, smbDetailTestBranchCount(fields, "Header"))
			for _, stream := range []bool{false, true} {
				got := workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, stream)
				require.Equal(t, want, smbDetailTestBranchCount(got.fields, "Header"))
			}
			require.True(t, reflect.DeepEqual(beforeFields, e.Fields))
			require.True(t, reflect.DeepEqual(beforeSession, e.Session))
		})
	}
}

func TestSMBDetailQualifiesOnlyRetainedDuplicateSections(t *testing.T) {
	for _, distinct := range []bool{false, true} {
		t.Run(fmt.Sprint(distinct), func(t *testing.T) {
			e := smbDetailTestEvent()
			e.Session = smbDetailTestClone(e.Fields).(map[string]any)
			if distinct {
				e.Session["Header"].(map[string]any)["Command"] = uint32(5)
			}
			localFields, localSources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			for _, got := range []packetDetail{{fields: localFields, sources: localSources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)} {
				branches := map[string]bool{}
				for _, item := range got.fields {
					if item.branch {
						branches[item.text] = true
					}
				}
				require.True(t, branches["Direct TCP"], "one carrier remains one protocol-named branch")
				require.Equal(t, 1, smbDetailTestBranchCount(got.fields, "Direct TCP"))
				require.True(t, branches["Body"], "a single body needs no Fields/Session qualifier")
				if distinct {
					require.True(t, branches["Header · Fields"])
					require.True(t, branches["Header · Session"])
					require.False(t, branches["Header"], "two unproved observations keep explicit origins")
				} else {
					require.True(t, branches["Header"])
					for name := range branches {
						require.NotContains(t, name, " · Fields")
						require.NotContains(t, name, " · Session")
					}
				}
			}
		})
	}
}

func TestSMBManualWorkerCannotDeduplicateUnprovedJSONEquality(t *testing.T) {
	e := smbDetailTestEvent()
	e.Session = smbDetailTestClone(e.Fields).(map[string]any)
	for _, stream := range []bool{false, true} {
		request := decodeRequest{Number: 1, Events: []*pcaputil.ProtocolEvent{e}}
		if stream {
			request.Events, request.Stream = nil, &streamDecodeRequest{ID: 1, Events: []*pcaputil.ProtocolEvent{e}}
		}
		wire, err := json.Marshal(request) // Deliberately bypass the typed parent projection.
		require.NoError(t, err)
		var output bytes.Buffer
		require.NoError(t, runDecodeWorker(bytes.NewReader(wire), &output))
		got, err := decodeWorkerOutput(output.Bytes(), &capturedPacket{number: 1})
		require.NoError(t, err)
		require.Equal(t, 2, smbDetailTestBranchCount(got.fields, "Header"))
		require.Equal(t, 4, smbDetailTestLeafCount(got.fields, "Command"))
	}
}

func TestSMBDisplayCopyPreservesAuthenticationPresentation(t *testing.T) {
	e := smbDetailTestEvent()
	e.Fields["Command"] = uint16(1)
	e.Fields["Header"].(map[string]any)["Command"] = uint16(1)
	e.Fields["Packet Name"] = "SESSION_SETUP"
	e.Fields["Authentication"] = map[string]any{"Status": "decoded", "Protocol": "SPNEGO", "Mechanism": map[string]any{"Status": "decoded", "Protocol": "NTLMSSP", "Message Name": "NEGOTIATE"}}
	e.Session = smbDetailTestClone(e.Fields).(map[string]any)
	before := applicationEventSummary(e)
	projected := protocolDisplayEvent(e)
	require.Equal(t, "NTLMSSP", eventProtocolName(projected))
	require.Equal(t, []string{"SPNEGO", "NTLMSSP"}, authenticationLayers(projected))
	require.Equal(t, before, applicationEventSummary(projected))
	for _, stream := range []bool{false, true} {
		got := workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, stream)
		require.Contains(t, detailLines(got.fields), "NTLMSSP · PDU #1")
		require.Contains(t, detailLines(got.fields), "authentication not verified")
		require.Contains(t, got.sources[0].Name, "SMB2 wire PDU (authentication)")
		require.Equal(t, 1, smbDetailTestBranchCount(got.fields, "Header"))
	}
	require.IsType(t, uint16(0), e.Fields["Command"], "original SDK aliases survive the presentation copy")
}

func TestSMBDisplayDedupNeverCrossesPDUOrBorrowsContextRanges(t *testing.T) {
	first, second := smbDetailTestEvent(), smbDetailTestEvent()
	second.ID, second.SourceBytes.Kind = 2, "captured"
	for _, e := range []*pcaputil.ProtocolEvent{first, second} {
		e.Session = map[string]any{"Connection Dialect": uint16(0x0311), "Retained identity": map[string]any{"Session ID": uint64(17)}}
	}
	for _, stream := range []bool{false, true} {
		got := workerMetadataDetail(t, []*pcaputil.ProtocolEvent{first, second}, stream)
		require.Len(t, got.sources, 2, "identical raw bytes in two PDU events still have separate owners")
		require.Equal(t, 2, smbDetailTestLeafCount(got.fields, "Command"))
		seenSources, unlocated := map[int]bool{}, 0
		for _, f := range got.fields {
			if strings.HasPrefix(f.text, "Command:") {
				require.True(t, f.hasRange)
				require.Equal(t, 16, f.start)
				require.Equal(t, 18, f.end)
				seenSources[f.source] = true
			}
			if strings.HasPrefix(f.text, "Connection Dialect:") || strings.HasPrefix(f.text, "Session ID:") && !f.hasRange {
				require.False(t, f.hasRange)
				require.Zero(t, f.source)
				unlocated++
			}
		}
		require.Len(t, seenSources, 2)
		require.Equal(t, 4, unlocated)
	}
}

func smbDetailTestDirect(payload []byte) []byte {
	out := append(make([]byte, 4), payload...)
	out[1], out[2], out[3] = byte(len(payload)>>16), byte(len(payload)>>8), byte(len(payload))
	return out
}

func smbDetailTestHeader(command uint16, flags uint32, mid, sid uint64, tid uint32, body []byte) []byte {
	out := make([]byte, 64+len(body))
	copy(out, []byte{0xfe, 'S', 'M', 'B'})
	binary.LittleEndian.PutUint16(out[4:6], 64)
	binary.LittleEndian.PutUint16(out[12:14], command)
	binary.LittleEndian.PutUint32(out[16:20], flags)
	binary.LittleEndian.PutUint64(out[24:32], mid)
	binary.LittleEndian.PutUint32(out[36:40], tid)
	binary.LittleEndian.PutUint64(out[40:48], sid)
	copy(out[64:], body)
	return out
}

func smbDetailTestNativeEvents(t *testing.T, wires ...[]byte) []*pcaputil.ProtocolEvent {
	t.Helper()
	session, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { session.Close("FIN") })
	var events []*pcaputil.ProtocolEvent
	for _, wire := range wires {
		result := session.Feed(0, time.Unix(1, 0), wire)
		require.Nil(t, result.Err, "%v", result.Err)
		require.Equal(t, len(wire), result.Consumed)
		for _, event := range result.Events {
			fields, err := event.GetFields()
			require.NoError(t, err)
			snapshot := *event
			snapshot.Fields = fields
			events = append(events, &snapshot)
		}
	}
	return events
}

func smbDetailTestSMB1Offer() []byte {
	var dialects []byte
	for _, name := range []string{"NT LM 0.12", "SMB 2.002", "SMB 2.???"} {
		dialects = append(append(dialects, byte(2)), []byte(name)...)
		dialects = append(dialects, 0)
	}
	payload := make([]byte, 35+len(dialects))
	copy(payload, []byte{0xff, 'S', 'M', 'B'})
	payload[4] = 0x72
	binary.LittleEndian.PutUint16(payload[10:12], 0x4000)
	binary.LittleEndian.PutUint16(payload[12:14], 0x1234)
	binary.LittleEndian.PutUint16(payload[26:28], 0x5678)
	binary.LittleEndian.PutUint16(payload[33:35], uint16(len(dialects)))
	copy(payload[35:], dialects)
	return smbDetailTestDirect(payload)
}

func TestSMBNativeSMB1HeaderPrecedesEveryDialectLocallyAndThroughWorkers(t *testing.T) {
	events := smbDetailTestNativeEvents(t, smbDetailTestSMB1Offer())
	require.Len(t, events, 1)
	e := events[0]
	require.Equal(t, "smb", e.Protocol)
	beforeFields, beforeSession := smbDetailTestClone(e.Fields), smbDetailTestClone(e.Session)
	localFields, localSources := protocolEventDetail(events)
	views := []packetDetail{{fields: localFields, sources: localSources}, workerMetadataDetail(t, events, false), workerMetadataDetail(t, events, true)}
	for _, got := range views {
		require.Equal(t, 1, smbDetailTestBranchCount(got.fields, "Direct TCP"))
		direct, header, body, pidHigh, firstDialect := -1, -1, -1, -1, -1
		for i, f := range got.fields {
			if strings.HasPrefix(f.text, "Direct TCP") && f.branch && direct < 0 {
				direct = i
			}
			if strings.HasPrefix(f.text, "Header") && f.branch && header < 0 {
				header = i
			}
			if strings.HasPrefix(f.text, "Body") && f.branch && body < 0 {
				body = i
			}
			if strings.HasPrefix(f.text, "Dialects[") && firstDialect < 0 {
				firstDialect = i
			}
			if strings.HasPrefix(f.text, "PID High:") {
				pidHigh = i
				require.True(t, f.hasRange)
				require.Equal(t, 16, f.start)
				require.Equal(t, 18, f.end)
				require.Equal(t, []byte{0x34, 0x12}, got.sources[f.source-1].Data[f.start:f.end])
			}
			require.NotContains(t, f.text, "Wire Field Ranges")
		}
		require.GreaterOrEqual(t, direct, 0)
		require.Greater(t, header, direct)
		require.Greater(t, pidHigh, header)
		require.Greater(t, body, pidHigh)
		require.Greater(t, firstDialect, body)
		require.Equal(t, 1, smbDetailTestBranchCount(got.fields, "Header"))
		require.Equal(t, 3, smbDetailTestLeafCount(got.fields, "Name"))
		require.Len(t, got.sources, 1)
		require.Equal(t, e.Raw, got.sources[0].Data)
	}
	require.True(t, reflect.DeepEqual(beforeFields, e.Fields))
	require.True(t, reflect.DeepEqual(beforeSession, e.Session))
}

func smbDetailTestCompoundRequest() ([]byte, []int) {
	name := utf16.Encode([]rune("a.txt"))
	create := make([]byte, 56+2*len(name))
	binary.LittleEndian.PutUint16(create[:2], 57)
	binary.LittleEndian.PutUint16(create[44:46], 120)
	binary.LittleEndian.PutUint16(create[46:48], uint16(2*len(name)))
	for i, char := range name {
		binary.LittleEndian.PutUint16(create[56+2*i:], char)
	}
	read := make([]byte, 49)
	binary.LittleEndian.PutUint16(read[:2], 49)
	binary.LittleEndian.PutUint32(read[4:8], 1)
	copy(read[16:32], bytes.Repeat([]byte{0xff}, 16))
	closeBody := make([]byte, 24)
	binary.LittleEndian.PutUint16(closeBody[:2], 24)
	copy(closeBody[8:24], bytes.Repeat([]byte{0xff}, 16))
	commands := [][]byte{smbDetailTestHeader(5, 0, 10, 17, 7, create), smbDetailTestHeader(8, 4, 11, ^uint64(0), ^uint32(0), read), smbDetailTestHeader(6, 4, 12, ^uint64(0), ^uint32(0), closeBody)}
	var payload []byte
	var starts []int
	for i, command := range commands {
		starts = append(starts, 4+len(payload))
		if i < len(commands)-1 {
			for len(command)%8 != 0 {
				command = append(command, 0)
			}
			binary.LittleEndian.PutUint32(command[20:24], uint32(len(command)))
		}
		payload = append(payload, command...)
	}
	return smbDetailTestDirect(payload), starts
}

func TestSMBNativeSMB2SingleCommandKeepsHeaderBodyOrderAndOwnRanges(t *testing.T) {
	body := make([]byte, 40)
	binary.LittleEndian.PutUint16(body[:2], 36)
	binary.LittleEndian.PutUint16(body[2:4], 2)
	binary.LittleEndian.PutUint16(body[36:38], 0x0202)
	binary.LittleEndian.PutUint16(body[38:40], 0x0210)
	events := smbDetailTestNativeEvents(t, smbDetailTestDirect(smbDetailTestHeader(0, 0, 0, 0, 0, body)))
	require.Len(t, events, 1)
	e := events[0]
	require.IsType(t, []uint16{}, e.Fields["Dialects"])
	localFields, localSources := protocolEventDetail(events)
	for _, got := range []packetDetail{{fields: localFields, sources: localSources}, workerMetadataDetail(t, events, false), workerMetadataDetail(t, events, true)} {
		require.Equal(t, 1, smbDetailTestBranchCount(got.fields, "Header"))
		header, bodyIndex, firstDialect := -1, -1, -1
		var structureStarts []int
		for i, f := range got.fields {
			if f.branch && strings.HasPrefix(f.text, "Header") {
				header = i
			}
			if f.branch && strings.HasPrefix(f.text, "Body") && bodyIndex < 0 {
				bodyIndex = i
			}
			if strings.HasPrefix(f.text, "Dialect:") && firstDialect < 0 {
				firstDialect = i
			}
			if strings.HasPrefix(f.text, "Structure Size:") {
				require.True(t, f.hasRange)
				require.Equal(t, 1, f.source)
				structureStarts = append(structureStarts, f.start)
			}
		}
		require.GreaterOrEqual(t, header, 0)
		require.Greater(t, bodyIndex, header)
		require.Greater(t, firstDialect, bodyIndex)
		require.Equal(t, []int{8, 68}, structureStarts, "Header.StructureSize and Body.StructureSize are separate wire occurrences")
		require.Len(t, got.sources, 1)
		require.Equal(t, e.Raw, got.sources[0].Data)
	}
	require.IsType(t, []uint16{}, e.Fields["Dialects"], "the SDK compatibility slice is not rewritten into body entries")
}

func TestSMBNativeCompoundShowsEachHeaderAndOwnRangeOnce(t *testing.T) {
	wire, starts := smbDetailTestCompoundRequest()
	events := smbDetailTestNativeEvents(t, wire)
	require.Len(t, events, 1)
	e := events[0]
	require.Equal(t, "smb2", e.Protocol)
	require.Equal(t, 3, e.Session["Compound Count"])
	before, err := json.Marshal(e)
	require.NoError(t, err)
	localFields, localSources := protocolEventDetail(events)
	for _, got := range []packetDetail{{fields: localFields, sources: localSources}, workerMetadataDetail(t, events, false), workerMetadataDetail(t, events, true)} {
		require.Len(t, got.sources, 1)
		require.Equal(t, 1, smbDetailTestBranchCount(got.fields, "Direct TCP"))
		require.Equal(t, wire, got.sources[0].Data)
		require.Equal(t, 3, smbDetailTestBranchCount(got.fields, "Header"), "the root copy of the first command must not add a fourth header")
		require.Equal(t, 3, smbDetailTestLeafCount(got.fields, "Command"))
		for ordinal := 0; ordinal < 3; ordinal++ {
			commandIndex, headerIndex, bodyIndex, contextIndex := -1, -1, -1, -1
			for i, f := range got.fields {
				if f.branch && f.text == fmt.Sprintf("Commands[%d]", ordinal) {
					commandIndex = i
					continue
				}
				if commandIndex < 0 {
					continue
				}
				if f.branch && strings.HasPrefix(f.text, "Commands[") {
					break
				}
				if f.branch && f.text == "Header" {
					headerIndex = i
				}
				if f.branch && f.text == "Body" {
					bodyIndex = i
				}
				if f.branch && f.text == "Observed context" {
					contextIndex = i
				}
			}
			require.GreaterOrEqual(t, commandIndex, 0)
			require.Greater(t, headerIndex, commandIndex)
			require.Greater(t, bodyIndex, headerIndex)
			if contextIndex >= 0 {
				require.Greater(t, contextIndex, bodyIndex)
			}
		}
		var commandStarts []int
		var fileIDStarts []int
		for _, f := range got.fields {
			if f.branch {
				require.NotContains(t, f.text, " · Fields", "the deduplicated compound has one canonical source")
				require.NotContains(t, f.text, " · Session", "the deduplicated compound has one canonical source")
			}
			if strings.HasPrefix(f.text, "Command:") {
				require.True(t, f.hasRange)
				require.Equal(t, 1, f.source)
				require.Equal(t, 2, f.end-f.start)
				commandStarts = append(commandStarts, f.start)
			}
			if strings.HasPrefix(f.text, "FileId:") && f.hasRange {
				fileIDStarts = append(fileIDStarts, f.start)
			}
			if strings.HasPrefix(f.text, "Effective Session ID:") || strings.HasPrefix(f.text, "Effective Tree ID:") || strings.HasPrefix(f.text, "Connection Dialect:") {
				require.False(t, f.hasRange, "derived connection context has no current-PDU literal range")
			}
		}
		require.Equal(t, []int{starts[0] + 12, starts[1] + 12, starts[2] + 12}, commandStarts)
		// CLOSE has an explicit owned range. READ remains visible even when
		// this native profile has not yet published its FileId range.
		require.Contains(t, fileIDStarts, starts[2]+72)
		require.GreaterOrEqual(t, smbDetailTestLeafCount(got.fields, "FileId"), 2)
	}
	after, err := json.Marshal(e)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestSMBCompoundRootCopiesWithDifferentEvidenceRemainVisible(t *testing.T) {
	wire, _ := smbDetailTestCompoundRequest()
	for _, distinct := range []string{"integer-width", "value", "range-position", "other-command-owner"} {
		t.Run(distinct, func(t *testing.T) {
			e := smbDetailTestNativeEvents(t, wire)[0]
			e.Fields = smbDetailTestClone(e.Fields).(map[string]any)
			e.Session = nil
			header := e.Fields["Header"].(map[string]any)
			switch distinct {
			case "integer-width":
				header["Command"] = uint32(5)
			case "value":
				header["Command"] = uint16(6)
			case "range-position":
				header["Wire Field Ranges"].(map[string]any)["Command"] = []any{uint64(18), uint64(20)}
			case "other-command-owner":
				commands := smbDisplayCommands(e.Fields["Commands"])
				second := commands[1]["Header"].(map[string]any)
				header["Wire Field Ranges"].(map[string]any)["Protocol ID"] = smbDisplayRange(second, "Protocol ID")
			}
			projected := protocolDisplayEvent(e)
			require.Contains(t, projected.Fields, "Header", "a differing first-command root copy is not a duplicate")
			fields, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			require.Equal(t, 4, smbDetailTestBranchCount(fields, "Header"))
			for _, stream := range []bool{false, true} {
				got := workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, stream)
				require.Equal(t, 4, smbDetailTestBranchCount(got.fields, "Header"))
			}
		})
	}
}

func TestSMBDetailKeepsUnlocatedBytesSeparateAndReportsViewBudget(t *testing.T) {
	e := smbDetailTestEvent()
	e.Session = map[string]any{"Unlocated signature": bytes.Repeat([]byte{0xaa}, 16)}
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	require.Len(t, sources, 2)
	require.Equal(t, bytes.Repeat([]byte{0xaa}, 16), sources[1].Data)
	require.Contains(t, sources[1].Name, "frame offsets unavailable")
	found := false
	for _, f := range fields {
		if strings.HasPrefix(f.text, "Unlocated signature:") {
			found = true
			require.Equal(t, 2, f.source)
			require.Equal(t, 0, f.start)
			require.Equal(t, 16, f.end)
		}
	}
	require.True(t, found)
	list := make([]any, 4200)
	for i := range list {
		list[i] = fmt.Sprintf("observation-%d", i)
	}
	e.Session["Large context"] = list
	fields, _ = protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	require.Contains(t, detailLines(fields), "Field view limited")
	require.LessOrEqual(t, len(fields), 4100)
	// The fixture's hex FileId remains the exact SDK string, not a byte cast.
	require.Equal(t, hex.EncodeToString(bytes.Repeat([]byte{0x12}, 16)), e.Fields["FileId"])
}
