package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func smb2ErrorResponse(command uint16, mid, sid uint64, tid uint32, status uint32, data []byte) []byte {
	header := smb2Hdr(command, smb2FlagResp, mid, sid, tid)
	binary.LittleEndian.PutUint32(header[8:12], status)
	body := make([]byte, 8+len(data))
	binary.LittleEndian.PutUint16(body[:2], 9)
	binary.LittleEndian.PutUint32(body[4:8], uint32(len(data)))
	copy(body[8:], data)
	return smb2TCP(append(header, body...))
}

func TestSMB2NegotiateErrorAndTruncationCannotSelectDialect(t *testing.T) {
	state := &binSMB2{client: 0, dialect: 0x0210}
	_, err := state.consumeFrom(0, smb2NegotiateReq(0x0311), 64)
	require.NoError(t, err)
	// Error.ByteCount has the same offset as the success Dialect field.
	failure := smb2ErrorResponse(0, 1, 0, 0, 0xc000000d, bytes.Repeat([]byte{0xa5}, 0x0311))
	info, err := state.consumeFrom(1, failure, 64)
	require.NoError(t, err)
	require.NotContains(t, info, "Dialect")
	require.NotContains(t, info, "Dialect Context Updated")
	require.Equal(t, true, info["Error Response"])
	require.Equal(t, uint32(0x0311), info["Body"].(map[string]any)["Byte Count"])
	require.Equal(t, uint16(0x0210), state.dialect)
	require.Empty(t, state.pending, "a complete error still finishes the matched request")

	valid := smb2NegotiateResp(0x0311)[4:]
	for end := 64; end < len(valid); end++ {
		t.Run(fmt.Sprintf("success-cut-%d", end), func(t *testing.T) {
			state := &binSMB2{client: 0, dialect: 0x0210}
			_, err := state.consumeFrom(0, smb2NegotiateReq(0x0311), 64)
			require.NoError(t, err)
			_, err = state.consumeFrom(1, smb2TCP(bytes.Clone(valid[:end])), 64)
			require.Error(t, err)
			require.Equal(t, uint16(0x0210), state.dialect)
			require.Len(t, state.pending, 1, "invalid success must not consume request state")
		})
	}
}

func TestSMB2NegotiateDeclaredCollectionsAndBuffersAreComplete(t *testing.T) {
	request := smb2NegotiateReq(0x0202, 0x0311)[4:]
	for _, cut := range []int{66, 99, 100, 101, 102, 103} {
		_, err := smb2Walk(request[:cut], 64)
		require.Error(t, err, "declared two dialects must not become a partial accepted list at %d", cut)
	}
	_, err := smb2Walk(request, 1)
	require.Error(t, err, "the collection budget applies inside one command")
	badSize := bytes.Clone(request)
	binary.LittleEndian.PutUint16(badSize[64:66], 35)
	_, err = smb2Walk(badSize, 64)
	require.Error(t, err)

	response := smb2NegotiateResp(0x0311)[4:]
	for _, offset := range []uint16{1, 64, 127, 129, 0xffff} {
		bad := bytes.Clone(response)
		binary.LittleEndian.PutUint16(bad[120:122], offset)
		binary.LittleEndian.PutUint16(bad[122:124], 1)
		_, err := smb2Walk(bad, 64)
		require.Error(t, err, "security buffer offset %d", offset)
	}
	bad := bytes.Clone(response)
	binary.LittleEndian.PutUint16(bad[70:72], 1)
	binary.LittleEndian.PutUint32(bad[124:128], 128)
	_, err = smb2Walk(bad, 64)
	require.Error(t, err, "a declared context needs its complete header and data")
}

