package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDHCPRefusalIdentityScopes(t *testing.T) {
	_, v4 := dhcpOwnedInputs(t)
	_, v6w := dhcp6OwnedInputs(t)
	for _, v6 := range []bool{false, true} {
		for _, kind := range []string{"first-refused", "foreign-client", "nonstandard", "duid-beyond-prefix", "marker-budget"} {
			if !v6 && (kind == "duid-beyond-prefix" || kind == "marker-budget") {
				continue
			}
			if v6 && kind == "nonstandard" {
				continue
			}
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("v6%t/%s/d%t", v6, kind, deferred), func(t *testing.T) {
					req, reply, want := v4[2], v4[3], dhcpOwnedFields(3)
					sp, dp := uint16(68), uint16(67)
					if v6 {
						req, reply, want = v6w[2], v6w[3], dhcp6OwnedFields(t, 3)
						sp, dp = 546, 547
					}
					if kind == "nonstandard" {
						sp, dp = 40000, 10067
					}
					b := DefaultParserBudget()
					b.MaxMessageBytes, b.MaxFrameBytes, b.ProbeBytes = 256, 256, 256
					b.MaxBufferedBytes = 4096
					if kind == "marker-budget" {
						b.MaxBufferedBytes = 280
					}
					s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(sp, dp))
					require.NoError(t, err)
					defer s.Close("cleanup")
					s.(*captureSession).f.a.config.Deferred = deferred
					var first *ProtocolEvent
					if kind != "first-refused" {
						out := s.Feed(0, time.Unix(100, 0), req)
						require.Nil(t, out.Err)
						require.Len(t, out.Events, 1)
						first = out.Events[0]
						require.Equal(t, first.ID, first.TransactionID)
					}
					denied := bytes.Clone(req)
					if kind == "foreign-client" {
						if v6 {
							denied[21] ^= 1
						} else {
							denied[33] ^= 1
						}
					}
					if v6 {
						if kind == "duid-beyond-prefix" || kind == "marker-budget" {
							denied = append(bytes.Clone(req[:4]), 0xfd, 0xe8, 1, 4)
							denied = append(denied, make([]byte, 260)...)
							denied = append(denied, req[4:]...)
						} else {
							denied = binary.BigEndian.AppendUint16(denied, 65000)
							denied = binary.BigEndian.AppendUint16(denied, 300)
							denied = append(denied, make([]byte, 300)...)
						}
					} else {
						denied = append(denied[:len(denied)-1], 12, 100)
						denied = append(denied, bytes.Repeat([]byte{'x'}, 100)...)
						denied = append(denied, 255)
					}
					out := s.Feed(0, time.Unix(101, 0), denied)
					rocTypedError(t, "ResourceExceeded", out.Err)
					require.Len(t, out.Events, 1)
					e := out.Events[0]
					require.Equal(t, denied[:256], e.Raw)
					require.Equal(t, len(denied), e.Length)
					require.Zero(t, e.TransactionID)
					require.Zero(t, e.ResponseTo)
					tlsDTLSPoison(e, "http")
					f, err := e.GetFields()
					rocTypedError(t, "ResourceExceeded", err)
					require.Nil(t, f)
					if kind == "first-refused" {
						out = s.Feed(0, time.Unix(102, 0), req)
						require.Nil(t, out.Err)
						require.Len(t, out.Events, 1)
						require.Zero(t, out.Events[0].TransactionID)
					}
					out = s.Feed(1, time.Unix(103, 0), reply)
					require.Nil(t, out.Err)
					require.Len(t, out.Events, 1)
					e = out.Events[0]
					if kind == "foreign-client" {
						require.Equal(t, first.ID, e.ResponseTo)
						require.Equal(t, first.ID, e.TransactionID)
					} else {
						require.Zero(t, e.ResponseTo)
						require.Zero(t, e.TransactionID)
						want["Association"] = "refused-client-xid"
					}
					f, err = e.GetFields()
					require.NoError(t, err)
					require.Equal(t, want, f)
					require.Equal(t, want, e.Session)
					require.Equal(t, reply, e.Raw)
					require.Empty(t, e.Error)
					require.LessOrEqual(t, s.Stats().BufferedBytes, int64(b.MaxBufferedBytes))
					require.Empty(t, s.Close("done"))
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}
