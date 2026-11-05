package pcaputil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzTLSHTTPGapPreservesObservedPrefix(f *testing.F) {
	for _, seed := range [][]byte{{0, 0}, {1, 1, 0xff}, {2, 2, 0}, {3, 3, 1, 2}, {7, 0, 3}, {12, 1, 4}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 2 || len(data) > 258 {
			return
		}
		suites := []uint16{0x009c, 0x1301, 0x1303}
		server := int(data[0]>>3) & 1
		flow, state, events := closeNotifyTestFlow(t, suites[int(data[0])%len(suites)], server, data[0]&4 != 0)
		closeNotifyTestSend(t, flow, state, 1-server, 23, []byte("GET /fuzz HTTP/1.1\r\nHost: observed.invalid\r\n\r\n"), false)
		response := append([]byte("HTTP/1.1 200 OK\r\nConnection: close\r\n\r\n"), data[2:]...)
		closeNotifyTestSend(t, flow, state, server, 23, response, false)
		action := data[1] % 4
		switch action {
		case 0:
			closeNotifyTestSend(t, flow, state, server, 21, []byte{1, 0}, true)
			closeNotifyTestSend(t, flow, state, server, 23, []byte("must not join"), false)
			closeNotifyTestSend(t, flow, state, server, 21, []byte{1, 0}, false)
		case 1:
			flow.markTLSInnerGap(server)
			flow.markTLSInnerGap(server)
		case 2:
			closeNotifyTestSend(t, flow, state, 1-server, 21, []byte{1, 0}, true)
			closeNotifyTestSend(t, flow, state, server, 21, []byte{1, 0}, false)
		case 3:
			closeNotifyTestSend(t, flow, state, server, 21, []byte{1, 0}, false)
		}
		flow.close(TrafficFlowCloseReason_FIN)
		complete, partial := closeNotifyTestHTTP(*events)
		if action < 2 {
			require.Len(t, complete, 1)
			require.Len(t, partial, 1)
			require.Equal(t, response, partial[0].Raw)
			require.Equal(t, true, partial[0].Session["TLS Plaintext Gap"])
			require.Nil(t, partial[0].SourceBytes.CompletedIn)
			require.Zero(t, partial[0].ResponseTo)
			fields, err := partial[0].GetFields()
			require.NoError(t, err)
			require.Equal(t, 200, fields["Status Code"])
			require.NotContains(t, fields, "Body")
		} else {
			require.Len(t, complete, 2)
			require.Empty(t, partial)
			require.Equal(t, response, complete[1].Raw)
		}
		require.Zero(t, flow.a.stats().BufferedBytes)
	})
}
