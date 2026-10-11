package pcaputil

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Reuse pinned independent field answers, including ordered packs. A root
// observation and its scalar per-message maps require two projection levels.
// Routing is private fixture setup because byte sessions currently expose no
// public DecodeAs option; budget, Feed, Decode, Stats and Close are real APIs.
func testOpenDroneIDDepthBudget(t *testing.T) {
	type input struct {
		profile, id string
		answer      bsapAnswer
	}
	var inputs []input
	for _, c := range odidControls(t) {
		if c.ID == "basic-serial" || c.ID == "pack-two-basic-location" {
			_, a := odidInput(t, c)
			inputs = append(inputs, input{"opendroneid", c.ID, a})
		}
	}
	for _, c := range odidSystemControls(t) {
		if c.ID == "system-normal" || c.ID == "pack-two-basic-location-system" {
			_, a := odidSelectedInput(t, "opendroneid-system", c.bsapControl)
			inputs = append(inputs, input{"opendroneid-system", c.ID, a})
		}
	}
	for _, c := range odidIdentityControls(t) {
		if c.ID == "self-description" || c.ID == "pack-six-ordered" {
			_, a := odidAliasedInput(t, "opendroneid-identity", c.bsapControl, c.CaptureAlias)
			inputs = append(inputs, input{"opendroneid-identity", c.ID, a})
		}
	}
	require.Len(t, inputs, 6)
	for _, tc := range inputs {
		t.Run(tc.profile+"/"+tc.id, func(t *testing.T) {
			require.Len(t, tc.answer.Events, 1)
			raw, err := hex.DecodeString(tc.answer.Events[0].Raw)
			require.NoError(t, err)
			for _, depth := range []int{1, 2} {
				t.Run(fmt.Sprintf("depth%d", depth), func(t *testing.T) {
					s, err := NewProtocolSessionWithOptions(ParserBudget{MaxRecursionDepth: depth}, WithSessionTransport("udp"), WithSessionPorts(39200, 28400))
					require.NoError(t, err)
					s.(*captureSession).f.a.datagramDecodeAs[28400] = tc.profile
					var previous uint64
					for dir := 0; dir < 2; dir++ {
						borrowed := bytes.Clone(raw)
						g := s.Feed(dir, time.Unix(int64(dir+1), 0), borrowed)
						clear(borrowed)
						require.Len(t, g.Events, 1)
						e := g.Events[0]
						require.Greater(t, e.ID, previous)
						previous = e.ID
						require.Equal(t, dir, e.Direction)
						require.Equal(t, raw, e.Raw)
						require.Zero(t, e.ResponseTo)
						require.Zero(t, e.TransactionID)
						require.Zero(t, e.FlowID)
					if depth == 1 {
						require.NotNil(t, g.Err, "configured projection depth must reject nested message maps")
						rocTypedError(t, "ResourceExceeded", g.Err)
							require.Nil(t, e.Session)
							require.Nil(t, e.Fields)
							require.Nil(t, e.Structured)
							fields, err := e.Decode()
							require.Nil(t, fields)
							rocTypedError(t, "ResourceExceeded", err)
						} else {
							require.Nil(t, g.Err)
							rocEqualFields(t, tc.answer.Events[0].Fields, e.Session)
							fields, err := e.Decode()
							require.NoError(t, err)
							rocEqualFields(t, tc.answer.Events[0].Fields, protocolFields(fields))
							rocEqualFields(t, tc.answer.Events[0].Fields, fields["session"].(map[string]any))
						}
						require.Zero(t, s.Stats().BufferedBytes)
					}
					// Layout diagnostics remain authoritative before projection admission.
					bad := bytes.Clone(raw)
					bad[0] = (bad[0] & 0xf0) | 3
				g := s.Feed(0, time.Unix(3, 0), bad)
				require.NotNil(t, g.Err)
					rocTypedError(t, "UnsupportedVersion", g.Err)
					require.Len(t, g.Events, 1)
					require.Nil(t, g.Events[0].Session)
					require.Zero(t, s.Stats().BufferedBytes)
					s.Close("done")
					s.Close("again")
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		})
	}
}