func TestSMB2FailedCreateDoesNotOpenFile(t *testing.T) {
	state := &binSMB2{client: 0}
	_, err := state.consumeFrom(0, smb2Create(7, 0x11, 1, "report.txt"), 64)
	require.NoError(t, err)
	failure := smb2ErrorResponse(5, 7, 0x11, 1, 0xc0000022, bytes.Repeat([]byte{0x77}, 96))
	info, err := state.consumeFrom(1, failure, 64)
	require.NoError(t, err)
	require.Equal(t, "matched", info["Association Status"])
	require.NotContains(t, info, "FileId")
	require.Empty(t, state.files)
	require.Empty(t, state.pending)

	fid := bytes.Repeat([]byte{0x77}, 16)
	read, err := state.consumeFrom(0, smb2Read(8, 0x11, 1, fid, 1), 64)
	require.NoError(t, err)
	require.Equal(t, "missing-create", read["File Context Status"])
	valid := smb2CreateResp(9, 0x11, 1, fid)[4:]
	for _, cut := range []int{128, 143, 144, 151} {
		_, err := smb2Walk(valid[:cut], 64)
		require.Error(t, err, "CREATE requires the whole success fixed body, not just FileId, at %d", cut)
	}
}

func TestSMB2NegotiateContextAlignmentAndOwnedBounds(t *testing.T) {
	response := append(smb2NegotiateResp(0x0311)[4:], make([]byte, 10)...)
	binary.LittleEndian.PutUint16(response[70:72], 1)
	binary.LittleEndian.PutUint32(response[124:128], 128)
	binary.LittleEndian.PutUint16(response[128:130], 1)
	binary.LittleEndian.PutUint16(response[130:132], 2)
	copy(response[136:138], []byte{0x34, 0x12})
	commands, err := smb2Walk(response, 64)
	require.NoError(t, err)
	contexts := commands[0]["Body"].(map[string]any)["Negotiate Contexts"].([]map[string]any)
	require.Equal(t, []byte{0x34, 0x12}, contexts[0]["Data"])
	require.Equal(t, []any{uint64(140), uint64(142)}, contexts[0]["Wire Field Ranges"].(map[string]any)["Data"], "alignment is relative to SMB2 header; owned ranges include Direct TCP")
	response[136] = 99
	require.Equal(t, []byte{0x34, 0x12}, contexts[0]["Data"])
	for name, mutate := range map[string]func([]byte){
		"misaligned-offset": func(w []byte) { binary.LittleEndian.PutUint32(w[124:128], 132) },
		"short-data":        func(w []byte) { binary.LittleEndian.PutUint16(w[130:132], 3) },
		"overlap-security": func(w []byte) {
			binary.LittleEndian.PutUint16(w[120:122], 128)
			binary.LittleEndian.PutUint16(w[122:124], 2)
		},
	} {
		t.Run(name, func(t *testing.T) {
			bad := bytes.Clone(response)
			mutate(bad)
			_, err := smb2Walk(bad, 64)
			require.Error(t, err)
		})
	}
	request := append(smb2NegotiateReq(0x0311)[4:], make([]byte, 12)...)
	binary.LittleEndian.PutUint32(request[92:96], 104)
	binary.LittleEndian.PutUint16(request[96:98], 1)
	binary.LittleEndian.PutUint16(request[104:106], 1)
	binary.LittleEndian.PutUint16(request[106:108], 2)
	_, err = smb2Walk(request, 64)
	require.NoError(t, err)
	_, err = smb2Walk(request[:113], 64)
	require.Error(t, err, "a request's declared contexts must also be complete")
}

func TestSMB2CreateContextBoundsBeforeOpening(t *testing.T) {
	fid := bytes.Repeat([]byte{0x12}, 16)
	response := append(smb2CreateResp(1, 0x11, 1, fid)[4:], make([]byte, 32)...)
	binary.LittleEndian.PutUint32(response[144:148], 152)
	binary.LittleEndian.PutUint32(response[148:152], 32)
	binary.LittleEndian.PutUint16(response[156:158], 16)
	binary.LittleEndian.PutUint16(response[158:160], 4)
	binary.LittleEndian.PutUint16(response[162:164], 24)
	binary.LittleEndian.PutUint32(response[164:168], 8)
	copy(response[168:172], "MxAc")
	copy(response[176:184], "abcdefgh")
	commands, err := smb2Walk(response, 64)
	require.NoError(t, err)
	contexts := commands[0]["Body"].(map[string]any)["Create Contexts"].([]map[string]any)
	require.Equal(t, []byte("MxAc"), contexts[0]["Name"])
	require.Equal(t, []byte("abcdefgh"), contexts[0]["Data"])
	for name, mutate := range map[string]func([]byte){
		"truncated-contexts": func(w []byte) { binary.LittleEndian.PutUint32(w[148:152], 33) },
		"invalid-next":       func(w []byte) { binary.LittleEndian.PutUint32(w[152:156], 8) },
		"name-over-header":   func(w []byte) { binary.LittleEndian.PutUint16(w[156:158], 1) },
		"data-over-next":     func(w []byte) { binary.LittleEndian.PutUint32(w[164:168], 9) },
		"name-data-overlap":  func(w []byte) { binary.LittleEndian.PutUint16(w[162:164], 16) },
	} {
		t.Run(name, func(t *testing.T) {
			state := &binSMB2{client: 0}
			_, err := state.consumeFrom(0, smb2Create(1, 0x11, 1, "test"), 64)
			require.NoError(t, err)
			bad := bytes.Clone(response)
			mutate(bad)
			_, err = state.consumeFrom(1, smb2TCP(bad), 64)
			require.Error(t, err)
			require.Empty(t, state.files)
			require.Len(t, state.pending, 1)
		})
	}
}

