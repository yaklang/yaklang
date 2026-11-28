package extrafp

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRDPVersionContextReleasesConnection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		canceled bool
		response []byte
	}{
		{name: "peer_finished"},
		{name: "caller_canceled", canceled: true},
		{name: "recognized_response", response: os___xrdp_2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
			require.NoError(t, err)
			defer listener.Close()
			require.NoError(t, listener.SetDeadline(time.Now().Add(2*time.Second)))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			type result struct {
				version string
				err     error
			}
			done := make(chan result, 1)
			go func() {
				version, _, err := RDPVersionContext(ctx, listener.Addr().String(), 5*time.Second)
				done <- result{version: version, err: err}
			}()
			peer, err := listener.AcceptTCP()
			require.NoError(t, err)
			defer peer.Close()
			require.NoError(t, peer.SetReadDeadline(time.Now().Add(time.Second)))
			var firstByte [1]byte
			_, err = peer.Read(firstByte[:])
			require.NoError(t, err)
			if tc.canceled {
				cancel()
			} else {
				if len(tc.response) > 0 {
					_, err := peer.Write(tc.response)
					require.NoError(t, err)
				}
				require.NoError(t, peer.CloseWrite())
			}
			select {
			case got := <-done:
				switch {
				case tc.canceled:
					require.ErrorIs(t, got.err, context.Canceled)
				case len(tc.response) > 0:
					require.NoError(t, got.err)
					require.Equal(t, "xrdp", got.version)
				default:
					require.Error(t, got.err)
				}
			case <-time.After(time.Second):
				t.Fatal("supplemental identification kept waiting on the connection")
			}
			require.NoError(t, peer.SetReadDeadline(time.Now().Add(time.Second)))
			_, err = io.Copy(io.Discard, peer)
			require.NoError(t, err)
		})
	}
}
