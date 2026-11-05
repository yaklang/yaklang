package pcaputil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzTLSCloseNotifyDirectionAndFragmentProof(f *testing.F) {
	for _, seed := range [][]byte{{0, 1, 0}, {1, 1, 0}, {1, 2, 0}, {0, 1, 90}, {2, 1, 0}, {4, 1, 0}, {8, 1, 0}, {0, 1, 0, 1}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) == 0 || len(input) > 257 {
			return
		}
		suite := uint16(0xc02f)
		if input[0]&1 != 0 {
			suite = 0x1301
		}
		flow, tls, events := closeNotifyTestFlow(t, suite, 1, false)
		closeNotifyTestSend(t, flow, tls, 0, 23, []byte("GET / HTTP/1.1\r\nHost: owned.invalid\r\n\r\n"), false)
		closeNotifyTestSend(t, flow, tls, 1, 23, []byte("HTTP/1.1 200 OK\r\n\r\nbody"), false)
		plain := input[1:]
		badTag, gap := input[0]&2 != 0, input[0]&4 != 0
		if gap {
			flow.markTLSInnerGap(1)
		}
		closeNotifyTestSend(t, flow, tls, 1, 21, plain, badTag)
		complete, _ := closeNotifyTestHTTP(*events)
		// Independent literal alert grammar. This oracle does not consult the
		// implementation's closure flags, public Session, or derived fields.
		possible := false
		if suite == 0x1301 {
			possible = !badTag && !gap && len(plain) == 2 && plain[1] == 0
		} else {
			for at := 0; !badTag && !gap && at+1 < len(plain); at += 2 {
				if plain[at] == 2 {
					break // A preceding fatal alert cannot become a clean close.
				}
				if plain[at] == 1 && plain[at+1] == 0 {
					possible = true
					break
				}
			}
		}
		if len(complete) == 2 {
			require.True(t, possible, "an unverified, unaligned or missing alert cannot certify HTTP EOF")
		}
		if possible {
			require.Len(t, complete, 2)
		} else {
			require.Len(t, complete, 1)
		}
	})
}
