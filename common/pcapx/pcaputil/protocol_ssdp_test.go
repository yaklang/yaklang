package pcaputil

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestNativeSSDPDatagramsDoNotUsePortStickiness(t *testing.T) {
	a, _ := remainingParser(t)
	w := []byte("NOTIFY * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nNT: upnp:rootdevice\r\nNTS: ssdp:alive\r\nUSN: uuid:device-a::upnp:rootdevice\r\nCACHE-CONTROL: max-age=66\r\nLOCATION: http://192.0.2.10/description.xml\r\nSERVER: Windows UPnP/1.0\r\n\r\n")
	e := &ProtocolEvent{Source: "192.0.2.10:15978", Destination: "239.255.255.250:1900"}
	require.True(t, a.decodeNativeDatagram(e, w, 15978, 1900))
	require.Equal(t, "ssdp", e.Protocol)
	require.Equal(t, "decoded", e.Status, e.Error)
	require.Zero(t, e.FlowID)
	extra := &ProtocolEvent{}
	require.True(t, a.decodeNativeDatagram(extra, append(append([]byte(nil), w...), '\r', '\n'), 15978, 1900))
	require.Equal(t, "decoded", extra.Status, extra.Error)
	require.NotEmpty(t, extra.Session["Conformance Warnings"])
	f, err := e.GetFields()
	require.NoError(t, err)
	require.Equal(t, uint64(66), f["Maximum Age Seconds"])
	require.False(t, a.decodeNativeDatagram(&ProtocolEvent{}, []byte("unrelated bytes"), 15978, 1900))
	for _, bad := range [][]byte{w[:len(w)-2], []byte(strings.Replace(string(w), "NTS: ssdp:alive", "NTS: ssdp:no", 1))} {
		e = &ProtocolEvent{}
		require.True(t, a.decodeNativeDatagram(e, bad, 15978, 1900))
		require.NotEqual(t, "decoded", e.Status)
	}
	// The complete wire grammar works on a nonstandard port too.
	e = &ProtocolEvent{}
	require.True(t, a.decodeNativeDatagram(e, w, 15978, 49000))
	require.Equal(t, "decoded", e.Status)
	require.Zero(t, a.stats().BufferedBytes)
}
