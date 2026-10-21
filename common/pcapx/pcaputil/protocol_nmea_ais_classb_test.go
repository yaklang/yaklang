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

func testNMEAAISClassBSealedReplay(t *testing.T) {
	testNMEANavigationSealedReplay(t, "nmea-ais-classb", "nmea-ais-type18/v1", 39)
	read := func(id string) ([]byte, map[string]any) {
		b, err := trafficfixture.ReadFile("nmea-ais-classb/answers/" + id + ".json")
		require.NoError(t, err)
		var a nmeaNavigationAnswer
		require.NoError(t, json.Unmarshal(b, &a))
		require.Len(t, a.Wire, 1)
		w, err := hex.DecodeString(a.Wire[0])
		require.NoError(t, err)
		return w, a.Fields[0]
	}
	wire, want := read("sotdma-0")
	for _, deferred := range []bool{false, true} {
		for _, budget := range []ParserBudget{{}, {MaxCollectionElements: 1, MaxRecursionDepth: 1}} {
			s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("udp"), WithSessionPorts(38000, 10110))
			require.NoError(t, err)
			s.(*captureSession).f.a.config.Deferred = deferred
			for _, dir := range []int{0, 1, 0} {
				owned := bytes.Clone(wire)
				out := s.Feed(dir, time.Unix(1, 0), owned)
				clear(owned)
				require.Nil(t, out.Err)
				require.Len(t, out.Events, 1)
				e := out.Events[0]
				require.Equal(t, "nmea-native", e.Profile)
				require.Equal(t, wire, e.Raw)
				require.Equal(t, dir, e.Direction)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				rocEqualFields(t, want, e.Session)
				testNativeObservationSessionQueries(t, e, want)
				require.Zero(t, s.Stats().BufferedBytes)
			}
			// A UDP sentence split does not form a byte stream. A subsequent
			// complete report still succeeds in either direction.
			for _, chunk := range [][]byte{wire[:20], wire[20:]} {
				out := s.Feed(0, time.Unix(2, 0), chunk)
				rocTypedError(t, "UnsupportedFeature", out.Err)
				require.Len(t, out.Events, 1)
				require.Nil(t, out.Events[0].Session)
			}
			out := s.Feed(1, time.Unix(3, 0), wire)
			require.Nil(t, out.Err)
			require.Len(t, out.Events, 1)
			rocEqualFields(t, want, out.Events[0].Session)
			require.Empty(t, s.Close("done"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		}
	}
	t.Run("complete-copacked-budget-refusal-and-recovery", func(t *testing.T) {
		pair, _ := read("copacked-observations")
		for _, b := range []ParserBudget{{MaxCollectionElements: 1}, {MaxRecursionDepth: 1}, {MaxMessageBytes: len(pair) - 1}} {
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(38000, 10110))
			require.NoError(t, err)
			out := s.Feed(0, time.Unix(1, 0), pair)
			rocTypedError(t, "ResourceExceeded", out.Err)
			require.Len(t, out.Events, 1)
			require.Nil(t, out.Events[0].Session)
			_, err = out.Events[0].GetFields()
			assertOwnedProtocolRefusal(t, out.Events[0], "ResourceExceeded", err)
			out = s.Feed(0, time.Unix(2, 0), wire)
			require.Nil(t, out.Err)
			require.Len(t, out.Events, 1)
			rocEqualFields(t, want, out.Events[0].Session)
			require.Empty(t, s.Close("done"))
			require.Zero(t, s.Stats().BufferedBytes)
		}
	})
	t.Run("every-truncation-has-no-success-prefix", func(t *testing.T) {
		for n := 0; n < len(wire); n++ {
			f, err := decodeNMEADatagram(wire[:n], 4)
			require.Error(t, err)
			require.Nil(t, f)
		}
	})
}
