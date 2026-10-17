package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// Fixed abstract grammar/source literals, not a purported successful peer or
// multiresult raw aggregate. This baseline requires only existing public APIs.
func TestDLMSWrapperSNMultiResultExistingAPI(t *testing.T) {
	raw := finsReadBatch(t, "dlms-sn-multiresult-source", "DLMS_SN_MULTIRESULT_ROOT", "literal-candidates.json")
	require.Equal(t, "b954edb8d07c1adda82cbab222e30368cdd530c4bcc7bd0b39e553bb0c1334ff", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var doc struct {
		Vectors []struct {
			Name   string                 `json:"name"`
			Wire   string                 `json:"wrapper_wire_hex"`
			Fields map[string]any         `json:"literal_fields"`
			Err    *struct{ Kind string } `json:"literal_error"`
		} `json:"vectors"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Vectors, 6)
	for i, v := range doc.Vectors {
		for _, tr := range []string{"tcp", "udp"} {
			for _, deferred := range []bool{false, true} {
				chunks := []int{0, 1, 7}
				if tr == "udp" {
					chunks = []int{0}
				}
				for _, chunk := range chunks {
					t.Run(fmt.Sprintf("%s/%s/d%t/chunk%d", v.Name, tr, deferred, chunk), func(t *testing.T) {
						session, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(40000, 4059))
						require.NoError(t, err)
						defer session.Close("cleanup")
						session.(*captureSession).f.a.config.Deferred = deferred
						wire := wrapperWire(t, v.Wire)
						size := chunk
						if size == 0 {
							size = len(wire)
						}
						var events []*ProtocolEvent
						for at := 0; at < len(wire); at += size {
							part := append([]byte(nil), wire[at:min(at+size, len(wire))]...)
							result := session.Feed(1, time.Unix(100, 0), part)
							clear(part)
							require.Equal(t, min(size, len(wire)-at), result.Consumed)
							if len(result.Events) == 0 {
								require.Equal(t, "tcp", tr)
								rocTypedError(t, "NeedMore", result.Err)
								require.True(t, result.NeedMore)
							} else if v.Err == nil {
								require.Nil(t, result.Err)
								require.False(t, result.NeedMore)
							}
							events = append(events, result.Events...)
						}
						require.Len(t, events, 1)
						e := events[0]
						require.NotZero(t, e.ID)
						if tr == "udp" {
							require.Zero(t, e.FlowID)
						} else {
							require.NotZero(t, e.FlowID)
						}
						require.Equal(t, "dlms-wrapper", e.Protocol)
						require.Zero(t, e.TransactionID)
						require.Zero(t, e.ResponseTo)
						require.Equal(t, time.Unix(100, 0), e.Timestamp)
						require.Zero(t, e.Offset)
						require.Equal(t, len(wire), e.Length)
						if v.Err != nil {
							require.Nil(t, e.Fields)
							require.Nil(t, e.Session)
							snReadQueryError(t, e, v.Err.Kind)
						} else {
							require.Equal(t, wire, e.Raw)
							status := "decoded"
							if deferred {
								status = "deferred"
							}
							require.Equal(t, status, e.Status)
							require.Equal(t, "message", e.Completeness)
							fields, err := e.GetFields()
							require.NoError(t, err)
							expected := cloneSession(v.Fields)
							expected["wrapper_version"] = uint16(1)
							expected["source_wport"] = uint16(1)
							expected["destination_wport"] = uint16(16)
							expected["wrapper_header_hex"] = v.Wire[:16]
							rocEqualFields(t, expected, fields)
							snReadTypes(t, fields)
							assoc := "unverified-sn-multiresult-observation"
							profile := "dlms-wrapper-v1-sn-multiresult"
							if i == 0 {
								assoc = "unverified-sn-read-observation"
								profile = "dlms-wrapper-v1-sn-read"
							}
							require.Equal(t, profile, e.Profile)
							expected["Association"], expected["Outstanding"] = assoc, 0
							rocEqualFields(t, expected, e.Session)
							snReadTypes(t, e.Session)
							decoded, err := e.Decode()
							require.NoError(t, err)
							rocEqualFields(t, expected, decoded["session"].(map[string]any))
							poisonWrapperStructured(fields)
							poisonWrapperStructured(e.Session)
							again, err := e.GetFields()
							require.NoError(t, err)
							delete(expected, "Association")
							delete(expected, "Outstanding")
							rocEqualFields(t, expected, again)
						}
						require.Empty(t, session.Close("baseline"))
						require.Empty(t, session.Close("again"))
						require.Zero(t, session.Stats().BufferedBytes)
					})
				}
			}
		}
	}
}
