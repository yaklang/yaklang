package pcaputil

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"testing"
	"time"
)

func TestDLMSWrapperStringsExistingAPIBaseline(t *testing.T) {
	for _, wire := range []string{"0001000100100008c403c10100120100", "0001000100100009c403c101000a024142", "0001000100100009c403c101000c02cebb"} {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
		require.NoError(t, err)
		out := s.Feed(1, time.Unix(1, 0), wrapperWire(t, wire))
		require.Nil(t, out.Err)
		require.Len(t, out.Events, 1)
		fields, err := out.Events[0].GetFields()
		require.NoError(t, err)
		require.NotNil(t, fields)
		require.Zero(t, out.Events[0].ResponseTo)
		require.Zero(t, out.Events[0].TransactionID)
		require.Empty(t, s.Close("strings-baseline"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
}

func TestDLMSWrapperStringsSealedMatrix(t *testing.T) {
	wrapperOwnedDataMatrix(t, wrapperOwnedDataControls(t, "dlms-strings", 31))
}
func TestDLMSWrapperStringsBudgetsOwnership(t *testing.T) {
	wrapperOwnedDataBudgetsOwnership(t, wrapperOwnedDataControls(t, "dlms-strings", 31))
}

// Current capability answer for an immutable historical input. The old ZIP
// keeps its former Unsupported answer; the new archive records this transition.
func wrapperStringsHistoricalAnswers(t *testing.T, old wrapperStructuredControl) []wrapperListAnswer {
	raw, err := trafficfixture.ReadFile("dlms-strings/answers/historical-text-now-supported.json")
	require.NoError(t, err)
	var c wrapperStructuredControl
	require.NoError(t, json.Unmarshal(raw, &c))
	require.Equal(t, old.SHA256, c.SHA256)
	require.Len(t, old.Answers, 1)
	require.Len(t, c.Answers, 1)
	require.Equal(t, old.Answers[0].Wire, c.Answers[0].Wire)
	require.Nil(t, c.Answers[0].Error)
	return c.Answers
}
