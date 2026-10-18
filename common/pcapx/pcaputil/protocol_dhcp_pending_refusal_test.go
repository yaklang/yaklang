package pcaputil

import (
	"bytes"
	"encoding/binary"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDHCPPendingByteRefusal(t *testing.T) {
	_, v4 := dhcpOwnedInputs(t)
	_, v6w := dhcp6OwnedInputs(t)
	for _, v6 := range []bool{false, true} {
		for _, deny := range []bool{false, true} {
			name := "v4"
			if v6 {
				name = "v6"
			}
			if deny {
				name += "/refused"
			} else {
				name += "/adjacent"
			}
			t.Run(name, func(t *testing.T) {
				req, reply, want := v4[2], v4[3], dhcpOwnedFields(3)
				client, server := uint16(68), uint16(67)
				if v6 {
					req, reply, want = v6w[2], v6w[3], dhcp6OwnedFields(t, 3)
					client, server = 546, 547
				}
				b := DefaultParserBudget()
				b.MaxMessageBytes, b.MaxFrameBytes, b.ProbeBytes = 256, 256, 256
				b.MaxBufferedBytes = 4096
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(client, server))
				require.NoError(t, err)
				defer s.Close("cleanup")
				first := s.Feed(0, time.Unix(100, 0), req)
				require.Nil(t, first.Err)
				require.Len(t, first.Events, 1)
				require.Equal(t, first.Events[0].ID, first.Events[0].TransactionID)
				if deny {
					w := bytes.Clone(req)
					if v6 {
						w = binary.BigEndian.AppendUint16(w, 65000)
						w = binary.BigEndian.AppendUint16(w, 300)
						w = append(w, bytes.Repeat([]byte{0x58}, 300)...)
					} else {
						w = append(w[:len(w)-1], 12, 100)
						w = append(w, bytes.Repeat([]byte{'x'}, 100)...)
						w = append(w, 255)
					}
					out := s.Feed(0, time.Unix(101, 0), w)
					rocTypedError(t, "ResourceExceeded", out.Err)
					require.Len(t, out.Events, 1)
					require.Equal(t, len(w), out.Events[0].Length)
					require.Equal(t, w[:256], out.Events[0].Raw)
					require.Zero(t, out.Events[0].TransactionID)
					require.Zero(t, out.Events[0].ResponseTo)
				}
				out := s.Feed(1, time.Unix(102, 0), reply)
				require.Nil(t, out.Err)
				require.Len(t, out.Events, 1)
				e := out.Events[0]
				if deny {
					require.Zero(t, e.ResponseTo, "observer budget refusal cannot disappear and leave an exact pending owner")
					require.Zero(t, e.TransactionID)
					want["Association"] = "refused-client-xid"
				} else {
					require.Equal(t, first.Events[0].ID, e.ResponseTo)
					require.Equal(t, first.Events[0].ID, e.TransactionID)
				}
				f, err := e.GetFields()
				require.NoError(t, err)
				require.Equal(t, want, f)
				require.Equal(t, want, e.Session)
				require.Equal(t, reply, e.Raw)
				require.Empty(t, e.Error)
				require.Empty(t, s.Close("done"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
