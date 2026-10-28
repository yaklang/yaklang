package pcaputil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func testNMEAGSASealedReplay(t *testing.T) {
	testNMEANavigationSealedReplay(t, "nmea-gsa", "nmea-gsa-slots/v1", 53)
	// Use the complete frozen multi-system answer. Collection/depth refusal
	// must not install a partial Session, combine independent systems, or
	// prevent the next single flat observation from being used.
	b, err := trafficfixture.ReadFile("nmea-gsa/answers/two-system-observations.json")
	require.NoError(t, err)
	var a nmeaNavigationAnswer
	require.NoError(t, json.Unmarshal(b, &a))
	require.Len(t, a.Wire, 1)
	wire, err := hex.DecodeString(a.Wire[0])
	require.NoError(t, err)
	end := bytes.IndexByte(wire, '\n') + 1
	require.Positive(t, end)
	var msgs []map[string]any
	encoded, err := json.Marshal(a.Fields[0]["Messages"])
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, &msgs))
	require.Len(t, msgs, 2)
	for _, budget := range []ParserBudget{{MaxCollectionElements: 1}, {MaxRecursionDepth: 1}} {
		for _, deferred := range []bool{false, true} {
			s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("udp"), WithSessionPorts(38000, 10110))
			require.NoError(t, err)
			s.(*captureSession).f.a.config.Deferred = deferred
			input := bytes.Clone(wire)
			out := s.Feed(0, time.Unix(1, 0), input)
			clear(input)
			require.Equal(t, len(wire), out.Consumed)
			rocTypedError(t, "ResourceExceeded", out.Err)
			require.Len(t, out.Events, 1)
			require.Nil(t, out.Events[0].Session)
			fields, err := out.Events[0].GetFields()
			rocTypedError(t, "ResourceExceeded", err)
			require.Nil(t, fields)
			for direction := 0; direction < 2; direction++ {
				next := s.Feed(direction, time.Unix(int64(2+direction), 0), wire[:end])
				require.Nil(t, next.Err)
				require.Len(t, next.Events, 1)
				want := cloneSession(msgs[0])
				want["Message Count"] = float64(1)
				testNativeObservationSessionQueries(t, next.Events[0], want)
				require.Zero(t, next.Events[0].ResponseTo)
				require.Zero(t, next.Events[0].TransactionID)
			}
			require.Empty(t, s.Close("done"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		}
	}
}
