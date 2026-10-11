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

func testNMEAGSVSealedReplay(t *testing.T) {
	testNMEANavigationSealedReplay(t, "nmea-gsv", "nmea-gsv-slots/v1", 68)
	// The former unsupported-type control is now an explicitly answered
	// normal observation; invalid part numbering remains a negative nearby.
	var previous map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{
  "Checksum": 127,
  "Checksum Valid": true,
  "Context Level": "observed",
  "Cycle Context": "unassembled",
  "Cycle Message Count": 1,
  "Cycle Message Number": 1,
  "Message Count": 1,
  "Packet Name": "GSV",
  "Reported Satellites In View": 4,
  "Reported Satellites In View Wire": "04",
  "Satellite Slot 1 Azimuth Degrees": 83,
  "Satellite Slot 1 Azimuth Degrees Present": true,
  "Satellite Slot 1 Azimuth Degrees Wire": "083",
  "Satellite Slot 1 Elevation Degrees": 40,
  "Satellite Slot 1 Elevation Degrees Present": true,
  "Satellite Slot 1 Elevation Degrees Wire": "40",
  "Satellite Slot 1 SV ID": 1,
  "Satellite Slot 1 SV ID Present": true,
  "Satellite Slot 1 SV ID Wire": "01",
  "Satellite Slot 1 Signal Level": 46,
  "Satellite Slot 1 Signal Level Present": true,
  "Satellite Slot 1 Signal Level Wire": "46",
  "Satellite Slot 2 Azimuth Degrees": 308,
  "Satellite Slot 2 Azimuth Degrees Present": true,
  "Satellite Slot 2 Azimuth Degrees Wire": "308",
  "Satellite Slot 2 Elevation Degrees": 17,
  "Satellite Slot 2 Elevation Degrees Present": true,
  "Satellite Slot 2 Elevation Degrees Wire": "17",
  "Satellite Slot 2 SV ID": 2,
  "Satellite Slot 2 SV ID Present": true,
  "Satellite Slot 2 SV ID Wire": "02",
  "Satellite Slot 2 Signal Level": 42,
  "Satellite Slot 2 Signal Level Present": true,
  "Satellite Slot 2 Signal Level Wire": "42",
  "Satellite Slot 3 Azimuth Degrees": 123,
  "Satellite Slot 3 Azimuth Degrees Present": true,
  "Satellite Slot 3 Azimuth Degrees Wire": "123",
  "Satellite Slot 3 Elevation Degrees": 22,
  "Satellite Slot 3 Elevation Degrees Present": true,
  "Satellite Slot 3 Elevation Degrees Wire": "22",
  "Satellite Slot 3 SV ID": 3,
  "Satellite Slot 3 SV ID Present": true,
  "Satellite Slot 3 SV ID Wire": "03",
  "Satellite Slot 3 Signal Level": 45,
  "Satellite Slot 3 Signal Level Present": true,
  "Satellite Slot 3 Signal Level Wire": "45",
  "Satellite Slot 4 Azimuth Degrees": 234,
  "Satellite Slot 4 Azimuth Degrees Present": true,
  "Satellite Slot 4 Azimuth Degrees Wire": "234",
  "Satellite Slot 4 Elevation Degrees": 30,
  "Satellite Slot 4 Elevation Degrees Present": true,
  "Satellite Slot 4 Elevation Degrees Wire": "30",
  "Satellite Slot 4 SV ID": 4,
  "Satellite Slot 4 SV ID Present": true,
  "Satellite Slot 4 SV ID Wire": "04",
  "Satellite Slot 4 Signal Level": 43,
  "Satellite Slot 4 Signal Level Present": true,
  "Satellite Slot 4 Signal Level Wire": "43",
  "Satellite Slot Count": 4,
  "Sentence ID": "GPGSV",
  "Signal ID Present": false,
  "Talker": "GP"
}`), &previous))
	f, err := decodeNMEADatagram(nmeaTestSentence("GPGSV,1,1,04,01,40,083,46,02,17,308,42,03,22,123,45,04,30,234,43"), 4)
	require.NoError(t, err)
	rocEqualFields(t, previous, f)

	read := func(id string) ([]byte, map[string]any) {
		b, err := trafficfixture.ReadFile("nmea-gsv/answers/" + id + ".json")
		require.NoError(t, err)
		var a nmeaNavigationAnswer
		require.NoError(t, json.Unmarshal(b, &a))
		require.Len(t, a.Wire, 1)
		w, err := hex.DecodeString(a.Wire[0])
		require.NoError(t, err)
		return w, a.Fields[0]
	}
	large, _ := read("blank-signal") // Four slots: even empty values cost slots.
	small, want := read("unknown-sv-id")
	two, _ := read("two-in-one-datagram")
	for _, scenario := range []struct {
		name   string
		budget ParserBudget
		wire   []byte
	}{
		{"slots", ParserBudget{MaxCollectionElements: 1}, large},
		{"messages", ParserBudget{MaxCollectionElements: 1}, two},
		{"projection", ParserBudget{MaxRecursionDepth: 1}, two},
	} {
		for _, deferred := range []bool{false, true} {
			t.Run(scenario.name, func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(scenario.budget, WithSessionTransport("udp"), WithSessionPorts(38000, 10110))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = deferred
				input := bytes.Clone(scenario.wire)
				out := s.Feed(0, time.Unix(1, 0), input)
				clear(input)
				require.Equal(t, len(scenario.wire), out.Consumed)
				rocTypedError(t, "ResourceExceeded", out.Err)
				require.Len(t, out.Events, 1)
				require.Nil(t, out.Events[0].Session)
				f, err := out.Events[0].GetFields()
				rocTypedError(t, "ResourceExceeded", err)
				require.Nil(t, f)
				for direction := 0; direction < 2; direction++ {
					next := s.Feed(direction, time.Unix(int64(2+direction), 0), small)
					require.Nil(t, next.Err)
					require.Len(t, next.Events, 1)
					testNativeObservationSessionQueries(t, next.Events[0], want)
					require.Zero(t, next.Events[0].ResponseTo)
					require.Zero(t, next.Events[0].TransactionID)
				}
				require.Empty(t, s.Close("done"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
