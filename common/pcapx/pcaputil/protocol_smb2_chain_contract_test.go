package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func smb2AsyncWire(wire []byte, id uint64) []byte {
	wire = bytes.Clone(wire)
	binary.LittleEndian.PutUint32(wire[20:24], binary.LittleEndian.Uint32(wire[20:24])|smb2FlagAsync)
	binary.LittleEndian.PutUint64(wire[36:44], id)
	return wire
}

func smb2CompoundWire(wires ...[]byte) []byte {
	var payload []byte
	for index, wire := range wires {
		command := bytes.Clone(wire[4:])
		if index != 0 {
			binary.LittleEndian.PutUint32(command[16:20], binary.LittleEndian.Uint32(command[16:20])|smb2FlagRelated)
		}
		if index < len(wires)-1 {
			for len(command)%8 != 0 {
				command = append(command, 0)
			}
			binary.LittleEndian.PutUint32(command[20:24], uint32(len(command)))
		}
		payload = append(payload, command...)
	}
	return smb2TCP(payload)
}

func smb2ChainFeed(t *testing.T, session ProtocolSession, direction int, wire []byte) map[string]any {
	t.Helper()
	r := session.Feed(direction, time.Unix(1, 0), wire)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, "decoded", r.Events[0].Status)
	return r.Events[0].Session
}

func TestSMB2AsyncCreateUsesMatchedRequestTreeWithoutLiteralFabrication(t *testing.T) {
	fid := bytes.Repeat([]byte{0x12}, 16)
	encoded := hex.EncodeToString(fid)
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { session.Close("FIN"); require.Zero(t, session.Stats().BufferedBytes) })
	smb2ChainFeed(t, session, 0, smb2Create(10, 17, 7, "async.txt"))
	interim := smb2AsyncWire(smb2ErrorResponse(5, 10, 17, 0, 0x103, nil), 0xabc)
	info := smb2ChainFeed(t, session, 1, interim)
	require.Equal(t, true, info["Operation Pending"])
	info = smb2ChainFeed(t, session, 1, smb2AsyncWire(smb2CreateResp(10, 17, 0, fid), 0xabc))
	require.Equal(t, "matched", info["Association Status"])
	require.Equal(t, uint32(7), info["Request Tree ID"])
	require.NotContains(t, info, "Tree ID")
	require.NotContains(t, info["Header"], "Tree ID")
	require.NotContains(t, info["Wire Field Ranges"], "Tree ID")
	state := session.(*captureSession).f.smb2
	require.Equal(t, "async.txt", state.files[smb2FileKey{sessionID: 17, treeID: 7, fileID: encoded}])
	require.NotContains(t, state.files, smb2FileKey{sessionID: 17, treeID: 0, fileID: encoded})
	require.Empty(t, state.async)
	require.Empty(t, state.pending)
	// The same opaque FileId in another tree cannot lend or lose its path.
	smb2ChainFeed(t, session, 0, smb2Create(20, 17, 8, "other.txt"))
	smb2ChainFeed(t, session, 1, smb2CreateResp(20, 17, 8, fid))
	read := smb2ChainFeed(t, session, 0, smb2Read(11, 17, 7, fid, 1))
	require.Equal(t, "async.txt", read["File Context"])
	smb2ChainFeed(t, session, 1, smb2ReadResp(11, 17, 7, []byte("x")))
	smb2ChainFeed(t, session, 0, smb2Close(12, 17, 7, fid))
	smb2ChainFeed(t, session, 1, smb2CloseResp(12, 17, 7, nil))
	require.Len(t, state.files, 1)
	require.Equal(t, "other.txt", state.files[smb2FileKey{sessionID: 17, treeID: 8, fileID: encoded}])
}

