package pcaputil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDLMSUDPByteRefusalRetiresPending(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, denied := range []bool{false, true} {
			b := DefaultParserBudget()
			b.MaxFrameBytes = 64
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(39200, 4059))
			require.NoError(t, err)
			s.(*captureSession).f.a.config.MaxMessageBytes = 64
			s.(*captureSession).f.a.config.Deferred = deferred
			q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "7ea0190321107fdae6e600c001c1000f0000280000ff020091537e"))
			require.Nil(t, q.Err)
			require.Len(t, q.Events, 1)
			if denied {
				out := s.Feed(1, time.Unix(2, 0), wrapperWire(t, "7ea0622103302dc7e6e700c401c100095041414141414141414141414141414141414141414141414141414141414141414141414141414141414141414141414141414141414141414141414141414141414141414141414141414141414141415fe27e"))
				require.Len(t, out.Events, 1)
				fields, err := out.Events[0].GetFields()
				rocTypedError(t, "ResourceExceeded", err)
				require.Nil(t, fields)
				require.Nil(t, out.Events[0].Session)
				require.Zero(t, out.Events[0].ResponseTo)
				require.Zero(t, out.Events[0].TransactionID)
			}
			r := s.Feed(1, time.Unix(3, 0), wrapperWire(t, "7ea012210330689de6e700c401c100112a43c37e"))
			require.Len(t, r.Events, 1)
			fields, err := r.Events[0].GetFields()
			if denied {
				rocTypedError(t, "ContextRequired", err)
				require.Nil(t, fields)
				require.Nil(t, r.Events[0].Session)
				require.Zero(t, r.Events[0].ResponseTo)
			} else {
				require.NoError(t, err)
				require.EqualValues(t, 42, fields["Data Value"])
				require.Equal(t, q.Events[0].ID, r.Events[0].ResponseTo)
			}
			require.Zero(t, r.Events[0].TransactionID)
			require.Empty(t, s.Close("byte-refusal"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		}
	}
}

func TestDLMSFullInvokeFlagsExistingAPI(t *testing.T) {
	for _, transport := range []string{"udp", "tcp"} {
		for _, deferred := range []bool{false, true} {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(39200, 4059))
			require.NoError(t, err)
			s.(*captureSession).f.a.config.Deferred = deferred
			q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "7ea0190321107fdae6e600c001c1000f0000280000ff020091537e"))
			require.Nil(t, q.Err)
			require.Len(t, q.Events, 1)
			wrong := s.Feed(1, time.Unix(2, 0), wrapperWire(t, "7ea012210330689de6e700c4018100112af4d57e"))
			require.Len(t, wrong.Events, 1)
			fields, err := wrong.Events[0].GetFields()
			rocTypedError(t, "ContextRequired", err)
			require.Nil(t, fields)
			require.Nil(t, wrong.Events[0].Session)
			require.Zero(t, wrong.Events[0].ResponseTo)
			require.Zero(t, wrong.Events[0].TransactionID)
			adjacent := s.Feed(1, time.Unix(3, 0), wrapperWire(t, "7ea012210330689de6e700c401c100112a43c37e"))
			require.Nil(t, adjacent.Err)
			require.Len(t, adjacent.Events, 1)
			require.Equal(t, q.Events[0].ID, adjacent.Events[0].ResponseTo)
			require.EqualValues(t, 193, adjacent.Events[0].Session["Invoke ID and Priority"])
			require.EqualValues(t, 42, adjacent.Events[0].Session["Data Value"])
			require.Empty(t, s.Close("contract"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		}
	}
}

func TestDLMSHDLCListExistingAPIBaseline(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(39200, 4059))
	require.NoError(t, err)
	q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "7ea01a032110b2ffe6e600c003c101000f0000280000ff02006a8c7e"))
	require.Nil(t, q.Err)
	require.Len(t, q.Events, 1)
	out := s.Feed(1, time.Unix(2, 0), wrapperWire(t, "7ea013210330d381e6e700c403c10100112ad2977e"))
	require.Nil(t, out.Err)
	require.Len(t, out.Events, 1)
	fields, err := out.Events[0].GetFields()
	require.NoError(t, err)
	require.Equal(t, "Get Response With List", fields["Frame Kind"])
	require.Equal(t, q.Events[0].ID, out.Events[0].ResponseTo)
	get := fields["Get List"].(map[string]any)
	require.EqualValues(t, 1, get["list_count"])
	require.EqualValues(t, 1, get["data_item_count"])
	require.EqualValues(t, 0, get["error_item_count"])
	results := get["results"].([]map[string]any)
	require.Len(t, results, 1)
	require.EqualValues(t, 42, results[0]["data"].(map[string]any)["value"])
	require.Empty(t, s.Close("list"))
	require.Zero(t, s.Stats().BufferedBytes)
}
