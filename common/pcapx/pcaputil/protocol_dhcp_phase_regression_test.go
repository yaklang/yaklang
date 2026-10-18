package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDHCPPhaseBootstrapRole(t *testing.T) {
	_, w := dhcpOwnedInputs(t)
	for _, bad := range []bool{false, true} {
		t.Run(fmt.Sprintf("wrong-op%t", bad), func(t *testing.T) {
			wire := bytes.Clone(w[2])
			if bad {
				wire[0] = 2
			}
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(68, 67))
			require.NoError(t, err)
			defer s.Close("cleanup")
			o := s.Feed(0, time.Unix(100, 0), wire)
			require.Len(t, o.Events, 1)
			if bad {
				rocTypedError(t, "MalformedMessage", o.Err)
				f, err := o.Events[0].GetFields()
				rocTypedError(t, "MalformedMessage", err)
				require.Nil(t, f)
				require.Zero(t, o.Events[0].TransactionID)
			} else {
				require.Nil(t, o.Err)
				f, err := o.Events[0].GetFields()
				require.NoError(t, err)
				require.Equal(t, dhcpOwnedFields(2), f)
				require.Equal(t, o.Events[0].ID, o.Events[0].TransactionID)
			}
			require.Empty(t, s.Close("done"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func dhcpPhaseAddServer(w []byte, server [4]byte) []byte {
	out := bytes.Clone(w[:len(w)-1])
	out = append(out, 54, 4)
	out = append(out, server[:]...)
	return append(out, 255)
}
func dhcpPhaseChange6Option(t *testing.T, w []byte, code uint16) []byte {
	t.Helper()
	out := bytes.Clone(w)
	for p := 4; p+4 <= len(out); {
		c, n := binary.BigEndian.Uint16(out[p:]), int(binary.BigEndian.Uint16(out[p+2:]))
		p += 4
		require.LessOrEqual(t, n, len(out)-p)
		if c == code {
			require.Positive(t, n)
			out[p+n-1] ^= 1
			return out
		}
		p += n
	}
	t.Fatal("fixed independent input lacks selected option")
	return nil
}
func TestDHCPPhaseSelectedServer(t *testing.T) {
	_, v4 := dhcpOwnedInputs(t)
	_, v6 := dhcp6OwnedInputs(t)
	for _, six := range []bool{false, true} {
		for _, wrong := range []bool{false, true} {
			t.Run(fmt.Sprintf("v6%t/wrong%t", six, wrong), func(t *testing.T) {
				req, reply := dhcpPhaseAddServer(v4[2], [4]byte{192, 0, 2, 67}), dhcpPhaseAddServer(v4[3], [4]byte{192, 0, 2, 67})
				sp, dp := uint16(68), uint16(67)
				if six {
					req, reply = v6[2], v6[3]
					sp, dp = 546, 547
				}
				bad := bytes.Clone(reply)
				if wrong {
					if six {
						bad = dhcpPhaseChange6Option(t, reply, 2)
					} else {
						bad = dhcpPhaseAddServer(v4[3], [4]byte{192, 0, 2, 68})
					}
				}
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(sp, dp))
				require.NoError(t, err)
				defer s.Close("cleanup")
				q := s.Feed(0, time.Unix(100, 0), req)
				require.Nil(t, q.Err)
				require.Len(t, q.Events, 1)
				r := s.Feed(1, time.Unix(101, 0), bad)
				require.Nil(t, r.Err)
				require.Len(t, r.Events, 1)
				if wrong {
					require.Zero(t, r.Events[0].ResponseTo, "same client/xid is insufficient when selected server identifier disagrees")
					require.Zero(t, r.Events[0].TransactionID)
					next := s.Feed(1, time.Unix(102, 0), reply)
					require.Nil(t, next.Err)
					require.Len(t, next.Events, 1)
					require.Equal(t, q.Events[0].ID, next.Events[0].ResponseTo)
				} else {
					require.Equal(t, q.Events[0].ID, r.Events[0].ResponseTo)
					require.Equal(t, q.Events[0].ID, r.Events[0].TransactionID)
				}
				require.Empty(t, s.Close("done"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}

func TestDHCPPhaseDORAResponseOwners(t *testing.T) {
	_, w := dhcpOwnedInputs(t)
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(68, 67))
	require.NoError(t, err)
	defer s.Close("cleanup")
	var ev []*ProtocolEvent
	for i, raw := range w {
		o := s.Feed(i%2, time.Unix(int64(100+i), 0), raw)
		require.Nil(t, o.Err)
		require.Len(t, o.Events, 1)
		e := o.Events[0]
		ev = append(ev, e)
		f, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, dhcpOwnedFields(i), f)
		require.Equal(t, dhcpOwnedFields(i), e.Session)
		require.Equal(t, raw, e.Raw)
		require.Equal(t, len(raw), e.Length)
		require.Equal(t, ev[0].ID, e.TransactionID)
	}
	require.Equal(t, ev[0].ID, ev[1].ResponseTo)
	require.Equal(t, ev[2].ID, ev[3].ResponseTo, "RFC2131 Table3/4 ACK answers the observed Request phase, while TransactionID retains the discovery transaction root")
	require.Empty(t, s.Close("done"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}
func TestDHCPPhaseRetryAge(t *testing.T) {
	_, v4 := dhcpOwnedInputs(t)
	_, v6 := dhcp6OwnedInputs(t)
	for _, six := range []bool{false, true} {
		t.Run(fmt.Sprintf("v6%t", six), func(t *testing.T) {
			req, reply := v4[2], v4[3]
			sp, dp := uint16(68), uint16(67)
			if six {
				req, reply = v6[2], v6[3]
				sp, dp = 546, 547
			}
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(sp, dp))
			require.NoError(t, err)
			defer s.Close("cleanup")
			first := s.Feed(0, time.Unix(100, 0), req)
			require.Nil(t, first.Err)
			require.Len(t, first.Events, 1)
			retry := s.Feed(0, time.Unix(129, 0), req)
			require.Nil(t, retry.Err)
			require.Len(t, retry.Events, 1)
			require.Equal(t, first.Events[0].ID, retry.Events[0].TransactionID)
			out := s.Feed(1, time.Unix(131, 0), reply)
			require.Nil(t, out.Err)
			require.Len(t, out.Events, 1)
			require.Equal(t, first.Events[0].ID, out.Events[0].ResponseTo, "exact live retry should renew analyzer idle without changing the first request owner")
			want := dhcpOwnedFields(3)
			if six {
				want = dhcp6OwnedFields(t, 3)
			}
			fields, err := out.Events[0].GetFields()
			require.NoError(t, err)
			require.Equal(t, want, fields)
			require.Equal(t, want, out.Events[0].Session)
			require.Empty(t, s.Close("done"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDHCPPhaseSingletonAndWidth(t *testing.T) {
	_, w := dhcp6OwnedInputs(t)
	for _, kind := range []string{"client-duplicate", "server-duplicate", "elapsed-width", "rapid-width"} {
		t.Run(kind, func(t *testing.T) {
			wire := bytes.Clone(w[2])
			switch kind {
			case "client-duplicate":
				wire = append(wire, w[2][4:22]...)
			case "server-duplicate":
				wire = append(wire, w[2][22:40]...)
			case "elapsed-width":
				wire = dhcpPhaseReplace6Option(t, wire, 8, []byte{0, 0, 0})
			case "rapid-width":
				wire = append(wire, 0, 14, 0, 1, 0)
			}
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(546, 547))
			require.NoError(t, err)
			defer s.Close("cleanup")
			out := s.Feed(0, time.Unix(100, 0), wire)
			rocTypedError(t, "MalformedMessage", out.Err)
			require.Len(t, out.Events, 1)
			e := out.Events[0]
			require.Zero(t, e.TransactionID)
			require.Zero(t, e.ResponseTo)
			f, err := e.GetFields()
			rocTypedError(t, "MalformedMessage", err)
			require.Nil(t, f)
			require.Equal(t, wire, e.Raw)
			require.Empty(t, s.Close("done"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestDHCPPhaseRapidCommit(t *testing.T) {
	_, w := dhcp6OwnedInputs(t)
	for _, rapid := range []bool{false, true} {
		t.Run(fmt.Sprintf("rapid%t", rapid), func(t *testing.T) {
			q, r := bytes.Clone(w[0]), bytes.Clone(w[1])
			r[0] = 7
			qf, rf := dhcp6OwnedFields(t, 0), dhcp6OwnedFields(t, 1)
			rf["Message Type"] = byte(7)
			if rapid {
				q = append(q, 0, 14, 0, 0)
				r = append(r, 0, 14, 0, 0)
				qf["Options"] = append(qf["Options"].([]map[string]any), map[string]any{"Code": uint16(14), "Raw": []byte{}})
				rf["Options"] = append(rf["Options"].([]map[string]any), map[string]any{"Code": uint16(14), "Raw": []byte{}})
			}
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(546, 547))
			require.NoError(t, err)
			defer s.Close("cleanup")
			first := s.Feed(0, time.Unix(100, 0), q)
			require.Nil(t, first.Err)
			require.Len(t, first.Events, 1)
			f, err := first.Events[0].GetFields()
			require.NoError(t, err)
			require.Equal(t, qf, f)
			out := s.Feed(1, time.Unix(101, 0), r)
			require.Nil(t, out.Err)
			require.Len(t, out.Events, 1)
			e := out.Events[0]
			if rapid {
				require.Equal(t, first.Events[0].ID, e.ResponseTo)
				require.Equal(t, first.Events[0].ID, e.TransactionID)
			} else {
				require.Zero(t, e.ResponseTo, "Solicit/Reply needs explicitly observed Rapid Commit on both messages")
				require.Zero(t, e.TransactionID)
				rf["Association"] = "missing-request-phase"
			}
			f, err = e.GetFields()
			require.NoError(t, err)
			require.Equal(t, rf, f)
			require.Equal(t, rf, e.Session)
			require.Equal(t, r, e.Raw)
			require.Empty(t, s.Close("done"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDHCPPhaseForeignServerClock(t *testing.T) {
	_, v4 := dhcpOwnedInputs(t)
	_, v6 := dhcp6OwnedInputs(t)
	for _, six := range []bool{false, true} {
		t.Run(fmt.Sprintf("v6%t", six), func(t *testing.T) {
			req, reply := dhcpPhaseAddServer(v4[2], [4]byte{192, 0, 2, 67}), dhcpPhaseAddServer(v4[3], [4]byte{192, 0, 2, 67})
			bad := dhcpPhaseAddServer(v4[3], [4]byte{192, 0, 2, 68})
			sp, dp := uint16(68), uint16(67)
			if six {
				req, reply = v6[2], v6[3]
				bad = dhcpPhaseChange6Option(t, reply, 2)
				sp, dp = 546, 547
			}
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(sp, dp))
			require.NoError(t, err)
			defer s.Close("cleanup")
			q := s.Feed(0, time.Unix(100, 0), req)
			require.Nil(t, q.Err)
			require.Len(t, q.Events, 1)
			r := s.Feed(1, time.Unix(1000, 0), bad)
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			require.Zero(t, r.Events[0].ResponseTo)
			good := s.Feed(1, time.Unix(101, 0), reply)
			require.Nil(t, good.Err)
			require.Len(t, good.Events, 1)
			require.Equal(t, q.Events[0].ID, good.Events[0].ResponseTo, "a mismatched selected server cannot advance the valid owner's idle clock")
			require.Empty(t, s.Close("done"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestDHCPPhaseLateOfferKeepsRequest(t *testing.T) {
	_, w := dhcpOwnedInputs(t)
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(68, 67))
	require.NoError(t, err)
	defer s.Close("cleanup")
	var ev []*ProtocolEvent
	for _, step := range []struct {
		dir int
		ts  int64
		raw []byte
	}{{0, 100, w[0]}, {0, 129, w[2]}, {1, 131, w[1]}, {1, 131, w[3]}} {
		out := s.Feed(step.dir, time.Unix(step.ts, 0), step.raw)
		require.Nil(t, out.Err)
		require.Len(t, out.Events, 1)
		ev = append(ev, out.Events[0])
	}
	require.Zero(t, ev[2].ResponseTo, "an expired discovery reply has no live owner")
	require.Equal(t, ev[1].ID, ev[3].ResponseTo, "the expired Offer phase cannot erase the active Request phase")
	require.Equal(t, ev[0].ID, ev[3].TransactionID)
	require.Empty(t, s.Close("done"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func dhcpPhaseReplace6Option(t *testing.T, w []byte, code uint16, value []byte) []byte {
	t.Helper()
	for p := 4; p+4 <= len(w); {
		c, n := binary.BigEndian.Uint16(w[p:]), int(binary.BigEndian.Uint16(w[p+2:]))
		require.LessOrEqual(t, n, len(w)-p-4)
		if c == code {
			out := bytes.Clone(w[:p])
			out = binary.BigEndian.AppendUint16(out, code)
			out = binary.BigEndian.AppendUint16(out, uint16(len(value)))
			out = append(out, value...)
			return append(out, w[p+4+n:]...)
		}
		p += 4 + n
	}
	t.Fatal("fixed reference lacks option")
	return nil
}
