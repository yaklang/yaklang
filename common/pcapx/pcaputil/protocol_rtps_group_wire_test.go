package pcaputil

import (
	"bytes"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRTPSControlGroupWireReplay(t *testing.T) {
	testRTPSSealedReplayProfile(t, rtpsBatchControls(t, "rtps-control-wire", 16), "rtps-control")
}

func TestRTPSControlGroupWireSession(t *testing.T) {
	for _, c := range rtpsBatchControls(t, "rtps-control-wire", 16) {
		t.Run(c.ID, func(t *testing.T) {
			_, want := rtpsInput(t, c)
			s := newRTPSControlTestSession(t, DefaultParserBudget())
			for i, encoded := range want.Wires {
				wire, err := hex.DecodeString(encoded)
				require.NoError(t, err)
				input := bytes.Clone(wire)
				r := s.Feed(i%2, time.Unix(1700000000, int64(i)), input)
				clear(input)
				require.Len(t, r.Events, 1)
				e := r.Events[0]
				require.Equal(t, wire, e.Raw)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				f, err := e.GetFields()
				if want.Events[i].Error != "" {
					assertOwnedProtocolRefusal(t, e, want.Events[i].Error, err)
					require.Nil(t, f)
				} else {
					require.NoError(t, err)
					rocEqualFields(t, want.Events[i].Fields, f)
					rocEqualFields(t, want.Events[i].Fields, e.Session)
					for _, d := range f["Controls"].([]map[string]any) {
						require.NotContains(t, d, "Writer Set Digest")
						require.NotContains(t, d, "Secure Writer Set Digest")
					}
					mutateRTPSQuery(f)
					mutateRTPSQuery(e.Session)
					testNativeObservationSessionQueries(t, e, want.Events[i].Fields)
				}
				require.Zero(t, s.Stats().BufferedBytes)
			}
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestRTPSControlGroupMinimumExistingAPI(t *testing.T) {
	for _, c := range rtpsBatchControls(t, "rtps-control-wire", 16) {
		if c.ID != "group-minimum-v4-le" {
			continue
		}
		raw, want := rtpsInput(t, c)
		var events []*ProtocolEvent
		require.NoError(t, ReplayPcap(bytes.NewReader(raw), WithProtocolDecodeAs("udp", 28400, "rtps-control"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
		require.Len(t, events, 1)
		f, err := events[0].GetFields()
		require.NoError(t, err)
		rocEqualFields(t, want.Events[0].Fields, f)
		rocEqualFields(t, want.Events[0].Fields, events[0].Session)
		return
	}
	t.Fatal("missing independent minimum group wire case")
}
