package core

import (
	"crypto/rsa"
	"math/big"

	"github.com/huin/asn1ber"

	"errors"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"net"
)

type SocketLayer struct {
	conn    net.Conn
	tlsConn *gmtls.Conn
}

func NewSocketLayer(conn net.Conn) *SocketLayer {
	l := &SocketLayer{
		conn:    conn,
		tlsConn: nil,
	}
	return l
}

func (s *SocketLayer) Read(b []byte) (n int, err error) {
	if s.tlsConn != nil {
		return s.tlsConn.Read(b)
	}
	return s.conn.Read(b)
}

func (s *SocketLayer) Write(b []byte) (n int, err error) {
	if s.tlsConn != nil {
		return s.tlsConn.Write(b)
	}
	return s.conn.Write(b)
}

func (s *SocketLayer) Close() error {
	if s.tlsConn != nil {
		err := s.tlsConn.Close()
		if err != nil {
			return err
		}
	}
	return s.conn.Close()
}

func (s *SocketLayer) StartTLS() error {
	config := &gmtls.Config{
		InsecureSkipVerify:       true,
		MinVersion:               gmtls.VersionTLS10,
		MaxVersion:               gmtls.VersionTLS13,
		PreferServerCipherSuites: true,
	}
	s.tlsConn = gmtls.Client(s.conn, config)
	return s.tlsConn.Handshake()
}

type PublicKey struct {
	N *big.Int `asn1:"explicit,tag:0"` // modulus
	E int      `asn1:"explicit,tag:1"` // public exponent
}

func (s *SocketLayer) TlsPubKey() ([]byte, error) {
	if s.tlsConn == nil {
		return nil, errors.New("TLS conn does not exist")
	}
	pub := s.tlsConn.ConnectionState().PeerCertificates[0].PublicKey.(*rsa.PublicKey)
	return asn1ber.Marshal(*pub)
}
