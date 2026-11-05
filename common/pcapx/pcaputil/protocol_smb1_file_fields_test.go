package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSMB1FileCommandsAliasesRetainLiteralAndContextSources(t *testing.T) {
	for _, command := range []byte{0x75, 0x06} {
		for _, response := range []bool{false, true} {
			t.Run(fmt.Sprintf("command%02x/response%v", command, response), func(t *testing.T) {
				s := smb1SetupSession(t)
				e := smb1FeedComplete(t, s, 0, smb1FileWire(command, response, 4, 2048, 2048))
				fields, err := e.GetFields()
				require.NoError(t, err)
				body := fields["Body"].(map[string]any)
				aliases := map[string]string{}
				if command == 0x75 {
					aliases["smb.andxoffset"] = "AndX Offset"
					aliases["smb.andx.offset"] = "AndX Offset"
					aliases["smb.service"] = "Service"
					if response {
						aliases["smb.connect.support"] = "Optional Support"
						aliases["smb.native_fs"] = "Native File System"
					} else {
						aliases["smb.pwlen"] = "Password Length"
						aliases["smb.connect.flags"] = "Flags"
						aliases["smb.path"] = "Path"
					}
				} else if !response {
					aliases["smb.search.attribute"] = "Search Attributes"
					aliases["smb.buffer_format"] = "Buffer Format"
					aliases["smb.file"] = "File Name"
				}
				for alias, key := range aliases {
					v := smb1AliasContractValue(t, e, alias)
					require.Equal(t, "pdu", v.Origin)
					span := body["Wire Field Ranges"].(map[string]any)[key].([]any)
					start, end := span[0].(uint64), span[1].(uint64)
					require.Equal(t, &DisplayFieldRange{Offset: int(start), Length: int(end - start)}, v.Range)
					switch value := body[key].(type) {
					case string:
						require.Equal(t, value, v.Value)
					case byte:
						require.Equal(t, uint64(value), v.Value)
					case uint16:
						require.Equal(t, uint64(value), v.Value)
					default:
						t.Fatalf("unexpected literal type %T", value)
					}
				}
				pid := smb1AliasContractValue(t, e, "smb.pid")
				require.Equal(t, uint64(0x12345678), pid.Value)
				require.Nil(t, pid.Range)
				require.Equal(t, "derived", pid.Origin)
				if response && command == 0x06 {
					require.NotContains(t, e.DisplayFieldValues().FieldMap(), "smb.file", "WC0 response has no filename bytes")
				}
			})
		}
	}
	for _, command := range []byte{0x75, 0x06} {
		s := smb1SetupSession(t)
		r := smb1FeedComplete(t, s, 0, smb1FileWire(command, false, 4, 2048, 2048))
		original, err := r.GetFields()
		require.NoError(t, err)
		before, err := json.Marshal(original)
		require.NoError(t, err)
		r.Session["Header"].(map[string]any)["User ID"] = uint16(999)
		r.Session["Body"].(map[string]any)["Path"] = "edited"
		r.Fields = map[string]any{"fake": true}
		r.Structured = map[string]any{"fields": r.Fields}
		r.Protocol, r.Profile = "edited", "edited"
		e := smb1FeedComplete(t, s, 1, smb1FileWire(command, true, 4, 2048, 2048))
		require.Equal(t, r.ID, e.ResponseTo)
		alias, key := "smb.request.file", "Request File Name"
		if command == 0x75 {
			alias, key = "smb.request.path", "Request Path"
		}
		v := smb1AliasContractValue(t, e, alias)
		require.Equal(t, "context", v.Origin)
		require.Nil(t, v.Range)
		require.Equal(t, e.Session[key], v.Value)
		fresh, err := r.GetFields()
		require.NoError(t, err)
		after, err := json.Marshal(fresh)
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
}

func TestSMB1FileCommandsResourceRefusalAndPrivateRollback(t *testing.T) {
	w := smb1FileWire(0x75, false, 4, 2048, 0)
	for _, budget := range []ParserBudget{{MaxFrameBytes: len(w) - 1}, {MaxMessageBytes: len(w) - 1}, {MaxBufferedBytes: len(w) - 1}, {MaxRecursionDepth: 2}, {MaxCollectionElements: -1}} {
		_, err := decodeSMB1FileFields(w, budget)
		var typed *ProtocolError
		require.ErrorAs(t, err, &typed)
		require.Equal(t, ErrResourceExceeded, typed.Kind)
	}
	a := &binParser{budget: DefaultParserBudget(), config: BinParserConfig{MaxBufferedBytes: 1 << 20}}
	f := &binFlow{a: a}
	f.smb2 = &binSMB2{flow: f}
	request := &ProtocolEvent{Raw: bytes.Clone(w), Entry: "CIFSDirectTCPTreeConnectAndXRequest"}
	require.NoError(t, f.consumeSMB1FileCommand(0, request))
	carrier := f.smb2
	previous := f.smb2.smb1Legacy
	response := smb1FileWire(0x75, true, 4, 2048, 2048)
	fields, err := decodeSMB1FileFields(response, DefaultParserBudget())
	require.NoError(t, err)
	e := &ProtocolEvent{ID: a.ids.Add(1), Raw: response, nativeSessionMessage: true}
	tx, err := f.beginSMB1Legacy(1, e, fields)
	require.NoError(t, err)
	e.smb1FileBodyRefusal = true
	e.Session = map[string]any{"Error Scope": "message"}
	e.semanticFields = fields
	e.semanticSession = fields
	tx.refused = true
	cause := protocolError(ErrResourceExceeded, "injected current resource refusal")
	var result error = cause
	tx.finish(&result)
	require.Same(t, carrier, f.smb2)
	require.Same(t, previous, f.smb2.smb1Legacy)
	require.ErrorIs(t, result, cause)
	require.False(t, e.smb1FileBodyRefusal)
	require.False(t, e.smb1SetupBodyRefusal)
	require.Nil(t, e.Session)
	require.Nil(t, e.semanticFields)
	require.Zero(t, e.ResponseTo)
	f.closeSession()
	require.Zero(t, a.buffered.Load())
}

func TestSMB1FileCommandsRefusedOwnerAndModernCarrierBoundaries(t *testing.T) {
	for _, wrong := range []string{"none", "UID", "TID"} {
		t.Run("delete-refusal-"+wrong, func(t *testing.T) {
			s := smb1SetupSession(t)
			req := smb1FeedComplete(t, s, 0, smb1FileWire(0x06, false, 4, 2048, 2048))
			bad := smb1FileWire(0x06, true, 4, 2048, 2048)
			binary.LittleEndian.PutUint32(bad[9:13], 0xc0000022)
			if wrong == "UID" {
				bad[32] ^= 1
			}
			if wrong == "TID" {
				bad[28] ^= 1
			}
			r := s.Feed(1, time.Unix(101, 0), bad)
			require.NotNil(t, r.Err)
			require.Equal(t, ErrUnsupportedFeature, r.Err.Kind)
			require.Len(t, r.Events, 1)
			require.Equal(t, "headers", r.Events[0].Completeness)
			require.NotContains(t, r.Events[0].Session, "Body")
			require.Zero(t, r.Events[0].ResponseTo)
			good := smb1FeedComplete(t, s, 1, smb1FileWire(0x06, true, 4, 2048, 2048))
			if wrong == "none" {
				require.Zero(t, good.ResponseTo)
			} else {
				require.Equal(t, req.ID, good.ResponseTo)
				require.NotContains(t, r.Events[0].Session["Context Eligibility"], "retired")
			}
		})
	}
	for _, diagnostic := range []bool{false, true} {
		t.Run(fmt.Sprintf("modern-diagnostic%v", diagnostic), func(t *testing.T) {
			s := smb1SetupSession(t)
			smb1FeedComplete(t, s, 0, smb1FileWire(0x75, false, 4, 2048, 0))
			state := s.(*captureSession).f.smb2
			require.NotEmpty(t, state.smb1Legacy.pending)
			modern := smb2NegotiateReq(0x0311)
			if diagnostic {
				modern = smb2UnsupportedSetup()
			}
			r := s.Feed(0, time.Unix(101, 0), modern)
			require.Len(t, r.Events, 1)
			if diagnostic {
				require.NotNil(t, r.Err)
				require.Equal(t, "smb2-record-body-diagnostic", r.Events[0].Profile)
			} else {
				require.Nil(t, r.Err)
			}
			require.Same(t, state, s.(*captureSession).f.smb2)
			require.True(t, state.smb1Modern)
			require.Nil(t, state.smb1Legacy)
			late := smb1FeedComplete(t, s, 1, smb1FileWire(0x75, true, 4, 2048, 2048))
			require.Zero(t, late.ResponseTo)
			require.NotContains(t, late.Session, "Request Path")
		})
	}
}
