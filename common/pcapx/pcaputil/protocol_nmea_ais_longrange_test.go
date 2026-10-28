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

func TestNMEAAISLongRangeSealedReplay(t *testing.T) {
	testNMEANavigationSealedReplay(t, "nmea-ais-longrange", "nmea-ais-type27/v1", 3)
	b, err := trafficfixture.ReadFile("nmea-ais-longrange/answers/complete-reports.json")
	require.NoError(t, err)
	var answer nmeaNavigationAnswer
	require.NoError(t, json.Unmarshal(b, &answer))
	require.Len(t, answer.Wire, 14)
	require.Len(t, answer.Fields, 14)
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
		pair, err := hex.DecodeString(answer.Wire[13])
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
		for _, encoded := range answer.Wire[:10] {
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

// The fixed ITU table is authoritative for latency semantics; the reference
// codec's inverse field named gnss does not establish GNSS validity.
func TestNMEAAISLongRangeNormativeFields(t *testing.T) {
	b, err := trafficfixture.ReadFile("nmea-ais-longrange/answers/complete-reports.json")
	require.NoError(t, err)
	var a nmeaNavigationAnswer
	require.NoError(t, json.Unmarshal(b, &a))
	for i := 0; i < 10; i++ {
		w, err := hex.DecodeString(a.Wire[i])
		require.NoError(t, err)
		f, err := decodeNMEADatagram(w, 4)
		require.NoError(t, err)
		rocEqualFields(t, a.Fields[i], f)
		for _, k := range []string{"GNSS Valid", "GNSS Position Current", "UTC Second", "Time Stamp Raw", "True Heading Raw", "Communication State Raw", "Speed Over Ground Lower Bound"} {
			require.NotContains(t, f, k)
		}
	}
	w, err := hex.DecodeString(a.Wire[1])
	require.NoError(t, err)
	f, err := decodeNMEADatagram(w, 4)
	require.NoError(t, err)
	require.Equal(t, 62, f["Speed Knots"])
	require.Equal(t, "greater-than-5-seconds", f["Position Latency Status"])
	require.Equal(t, float64(-180), f["Longitude"])
	require.Equal(t, float64(-90), f["Latitude"])
	w, err = hex.DecodeString(a.Wire[3])
	require.NoError(t, err)
	f, err = decodeNMEADatagram(w, 4)
	require.NoError(t, err)
	for _, k := range []string{"Longitude", "Latitude", "Speed Knots", "Course Degrees True"} {
		require.NotContains(t, f, k)
	}
	require.Equal(t, 108600, f["Longitude Raw"])
	require.Equal(t, 54600, f["Latitude Raw"])
	require.False(t, f["Position Available"].(bool))
	w, err = hex.DecodeString(a.Wire[7])
	require.NoError(t, err)
	f, err = decodeNMEADatagram(w, 4)
	require.NoError(t, err)
	require.False(t, f["Repeat Indicator Conforms"].(bool))
	require.False(t, f["Spare Bits Conform"].(bool))
}
