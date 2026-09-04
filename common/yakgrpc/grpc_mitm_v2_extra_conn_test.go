//go:build !yakit_exclude

package yakgrpc

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

type mitmV2TransparentTestConn struct {
	net.Conn
	destination net.Addr
}

func (c *mitmV2TransparentTestConn) OriginalDestination() net.Addr {
	return c.destination
}

// Extra-connection behavior contract:
//
//   - A generic injected net.Conn is a normal proxy frontend. Its LocalAddr is
//     never inferred to be an upstream destination, so HTTP and SOCKS5 remain
//     available to custom injectors and multi-listener-style producers.
//   - Only a connection that explicitly implements OriginalDestination is a
//     transparent/TUN input. MITMv2 copies that target into WrapperedConn so
//     minimartian can skip frontend SOCKS5 detection and transparently forward
//     unknown payloads.
//   - An explicitly transparent connection with no valid target is rejected;
//     it must not silently downgrade to a generic proxy frontend.

func TestWrapMITMV2ExtraIncomingConnGenericRemainsProxyFrontend(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	wrapped, err := wrapMITMV2ExtraIncomingConn(serverConn, "127.0.0.1")
	require.NoError(t, err)
	require.Empty(t, wrapped.GetOriginalDestination(), "a generic extra conn must not infer its LocalAddr as an upstream target")
	require.True(t, wrapped.IsStrongHostMode())
	require.Equal(t, "127.0.0.1", wrapped.GetStrongHostLocalAddr())
}

func TestWrapMITMV2ExtraIncomingConnTransparentCarriesTarget(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	target := &net.TCPAddr{IP: net.ParseIP("203.0.113.9"), Port: 443}
	transparentConn := &mitmV2TransparentTestConn{Conn: serverConn, destination: target}
	wrapped, err := wrapMITMV2ExtraIncomingConn(transparentConn, "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, target.String(), wrapped.GetOriginalDestination())
	require.True(t, wrapped.IsStrongHostMode())
}

func TestWrapMITMV2ExtraIncomingConnRejectsMissingTransparentTarget(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	transparentConn := &mitmV2TransparentTestConn{Conn: serverConn}
	wrapped, err := wrapMITMV2ExtraIncomingConn(transparentConn, "127.0.0.1")
	require.Error(t, err)
	require.Nil(t, wrapped)
}
