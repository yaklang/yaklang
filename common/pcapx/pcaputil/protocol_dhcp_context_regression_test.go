package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDHCPContextProjection(t *testing.T) {
	for _, v6 := range []bool{false, true} {
		t.Run(map[bool]string{false: "v4", true: "v6"}[v6], func(t *testing.T) {
			var raw []byte
			protocol := "dhcp"
			if v6 {
				raw, _ = dhcp6OwnedInputs(t)
				protocol = "dhcpv6"
			} else {
				raw, _ = dhcpOwnedInputs(t)
			}
			var events []*ProtocolEvent
			require.NoError(t, ReplayPcap(bytes.NewReader(raw), WithOnProtocolMessage(func(e *ProtocolEvent) {
				if e.Protocol == protocol {
					events = append(events, e)
				}
			})))
			count := 4
			if v6 {
				count = 6
			}
			require.Len(t, events, count)
			for i, e := range events {
				f, err := e.GetFields()
				require.NoError(t, err)
				if v6 {
					require.Equal(t, dhcp6OwnedFields(t, i), f)
				} else {
					require.Equal(t, dhcpOwnedFields(i), f)
				}
				before, err := e.Decode()
				require.NoError(t, err)
				want, err := json.Marshal(before)
				require.NoError(t, err)
				tlsDTLSPoison(e, "http")
				after, err := e.Decode()
				require.NoError(t, err)
				got, err := json.Marshal(after)
				require.NoError(t, err)
				require.JSONEq(t, string(want), string(got))
			}
		})
	}
}

func TestDHCPContextRefusal(t *testing.T) {
	_, v4 := dhcpOwnedInputs(t)
	_, v6w := dhcp6OwnedInputs(t)
	for _, v6 := range []bool{false, true} {
		for _, limited := range []bool{false, true} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("v6%t/limit%t/d%t", v6, limited, deferred), func(t *testing.T) {
					wire := bytes.Clone(v4[0])
					port := uint16(67)
					if v6 {
						wire = bytes.Clone(v6w[0])
						port = 547
					}
					budget := DefaultParserBudget()
					kind := "MalformedMessage"
					if limited {
						budget.MaxCollectionElements = 1
						kind = "ResourceExceeded"
						if !v6 {
							wire = append(wire[:len(wire)-1], 12, 1, 'x', 255)
						}
					} else {
						wire = wire[:len(wire)-1]
					}
					s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("udp"), WithSessionPorts(map[bool]uint16{false: 68, true: 546}[v6], port))
					require.NoError(t, err)
					s.(*captureSession).f.a.config.Deferred = deferred
					o := s.Feed(0, time.Unix(1, 0), wire)
					rocTypedError(t, kind, o.Err)
					require.Len(t, o.Events, 1)
					e := o.Events[0]
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
					if v6 {
						require.Equal(t, map[string]any{"Message Type": byte(1), "Observation": "observed", "Transaction ID": uint32(0x100874), "Options": []map[string]any(nil)}, e.Session)
					} else {
						require.Nil(t, e.Session)
					}
					require.Equal(t, wire, e.Raw)
					require.Equal(t, len(wire), e.Length)
					tlsDTLSPoison(e, "http")
					f, err := e.GetFields()
					rocTypedError(t, kind, err)
					require.Nil(t, f)
					f, err = e.Decode()
					rocTypedError(t, kind, err)
					require.Nil(t, f)
					require.Empty(t, s.Close("complete"))
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}
