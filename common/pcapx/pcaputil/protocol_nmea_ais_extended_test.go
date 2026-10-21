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

func testNMEAAISExtendedSealedReplay(t *testing.T) {
	testNMEANavigationSealedReplay(t, "nmea-ais-extended", "nmea-ais-type19/v1", 3)
	b, err := trafficfixture.ReadFile("nmea-ais-extended/answers/complete-reports.json")
	require.NoError(t, err)
	var answer nmeaNavigationAnswer
	require.NoError(t, json.Unmarshal(b, &answer))
	require.Len(t, answer.Wire, 17)
	require.Len(t, answer.Fields, 17)
	wire, err := hex.DecodeString(answer.Wire[0])
	require.NoError(t, err)
	for _, deferred := range []bool{false, true} {
		s, err := NewProtocolSessionWithOptions(ParserBudget{}, WithSessionTransport("udp"), WithSessionPorts(38000, 10110))
		require.NoError(t, err)
		s.(*captureSession).f.a.config.Deferred = deferred
		for i, encoded := range answer.Wire {
			w, err := hex.DecodeString(encoded)
			require.NoError(t, err)
			for dir := 0; dir < 2; dir++ {
				owned := bytes.Clone(w)
				out := s.Feed(dir, time.Unix(int64(i+1), 0), owned)
				clear(owned)
				require.Nil(t, out.Err)
				require.Len(t, out.Events, 1)
				require.Equal(t, len(w), out.Consumed)
				e := out.Events[0]
				require.Equal(t, "nmea-native", e.Profile)
				require.Equal(t, w, e.Raw)
				require.Equal(t, dir, e.Direction)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				rocEqualFields(t, answer.Fields[i], e.Session)
				testNativeObservationSessionQueries(t, e, answer.Fields[i])
				require.Zero(t, s.Stats().BufferedBytes)
			}
		}
		// A UDP feed is a complete datagram, never a stream fragment.
		// Adjacent reports do not acquire an association merely by MMSI.
		for _, chunk := range [][]byte{wire[:20], wire[20:]} {
			out := s.Feed(0, time.Unix(30, 0), chunk)
			rocTypedError(t, "UnsupportedFeature", out.Err)
			require.Len(t, out.Events, 1)
			require.Nil(t, out.Events[0].Session)
		}
		require.Empty(t, s.Close("done"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
	t.Run("projection-budget-refusal-and-recovery", func(t *testing.T) {
		pair, err := hex.DecodeString(answer.Wire[16])
		require.NoError(t, err)
		for _, budget := range []ParserBudget{{MaxCollectionElements: 1}, {MaxRecursionDepth: 1}, {MaxMessageBytes: len(pair) - 1}} {
			s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("udp"), WithSessionPorts(38000, 10110))
			require.NoError(t, err)
			out := s.Feed(0, time.Unix(1, 0), pair)
			rocTypedError(t, "ResourceExceeded", out.Err)
			require.Len(t, out.Events, 1)
			require.Nil(t, out.Events[0].Session)
			_, err = out.Events[0].GetFields()
			assertOwnedProtocolRefusal(t, out.Events[0], "ResourceExceeded", err)
			out = s.Feed(1, time.Unix(2, 0), wire)
			require.Nil(t, out.Err)
			require.Len(t, out.Events, 1)
			rocEqualFields(t, answer.Fields[0], out.Events[0].Session)
			require.Empty(t, s.Close("done"))
			require.Zero(t, s.Stats().BufferedBytes)
		}
	})
	t.Run("all-prefixes-refused", func(t *testing.T) {
		for _, encoded := range answer.Wire[:13] {
			w, err := hex.DecodeString(encoded)
			require.NoError(t, err)
			for n := 0; n < len(w); n++ {
				fields, err := decodeNMEADatagram(w[:n], 4)
				require.Error(t, err)
				require.Nil(t, fields)
			}
		}
	})
}