func TestSMB2AsyncCreateNeedsExactAssociationAndSuccess(t *testing.T) {
	for _, mode := range []string{"error", "unknown-id", "wrong-direction", "other-session"} {
		t.Run(mode, func(t *testing.T) {
			state := &binSMB2{client: 0}
			_, err := state.consumeFrom(0, smb2Create(10, 17, 7, "a.txt"), 64)
			require.NoError(t, err)
			_, err = state.consumeFrom(1, smb2AsyncWire(smb2ErrorResponse(5, 10, 17, 0, 0x103, nil), 0xabc), 64)
			require.NoError(t, err)
			response := smb2AsyncWire(smb2CreateResp(10, 17, 0, bytes.Repeat([]byte{1}, 16)), 0xabc)
			dir := 1
			switch mode {
			case "error":
				response = smb2AsyncWire(smb2ErrorResponse(5, 10, 17, 0, 0xc0000022, bytes.Repeat([]byte{1}, 96)), 0xabc)
			case "unknown-id":
				response = smb2AsyncWire(response, 0xdef)
			case "wrong-direction":
				dir = 0
			case "other-session":
				binary.LittleEndian.PutUint64(response[44:52], 18)
			}
			info, err := state.consumeFrom(dir, response, 64)
			require.NoError(t, err)
			require.Empty(t, state.files)
			if mode == "error" {
				require.Equal(t, "matched", info["Association Status"])
				require.Empty(t, state.pending)
				require.Empty(t, state.async)
			} else {
				require.Equal(t, true, info["Unmatched"])
				require.Len(t, state.pending, 1)
				require.Len(t, state.async, 1)
			}
		})
	}
}

func TestSMB2RelatedFileContextSurvivesNoFileIDResponses(t *testing.T) {
	for _, separate := range []bool{false, true} {
		for _, createFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("separate=%t/create=%t", separate, createFirst), func(t *testing.T) {
				session, err := NewProtocolSession(DefaultParserBudget())
				require.NoError(t, err)
				t.Cleanup(func() { session.Close("FIN"); require.Zero(t, session.Stats().BufferedBytes) })
				fid, placeholder := bytes.Repeat([]byte{0x12}, 16), bytes.Repeat([]byte{0xff}, 16)
				var requests, replies [][]byte
				if createFirst {
					requests = [][]byte{smb2Create(10, 17, 7, "chain.txt"), smb2Read(11, ^uint64(0), ^uint32(0), placeholder, 1), smb2Close(12, ^uint64(0), ^uint32(0), placeholder)}
					replies = [][]byte{smb2CreateResp(10, 17, 7, fid), smb2ReadResp(11, 17, 7, []byte("x")), smb2CloseResp(12, 17, 7, nil)}
				} else {
					smb2ChainFeed(t, session, 0, smb2Create(1, 17, 7, "chain.txt"))
					smb2ChainFeed(t, session, 1, smb2CreateResp(1, 17, 7, fid))
					requests = [][]byte{smb2Write(10, 17, 7, fid, []byte("x")), smb2Close(11, ^uint64(0), ^uint32(0), placeholder)}
					replies = [][]byte{smb2WriteResp(10, 17, 7, 1), smb2CloseResp(11, 17, 7, nil)}
				}
				request := smb2ChainFeed(t, session, 0, smb2CompoundWire(requests...))
				commands := request["Commands"].([]map[string]any)
				require.Equal(t, hex.EncodeToString(placeholder), commands[len(commands)-1]["FileId"])
				var responses []map[string]any
				if separate {
					for _, reply := range replies {
						responses = append(responses, smb2ChainFeed(t, session, 1, reply))
					}
				} else {
					responses = smb2ChainFeed(t, session, 1, smb2CompoundWire(replies...))["Commands"].([]map[string]any)
				}
				for i, response := range responses {
					require.Equal(t, "matched", response["Association Status"])
					if i > 0 || !createFirst {
						require.NotContains(t, response, "FileId")
						require.NotContains(t, response["Wire Field Ranges"], "Request File ID")
						require.Equal(t, hex.EncodeToString(fid), response["Request File ID"])
					}
				}
				last := responses[len(responses)-1]
				require.Equal(t, uint64(10), last["File Context Source Message ID"])
				require.Equal(t, uint64(10+len(replies)-2), last["Related Predecessor Message ID"])
				require.Equal(t, "closed", last["File Context Status"])
				state := session.(*captureSession).f.smb2
				require.Empty(t, state.files)
				require.Empty(t, state.pending)
				require.Empty(t, state.async)
			})
		}
	}
}