func TestSMB2CloseResponseTimesAreNotFileID(t *testing.T) {
	fid := bytes.Repeat([]byte{0x12}, 16)
	for _, status := range []uint32{0, 0xc0000022} {
		t.Run(fmt.Sprintf("status-%x", status), func(t *testing.T) {
			state := &binSMB2{client: 0}
			_, err := state.consumeFrom(0, smb2Create(1, 0x11, 1, "one.txt"), 64)
			require.NoError(t, err)
			_, err = state.consumeFrom(1, smb2CreateResp(1, 0x11, 1, fid), 64)
			require.NoError(t, err)
			_, err = state.consumeFrom(0, smb2Close(2, 0x11, 1, fid), 64)
			require.NoError(t, err)
			wire := smb2CloseResp(2, 0x11, 1, nil)
			if status != 0 {
				wire = smb2ErrorResponse(6, 2, 0x11, 1, status, bytes.Repeat([]byte{0x12}, 96))
			}
			info, err := state.consumeFrom(1, wire, 64)
			require.NoError(t, err)
			require.NotContains(t, info, "FileId")
			require.Equal(t, hex.EncodeToString(fid), info["Request File ID"])
			if status == 0 {
				require.Empty(t, state.files)
				require.Equal(t, uint64(0x0102030405060708), info["Body"].(map[string]any)["Creation Time"])
			} else {
				require.Len(t, state.files, 1, "failed CLOSE must preserve the open handle")
			}
		})
	}
	state := &binSMB2{client: 0}
	info, err := state.consumeFrom(1, smb2CloseResp(99, 0x11, 1, nil), 64)
	require.NoError(t, err)
	require.Equal(t, true, info["Unmatched"])
	require.NotContains(t, info, "Request File ID")
	require.NotContains(t, info, "FileId")
	_, err = smb2Walk(smb2CloseResp(99, 0x11, 1, nil)[4:92], 64)
	require.Error(t, err, "the historical 24-byte request-shaped response is invalid")
}

