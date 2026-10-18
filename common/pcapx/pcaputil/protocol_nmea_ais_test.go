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

func testNMEAAISSealedReplay(t *testing.T) {
	testNMEANavigationSealedReplay(t, "nmea-ais", "nmea-ais-position/v1", 74)
	read := func(id string) ([]byte, map[string]any) {
		b, err := trafficfixture.ReadFile("nmea-ais/answers/" + id + ".json")
		require.NoError(t, err)
		var a nmeaNavigationAnswer
		require.NoError(t, json.Unmarshal(b, &a))
		require.Len(t, a.Wire, 1)
		w, err := hex.DecodeString(a.Wire[0])
		require.NoError(t, err)
		return w, a.Fields[0]
	}
	one, want := read("received-type1-payload")
	two, _ := read("two-in-one-datagram")
	for _, scenario := range []struct {
		name   string
		budget ParserBudget
	}{
		{"sentences", ParserBudget{MaxCollectionElements: 1}},
		{"projection", ParserBudget{MaxRecursionDepth: 1}},
		{"bytes", ParserBudget{MaxMessageBytes: len(two) - 1}},
	} {
		for _, deferred := range []bool{false, true} {
			t.Run(scenario.name, func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(scenario.budget, WithSessionTransport("udp"), WithSessionPorts(38000, 10110))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = deferred
				input := bytes.Clone(two)
				out := s.Feed(0, time.Unix(1, 0), input)
				clear(input)
				require.Equal(t, len(two), out.Consumed)
				rocTypedError(t, "ResourceExceeded", out.Err)
				require.Len(t, out.Events, 1)
				require.Nil(t, out.Events[0].Session)
				f, err := out.Events[0].GetFields()
				rocTypedError(t, "ResourceExceeded", err)
				require.Nil(t, f)
				for direction := 0; direction < 2; direction++ {
					next := s.Feed(direction, time.Unix(int64(2+direction), 0), one)
					require.Nil(t, next.Err)
					require.Equal(t, len(one), next.Consumed)
					require.Len(t, next.Events, 1)
					require.Zero(t, next.Events[0].ResponseTo)
					require.Zero(t, next.Events[0].TransactionID)
					testNativeObservationSessionQueries(t, next.Events[0], want)
				}
				require.Empty(t, s.Close("done"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
