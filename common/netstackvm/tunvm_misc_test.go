package netstackvm

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
)

func TestTCPConnOriginalDestinationUsesInterceptedEndpoint(t *testing.T) {
	conn := &tcpConn{id: stack.TransportEndpointID{
		LocalAddress: tcpip.AddrFrom4([4]byte{203, 0, 113, 9}),
		LocalPort:    443,
	}}

	target, ok := conn.OriginalDestination().(*net.TCPAddr)
	require.True(t, ok)
	require.Equal(t, "203.0.113.9", target.IP.String())
	require.Equal(t, 443, target.Port)
}