func TestSMB2RelatedLiteralAndContextHaveSeparateSources(t *testing.T) {
	first := smb2Close(10, 0x1122334455667788, 0xaabbccdd, bytes.Repeat([]byte{1}, 16))[4:]
	binary.LittleEndian.PutUint32(first[20:24], uint32(len(first)))
	second := smb2Close(11, ^uint64(0), ^uint32(0), bytes.Repeat([]byte{0xff}, 16))[4:]
	binary.LittleEndian.PutUint32(second[16:20], smb2FlagRelated)
	binary.LittleEndian.PutUint32(second[20:24], uint32(len(second)))
	third := bytes.Clone(second)
	binary.LittleEndian.PutUint64(third[24:32], 12)
	binary.LittleEndian.PutUint32(third[20:24], 0)
	wire := smb2TCP(append(append(first, second...), third...))
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	r := s.Feed(0, time.Unix(1, 0), wire)
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	commands := r.Events[0].Session["Commands"].([]map[string]any)
	for index := 1; index < 3; index++ {
		c := commands[index]
		require.Equal(t, ^uint64(0), c["Session ID"])
		require.Equal(t, ^uint32(0), c["Tree ID"])
		require.Equal(t, uint64(0x1122334455667788), c["Effective Session ID"])
		require.Equal(t, uint32(0xaabbccdd), c["Effective Tree ID"])
		ranges := c["Wire Field Ranges"].(map[string]any)
		require.Equal(t, []any{uint64(4 + 88*index + 40), uint64(4 + 88*index + 48)}, ranges["Session ID"])
		require.NotContains(t, ranges, "Effective Session ID")
		require.NotContains(t, ranges, "Effective Tree ID")
	}
	encoded, err := json.Marshal(r.Events[0].Session)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"Session ID":18446744073709551615`)
	state := s.(*captureSession).f.smb2
	require.Len(t, state.pending, 3)
	for key := range state.pending {
		require.Equal(t, uint64(0x1122334455667788), key.sessionID)
		require.Equal(t, uint32(0xaabbccdd), key.treeID)
	}
	_, err = smb2Walk(third, 64)
	require.Error(t, err, "context cannot cross a Direct TCP PDU boundary")
}

func TestSMB2HeaderUnionAndOwnedLiteralRanges(t *testing.T) {
	request := smb2NegotiateReq(0x0202)
	binary.LittleEndian.PutUint16(request[12:14], 0x1234)
	binary.LittleEndian.PutUint16(request[14:16], 0xabcd)
	state := &binSMB2{client: 0}
	info, err := state.consumeFrom(0, request, 64)
	require.NoError(t, err)
	require.NotContains(t, info, "Status", "request union is not NTSTATUS")
	require.Equal(t, uint16(0x1234), info["Channel Sequence"])
	require.Equal(t, uint16(0xabcd), info["Reserved"])
	header := info["Header"].(map[string]any)
	require.NotContains(t, header, "Status")
	require.Equal(t, []any{uint64(12), uint64(14)}, header["Wire Field Ranges"].(map[string]any)["Channel Sequence"])
	signature := bytes.Clone(header["Signature"].([]byte))
	request[52] = 99
	require.Equal(t, signature, header["Signature"], "owned header bytes cannot borrow the input")

	response := smb2CloseResp(5, 1, 0, nil)
	binary.LittleEndian.PutUint32(response[20:24], smb2FlagResp|smb2FlagAsync)
	binary.LittleEndian.PutUint64(response[36:44], 0x1122334455667788)
	info, err = state.consumeFrom(1, response, 64)
	require.NoError(t, err)
	require.NotContains(t, info, "Channel Sequence")
	require.NotContains(t, info, "Process ID")
	require.NotContains(t, info, "Tree ID")
	require.Equal(t, uint64(0x1122334455667788), info["Async ID"])
	require.Equal(t, []any{uint64(36), uint64(44)}, info["Wire Field Ranges"].(map[string]any)["Async ID"])
}

func TestSMB2RelatedRequestIgnoresConcreteIDsWithoutChangingLiterals(t *testing.T) {
	first := smb2Read(1, 0x11, 1, bytes.Repeat([]byte{1}, 16), 1)[4:]
	second := smb2Read(2, 0x22, 2, bytes.Repeat([]byte{2}, 16), 1)[4:]
	binary.LittleEndian.PutUint32(first[20:24], uint32(len(first)))
	binary.LittleEndian.PutUint32(second[16:20], smb2FlagRelated)
	state := &binSMB2{client: 0}
	info, err := state.consumeFrom(0, smb2TCP(append(first, second...)), 64)
	require.NoError(t, err)
	c := info["Commands"].([]map[string]any)[1]
	require.Equal(t, uint64(0x22), c["Session ID"])
	require.Equal(t, uint32(2), c["Tree ID"])
	require.Equal(t, uint64(0x11), c["Effective Session ID"])
	require.Equal(t, uint32(1), c["Effective Tree ID"])
	require.Equal(t, hex.EncodeToString(bytes.Repeat([]byte{2}, 16)), c["FileId"])
	require.Equal(t, true, c["Related File ID Ignored"])
	require.NotContains(t, c, "Related File ID Placeholder", "ignored concrete IDs are not all-ones placeholders")
	badResponse, err := state.consumeFrom(1, smb2ReadResp(2, 0x22, 2, nil), 64)
	require.NoError(t, err)
	require.Equal(t, true, badResponse["Unmatched"])
	require.Len(t, state.pending, 2)
	matched, err := state.consumeFrom(1, smb2ReadResp(2, 0x11, 1, nil), 64)
	require.NoError(t, err)
	require.Equal(t, "matched", matched["Association Status"])
	require.Len(t, state.pending, 1)
}

func TestSMB2RelatedResponseCannotInventIDsOrInheritUnmatchedFile(t *testing.T) {
	fid := bytes.Repeat([]byte{1}, 16)
	first := smb2Create(1, 0x11, 1, "a.txt")[4:]
	for len(first)%8 != 0 {
		first = append(first, 0)
	}
	second := smb2Close(2, ^uint64(0), ^uint32(0), bytes.Repeat([]byte{0xff}, 16))[4:]
	binary.LittleEndian.PutUint32(first[20:24], uint32(len(first)))
	binary.LittleEndian.PutUint32(second[16:20], smb2FlagRelated)
	for _, unmatchedPredecessor := range []bool{false, true} {
		t.Run(fmt.Sprint(unmatchedPredecessor), func(t *testing.T) {
			state := &binSMB2{client: 0}
			_, err := state.consumeFrom(0, smb2TCP(append(bytes.Clone(first), second...)), 64)
			require.NoError(t, err)
			mid := uint64(1)
			if unmatchedPredecessor {
				mid = 99
			}
			a := smb2CreateResp(mid, 0x11, 1, fid)[4:]
			b := smb2CloseResp(2, 0x11, 1, nil)[4:]
			if !unmatchedPredecessor {
				binary.LittleEndian.PutUint32(b[36:40], ^uint32(0))
				binary.LittleEndian.PutUint64(b[40:48], ^uint64(0))
			}
			binary.LittleEndian.PutUint32(a[20:24], uint32(len(a)))
			binary.LittleEndian.PutUint32(b[16:20], smb2FlagResp|smb2FlagRelated)
			info, err := state.consumeFrom(1, smb2TCP(append(a, b...)), 64)
			require.NoError(t, err)
			c := info["Commands"].([]map[string]any)[1]
			require.NotContains(t, c, "FileId")
			if unmatchedPredecessor {
				require.NotContains(t, c, "Request File ID")
				require.NotContains(t, c, "Inherited File ID")
				require.Equal(t, "missing-request-file-context", c["File Context Status"])
				require.Empty(t, state.files)
			} else {
				require.Equal(t, ^uint64(0), c["Session ID"])
				require.NotContains(t, c, "Effective Session ID")
				require.Equal(t, true, c["Unmatched"])
				require.Len(t, state.files, 1, "invalid response IDs cannot close the correctly opened handle")
				require.Len(t, state.pending, 1)
			}
		})
	}
}

func TestSMB2ProbeRequiresFixedHeaderBeyondGenericWindow(t *testing.T) {
	wire := smb2NegotiateReq(0x0202)
	for _, cut := range []int{8, 9, 32, 63, 64, 67} {
		p := probeSMB2(wire[:cut], 64)
		require.Equal(t, ProbeNeedMore, p.Verdict, "cut %d", cut)
		require.Equal(t, 68-cut, p.NeedBytes)
	}
	require.Equal(t, ProbeAccept, probeSMB2(wire[:68], 64).Verdict)
	bad := bytes.Clone(wire)
	bad[8] = 63
	require.Equal(t, ProbeReject, probeSMB2(bad, 64).Verdict)

	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	require.Equal(t, ProbeAccept, s.Probe(wire).Verdict, "public Probe must inspect the complete fixed header before generic clipping")
	require.NotEqual(t, ProbeAccept, s.Probe(bad).Verdict)
	first := s.Feed(0, time.Unix(1, 0), wire[:64])
	require.Empty(t, first.Events)
	require.True(t, first.NeedMore)
	require.False(t, s.(*captureSession).f.directions[0].stopped)
	last := s.Feed(0, time.Unix(1, 0), wire[64:])
	require.Nil(t, last.Err)
	require.Len(t, last.Events, 1)
	require.Equal(t, "NEGOTIATE", last.Events[0].Session["Packet Name"])
}
