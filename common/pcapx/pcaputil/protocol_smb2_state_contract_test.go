package pcaputil

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSMB2CurrentNegotiateDialectIsNotCachedContext(t *testing.T) {
	for _, next := range []uint16{0x0210, 0x02ff} {
		t.Run(smb2DialectName(next), func(t *testing.T) {
			state := &binSMB2{client: 0, dialect: 0x0311}
			request, err := state.consumeFrom(0, smb2NegotiateReq(next), 64)
			require.NoError(t, err)
			require.NotContains(t, request, "Dialect", "a request contains offers, not a selected dialect")
			require.Equal(t, uint16(0x0311), request["Connection Dialect"])
			wire := smb2NegotiateResp(next)
			response, err := state.consumeFrom(1, wire, 64)
			require.NoError(t, err)
			require.Equal(t, next, binary.LittleEndian.Uint16(wire[72:74]))
			require.Equal(t, next, response["Dialect"], "preserve this response's literal value")
			require.Equal(t, smb2DialectName(next), response["Version Name"])
			require.Equal(t, uint16(0x0311), response["Connection Dialect"], "preexisting context is separately named")
			if next == 0x02ff {
				require.Zero(t, state.dialect)
				require.Equal(t, false, response["Dialect Negotiated"])
				require.Equal(t, true, response["SMB2 Negotiate Required"])
			} else {
				require.Equal(t, next, state.dialect)
			}
		})
	}
}

func TestSMB2UnmatchedOrSameDirectionCannotSelectDialect(t *testing.T) {
	for _, test := range []struct {
		name    string
		request bool
		dir     int
	}{
		{"missing-request", false, 1},
		{"same-direction", true, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := &binSMB2{client: 0, dialect: 0x0311}
			if test.request {
				_, err := state.consumeFrom(0, smb2NegotiateReq(0x0210), 64)
				require.NoError(t, err)
			}
			response, err := state.consumeFrom(test.dir, smb2NegotiateResp(0x0210), 64)
			require.NoError(t, err)
			require.Equal(t, uint16(0x0210), response["Dialect"])
			require.Equal(t, false, response["Dialect Context Updated"])
			require.Equal(t, true, response["Unmatched"])
			require.Equal(t, uint16(0x0311), state.dialect)
			if test.request {
				require.Len(t, state.pending, 1)
				matched, err := state.consumeFrom(1, smb2NegotiateResp(0x0210), 64)
				require.NoError(t, err)
				require.Equal(t, "matched", matched["Association Status"])
				require.Equal(t, uint16(0x0210), state.dialect)
			}
		})
	}
}
