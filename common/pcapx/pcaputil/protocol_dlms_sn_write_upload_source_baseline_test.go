package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// This source-field capability witness is deliberately narrower than a final
// group/host oracle. Association/Profile/Flow/Close policies remain pending.
// All session/event methods exist in the fixed parent; no new decoder API is
// referenced, so a missing internal symbol is never counted as an old failure.
func TestDLMSSNWriteUploadSourceExistingAPI(t *testing.T) {
	var raw []byte
	if root := os.Getenv("DLMS_SN_WRITE_UPLOAD_SOURCE_ROOT"); root != "" {
		var err error
		raw, err = os.ReadFile(filepath.Join(root, "BASELINE-CANDIDATES.json"))
		require.NoError(t, err)
	} else {
		raw = finsReadBatch(t, "dlms-sn-write-upload-source", "DLMS_SN_WRITE_UPLOAD_SOURCE_ROOT", "BASELINE-CANDIDATES.json")
	}
	require.Equal(t, "0be56458f16964ebf0c5158387dc2e5a0ececbeafa3ab3c68d894971d53a55ef", fmt.Sprintf("%x", sha256.Sum256(raw)))
	type vector struct {
		Wire      string         `json:"input_hex"`
		SHA       string         `json:"input_sha256"`
		Direction int            `json:"caller_direction"`
		Fields    map[string]any `json:"literal_fields_proposal"`
	}
	var gold struct {
		Vectors []vector `json:"vectors"`
		Body    struct {
			Fields map[string]any `json:"assembled_request_body_proposal"`
			Hex    string         `json:"assembled_request_body_hex_proposal"`
			Length int            `json:"assembled_body_length_proposal"`
		} `json:"joined_body_answer"`
	}
	require.NoError(t, json.Unmarshal(raw, &gold))
	require.Len(t, gold.Vectors, 4)
	for _, tr := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/d%v", tr, deferred), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = deferred
				offsets := [2]uint64{}
				saved := []*ProtocolEvent{}
				for i, v := range gold.Vectors {
					wire := wrapperWire(t, v.Wire)
					require.Equal(t, v.SHA, fmt.Sprintf("%x", sha256.Sum256(wire)))
					in := bytes.Clone(wire)
					r := s.Feed(v.Direction, time.Unix(100+int64(i), 0), in)
					clear(in)
					require.Equal(t, len(wire), r.Consumed)
					require.False(t, r.NeedMore)
					require.Nil(t, r.Err)
					require.Len(t, r.Events, 1)
					e := r.Events[0]
					saved = append(saved, e)
					require.NotZero(t, e.ID)
					for _, old := range saved[:len(saved)-1] {
						require.NotEqual(t, old.ID, e.ID)
					}
					require.Zero(t, e.TransactionID)
					require.Zero(t, e.ResponseTo)
					require.Equal(t, v.Direction, e.Direction)
					require.Equal(t, "dlms-wrapper", e.Protocol)
					require.Equal(t, time.Unix(100+int64(i), 0), e.Timestamp)
					require.Equal(t, wire, e.Raw)
					require.Equal(t, len(wire), e.Length)
					if tr == "tcp" {
						require.Equal(t, offsets[v.Direction], e.Offset)
					} else {
						require.Zero(t, e.Offset)
					}
					offsets[v.Direction] += uint64(len(wire))
					f, err := e.GetFields()
					require.NoError(t, err)
					snWriteTypes(t, f)
					// Assert every independently known outer field. Additional adopted state
					// metadata is not guessed here and must be frozen by the later full gold.
					for k, want := range v.Fields {
						got, ok := f[k]
						require.True(t, ok, k)
						rocEqualFields(t, map[string]any{k: want}, map[string]any{k: got})
					}
					if i == 2 {
						require.IsType(t, map[string]any{}, f["assembled_request_body"])
						rocEqualFields(t, gold.Body.Fields, f["assembled_request_body"].(map[string]any))
						require.Equal(t, gold.Body.Hex, f["assembled_request_body_hex"])
						require.Equal(t, gold.Body.Length, f["assembled_body_length"])
					}
					f["command"] = "poison"
					again, err := e.GetFields()
					require.NoError(t, err)
					require.IsType(t, byte(0), again["command"])
					d, err := e.Decode()
					require.NoError(t, err)
					require.IsType(t, map[string]any{}, d["fields"])
					require.IsType(t, map[string]any{}, d["session"])
				}
				// Completed raw upload observation ends before the independent ordinary
				// response; it is never promoted into a verified SN business owner.
				require.Empty(t, s.Close("source baseline complete"))
				require.Empty(t, s.Close("repeat"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
