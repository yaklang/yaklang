package mitm

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// certTestConn exposes only LocalAddr. Certificate hostname resolution must
// never read or write the handshake connection, so a stub is enough and the
// test stays hermetic (no dialing, no real addresses).
type certTestConn struct {
	net.Conn
	localAddr string
}

func (c certTestConn) LocalAddr() net.Addr { return certTestAddr(c.localAddr) }

type certTestAddr string

func (a certTestAddr) Network() string { return "tcp" }
func (a certTestAddr) String() string  { return string(a) }

// TestCertHostnameForHandshake pins the resolution order used to mint the
// client-facing leaf certificate:
//
//	SNI > upstream target > accepted connection LocalAddr > error
//
// The upstream target is what makes https://<ip>:<port> work: an IP literal
// gets no SNI (RFC 6066 section 3), so the CONNECT authority is the only name
// the client can verify against.
func TestCertHostnameForHandshake(t *testing.T) {
	conn := certTestConn{localAddr: "127.0.0.9:8443"}

	tests := []struct {
		name       string
		serverName string
		upstream   string
		conn       net.Conn
		want       string
		wantErr    string
	}{
		{
			name:       "SNI wins over upstream target",
			serverName: "sni.cert-hostname.invalid",
			upstream:   "127.0.0.2",
			conn:       conn,
			want:       "sni.cert-hostname.invalid",
		},
		{
			name:     "no SNI falls back to upstream target and drops the port",
			upstream: "127.0.0.2:1000",
			conn:     conn,
			want:     "127.0.0.2",
		},
		{
			name: "no SNI and no upstream target falls back to LocalAddr",
			conn: conn,
			want: "127.0.0.9",
		},
		{
			name:    "nothing to mint a certificate for is an error",
			wantErr: "SNI not provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := certHostnameForHandshake(tt.serverName, tt.upstream, tt.conn)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
