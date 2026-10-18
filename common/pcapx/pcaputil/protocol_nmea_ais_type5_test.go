package pcaputil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"testing"
	"time"
)

func testNMEAAIS5SealedReplay(t *testing.T) {
	testNMEANavigationSealedReplay(t, "nmea-ais-type5", "nmea-ais-type5-datagram/v1", 30)
	read := func(root, id string) ([]byte, map[string]any) {
		b, err := trafficfixture.ReadFile(root + "/answers/" + id + ".json")
		require.NoError(t, err)
		var a nmeaNavigationAnswer
		require.NoError(t, json.Unmarshal(b, &a))
		require.Len(t, a.Wire, 1)
		require.Len(t, a.Fields, 1)
		w, err := hex.DecodeString(a.Wire[0])
		require.NoError(t, err)
		return w, a.Fields[0]
	}
	pair, want := read("nmea-ais-type5", "voyage-reported")
	adjacent, adjacentWant := read("nmea-ais", "received-type1-payload")
	for _, b := range []struct {
		name   string
		budget ParserBudget
	}{{"sentence-count", ParserBudget{MaxCollectionElements: 1}}, {"projection-depth", ParserBudget{MaxRecursionDepth: 1}}, {"bytes", ParserBudget{MaxMessageBytes: len(pair) - 1}}} {
		for _, deferred := range []bool{false, true} {
			t.Run(b.name, func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(b.budget, WithSessionTransport("udp"), WithSessionPorts(38000, 10110))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = deferred
				in := bytes.Clone(pair)
				out := s.Feed(0, time.Unix(1, 0), in)
				clear(in)
				require.Equal(t, len(pair), out.Consumed)
				rocTypedError(t, "ResourceExceeded", out.Err)
				require.Len(t, out.Events, 1)
				if b.name == "bytes" {
					require.Equal(t, pair[:64], out.Events[0].Raw)
					require.Equal(t, len(pair), out.Events[0].Length)
				} else {
					require.Equal(t, pair, out.Events[0].Raw)
				}
				require.Nil(t, out.Events[0].Session)
				fields, err := out.Events[0].GetFields()
				require.Nil(t, fields)
				rocTypedError(t, "ResourceExceeded", err)
				assertOwnedProtocolRefusal(t, out.Events[0], "ResourceExceeded", err)
				for dir := 0; dir < 2; dir++ {
					good := s.Feed(dir, time.Unix(int64(dir+2), 0), adjacent)
					require.Nil(t, good.Err)
					require.Len(t, good.Events, 1)
					require.Equal(t, dir, good.Events[0].Direction)
					require.Equal(t, "nmea-native", good.Events[0].Profile)
					rocEqualFields(t, adjacentWant, good.Events[0].Session)
					testNativeObservationSessionQueries(t, good.Events[0], adjacentWant)
				}
				require.Empty(t, s.Close("done"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
	t.Run("datagram-boundary-and-recovery", func(t *testing.T) {
		for _, deferred := range []bool{false, true} {
			s, err := NewProtocolSessionWithOptions(ParserBudget{}, WithSessionTransport("udp"), WithSessionPorts(38000, 10110))
			require.NoError(t, err)
			s.(*captureSession).f.a.config.Deferred = deferred
			cut := bytes.IndexByte(pair, '\n') + 1
			require.Positive(t, cut)
			for _, fragment := range [][]byte{pair[:cut], pair[cut:]} {
				out := s.Feed(0, time.Unix(1, 0), fragment)
				rocTypedError(t, "UnsupportedFeature", out.Err)
				require.Len(t, out.Events, 1)
				require.Nil(t, out.Events[0].Session)
				require.Zero(t, s.Stats().BufferedBytes)
			}
			for _, dir := range []int{0, 1, 0} {
				owned := bytes.Clone(pair)
				out := s.Feed(dir, time.Unix(2, 0), owned)
				clear(owned)
				require.Nil(t, out.Err)
				require.Equal(t, len(pair), out.Consumed)
				require.Len(t, out.Events, 1)
				e := out.Events[0]
				require.Equal(t, "nmea-native", e.Profile)
				require.Equal(t, dir, e.Direction)
				require.Equal(t, pair, e.Raw)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				rocEqualFields(t, want, e.Session)
				testNativeObservationSessionQueries(t, e, want)
			}
			require.Empty(t, s.Close("done"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		}
	})
	t.Run("every-truncation-refuses-without-prefix", func(t *testing.T) {
		for n := 0; n < len(pair); n++ {
			fields, err := decodeNMEADatagram(pair[:n], 4)
			require.Error(t, err)
			require.Nil(t, fields)
		}
	})
}
