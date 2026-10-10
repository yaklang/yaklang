package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func TestDLMSHDLCNormalDataExistingAPI(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 4059))
		require.NoError(t, err)
		q := wrapperWire(t, "7ea0190321107fdae6e600c001c1000f0000280000ff020091537e")
		r := wrapperWire(t, "7ea0192103307d5de6e700c401c10002021201000c02cebbfea57e")
		request := s.Feed(0, time.Unix(1, 0), q)
		require.Len(t, request.Events, 1)
		_, err = request.Events[0].GetFields()
		require.NoError(t, err)
		response := s.Feed(1, time.Unix(2, 0), r)
		require.Len(t, response.Events, 1)
		f, err := response.Events[0].GetFields()
		require.NoError(t, err, "existing API must observe selected complete normal structured Data")
		var expected map[string]any
		require.NoError(t, json.Unmarshal([]byte(`{"value":{"type":2,"length":2,"length_encoding_hex":"02","raw_hex":"02021201000c02cebb","elements":[{"type":18,"value":256,"raw_hex":"120100"},{"type":12,"length":2,"length_encoding_hex":"02","value_hex":"cebb","raw_hex":"0c02cebb","text_encoding":"utf-8","value":"λ","code_points":1}]}}`), &expected))
		rocEqualFields(t, expected, map[string]any{"value": f["Data Value"]})
		require.Equal(t, request.Events[0].ID, response.Events[0].ResponseTo)
		require.Zero(t, response.Events[0].TransactionID)
		require.Empty(t, s.Close("normal-data"))
		require.Empty(t, s.Close("idempotent"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
}

func dlmsNormalDataControls(t *testing.T) []dlmsListControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("dlms-hdlc-normal-data/controls.json")
	require.NoError(t, err)
	var m struct {
		Schema string
		Cases  []dlmsListControl
	}
	require.NoError(t, json.Unmarshal(b, &m))
	require.Equal(t, "owned-dlms-hdlc-normal-data/v1", m.Schema)
	require.Len(t, m.Cases, 31)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for _, c := range m.Cases {
		answer, err := trafficfixture.ReadFile("dlms-hdlc-normal-data/" + c.Answer)
		require.NoError(t, err)
		require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
		var a dlmsListControl
		require.NoError(t, json.Unmarshal(answer, &a))
		require.Equal(t, c.Events, a.Events)
		bound := 0
		for _, batch := range all {
			for _, input := range batch.Cases {
				if input.ID != "dlms-hdlc-normal-data/"+c.ID {
					continue
				}
				require.Equal(t, c.SHA256, input.Input.SHA256)
				require.Equal(t, c.InputAlias, input.Input.OriginalPath)
				require.Equal(t, c.Packets, input.Facts.PacketCount)
				for _, expectation := range input.Expectations {
					var bind struct {
						File string `json:"answer_file"`
						SHA  string `json:"answer_sha256"`
					}
					require.NoError(t, json.Unmarshal(expectation.PayloadConstraints, &bind))
					if bind.File == c.Answer {
						require.Equal(t, c.AnswerSHA, bind.SHA)
						bound++
					}
				}
			}
		}
		require.Equal(t, 1, bound, c.ID)
	}
	return m.Cases
}

func TestDLMSHDLCNormalDataSealedMatrix(t *testing.T) { dlmsSealedMatrix(t, dlmsNormalDataControls(t)) }
func TestDLMSHDLCNormalDataOwnershipAndChunks(t *testing.T) {
	dlmsOwnershipAndChunks(t, dlmsNormalDataControls(t))
}
func TestDLMSHDLCNormalDataProjectionLimits(t *testing.T) {
	for _, c := range dlmsNormalDataControls(t) {
		if c.ID != "structure-udp" && c.ID != "nested-compact-udp" {
			continue
		}
		q, r := wrapperWire(t, c.Events[0].Raw), wrapperWire(t, c.Events[1].Raw)
		require.Equal(t, ProbeAccept, probeDLMS(q, 2049).Verdict)
		require.Equal(t, ProbeReject, probeDLMS(r, 2049).Verdict)
		_, decodeErr := decodeDLMSBudget(r, 0, 64)
		rocTypedError(t, "ResourceExceeded", decodeErr)
		need := int(512 + 2*int64(len(q)) + dlmsProjection(r))
		for _, delta := range []int{-1, 0} {
			b := DefaultParserBudget()
			b.MaxFrameBytes, b.MaxMessageBytes, b.MaxBufferedBytes = 2049, 2049, need+delta
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			request := s.Feed(0, time.Unix(1, 0), q)
			require.Len(t, request.Events, 1)
			out := s.Feed(1, time.Unix(2, 0), r)
			require.Len(t, out.Events, 1)
			if delta == 0 {
				dlmsListAssert(t, c, append(request.Events, out.Events...))
			} else {
				f, err := out.Events[0].GetFields()
				rocTypedError(t, "ResourceExceeded", err)
				require.Nil(t, f)
				require.Nil(t, out.Events[0].Session)
				require.Zero(t, out.Events[0].ResponseTo)
				require.Zero(t, out.Events[0].TransactionID)
				late := s.Feed(1, time.Unix(3, 0), r)
				require.Len(t, late.Events, 1)
				_, err = late.Events[0].GetFields()
				rocTypedError(t, "ContextRequired", err)
				require.Zero(t, late.Events[0].ResponseTo)
			}
			require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
			require.Empty(t, s.Close("projection"))
			require.Zero(t, s.Stats().BufferedBytes)
		}
	}
	for _, c := range dlmsNormalDataControls(t) {
		if c.ID != "structure-udp" {
			continue
		}
		for _, nodes := range []int{2, 3} {
			b := DefaultParserBudget()
			b.MaxCollectionElements = nodes
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"))
			require.NoError(t, err)
			q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, c.Events[0].Raw))
			r := s.Feed(1, time.Unix(2, 0), wrapperWire(t, c.Events[1].Raw))
			require.Len(t, r.Events, 1)
			if nodes == 3 {
				dlmsListAssert(t, c, append(q.Events, r.Events...))
			} else {
				_, err := r.Events[0].GetFields()
				rocTypedError(t, "ResourceExceeded", err)
				require.Zero(t, r.Events[0].ResponseTo)
				require.Nil(t, r.Events[0].Session)
			}
			require.Empty(t, s.Close("nodes"))
			require.Zero(t, s.Stats().BufferedBytes)
		}
	}
}