func TestSMB2FailedCreateAndOtherChainCannotSupplyFileContext(t *testing.T) {
	fid, placeholder := bytes.Repeat([]byte{0x12}, 16), bytes.Repeat([]byte{0xff}, 16)
	for _, mode := range []string{"failed-create", "unmatched-create", "foreign-chain"} {
		t.Run(mode, func(t *testing.T) {
			state := &binSMB2{client: 0}
			_, err := state.consumeFrom(0, smb2Create(1, 17, 7, "retained.txt"), 64)
			require.NoError(t, err)
			_, err = state.consumeFrom(1, smb2CreateResp(1, 17, 7, fid), 64)
			require.NoError(t, err)
			_, err = state.consumeFrom(0, smb2CompoundWire(smb2Create(10, 17, 7, "chain.txt"), smb2Close(11, ^uint64(0), ^uint32(0), placeholder)), 64)
			require.NoError(t, err)
			var preceding []byte
			switch mode {
			case "failed-create":
				preceding = smb2ErrorResponse(5, 10, 17, 7, 0xc0000022, bytes.Repeat([]byte{0x12}, 96))
			case "unmatched-create":
				preceding = smb2CreateResp(99, 17, 7, fid)
			case "foreign-chain":
				_, err = state.consumeFrom(0, smb2Create(20, 17, 8, "foreign.txt"), 64)
				require.NoError(t, err)
				preceding = smb2CreateResp(20, 17, 8, fid)
			}
			_, err = state.consumeFrom(1, preceding, 64)
			require.NoError(t, err)
			response, err := state.consumeFrom(1, smb2CloseResp(11, 17, 7, nil), 64)
			require.NoError(t, err)
			require.Equal(t, "matched", response["Association Status"])
			require.NotContains(t, response, "Request File ID")
			require.Equal(t, "missing-request-file-context", response["File Context Status"])
			require.Equal(t, "retained.txt", state.files[smb2FileKey{sessionID: 17, treeID: 7, fileID: hex.EncodeToString(fid)}])
		})
	}
}

func TestSMB2FailedReadRetainsKnownUsedFileIdentity(t *testing.T) {
	fid := bytes.Repeat([]byte{1}, 16)
	state := &binSMB2{client: 0}
	_, err := state.consumeFrom(0, smb2Create(1, 17, 7, "known.txt"), 64)
	require.NoError(t, err)
	_, err = state.consumeFrom(1, smb2CreateResp(1, 17, 7, fid), 64)
	require.NoError(t, err)
	_, err = state.consumeFrom(0, smb2CompoundWire(smb2Read(10, 17, 7, fid, 1), smb2Close(11, ^uint64(0), ^uint32(0), bytes.Repeat([]byte{0xff}, 16))), 64)
	require.NoError(t, err)
	_, err = state.consumeFrom(1, smb2ErrorResponse(8, 10, 17, 7, 0xc0000011, nil), 64)
	require.NoError(t, err)
	response, err := state.consumeFrom(1, smb2CloseResp(11, 17, 7, nil), 64)
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(fid), response["Request File ID"])
	require.Equal(t, uint32(0xc0000011), response["Related Predecessor Status"])
	require.Empty(t, state.files, "an error did not erase the proven request's used FileId")
}

func TestSMB2CompoundStateIsReservedBeforeRetention(t *testing.T) {
	parser := &binParser{config: BinParserConfig{MaxBufferedBytes: 700}}
	flow := &binFlow{a: parser}
	state := &binSMB2{flow: flow, client: 0}
	_, err := state.consumeFrom(0, smb2CompoundWire(smb2Create(10, 17, 7, "a.txt"), smb2Close(11, ^uint64(0), ^uint32(0), bytes.Repeat([]byte{0xff}, 16))), 64)
	require.Error(t, err)
	require.Empty(t, state.pending)
	require.Empty(t, state.files)
	require.Zero(t, flow.sessionBytes)
	require.Zero(t, parser.buffered.Load())
}

func TestSMB2CompoundDoesNotMixRequestStylesOrRoles(t *testing.T) {
	fid := bytes.Repeat([]byte{1}, 16)
	wire := smb2CompoundWire(smb2Close(1, 17, 7, fid), smb2Close(2, 17, 7, fid), smb2Close(3, 17, 7, fid))
	// Three 88-byte commands; the third cannot start a second style in one PDU.
	binary.LittleEndian.PutUint32(wire[4+176+16:4+176+20], 0)
	_, err := (&binSMB2{}).consumeFrom(0, wire, 64)
	require.Error(t, err)
	_, err = (&binSMB2{}).consumeFrom(0, smb2CompoundWire(smb2Close(1, 17, 7, fid), smb2CloseResp(2, 17, 7, nil)), 64)
	require.Error(t, err)
}
