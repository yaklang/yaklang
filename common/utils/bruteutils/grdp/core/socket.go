package core

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"net"

	"github.com/yaklang/yaklang/common/gmsm/gmtls"
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
	// Cap at TLS 1.2: Windows CredSSP silently fails over TLS 1.3
	// (handshake succeeds, pubKeyAuth verification is dropped).
	// Keep TLS 1.0 as floor so Windows 7 / Server 2008 still negotiate.
	config := &gmtls.Config{
		InsecureSkipVerify:       true,
		MinVersion:               gmtls.VersionTLS10,
		MaxVersion:               gmtls.VersionTLS12,
		PreferServerCipherSuites: true,
	}
	s.tlsConn = gmtls.Client(s.conn, config)
	return s.tlsConn.Handshake()
}

// TlsPubKey extracts the server's public key in the encoding CredSSP uses
// for pubKeyAuth (MS-CSSP SubjectPublicKey): PKCS#1 for RSA, uncompressed
// EC point for ECDSA, raw 32-byte key for Ed25519.
func (s *SocketLayer) TlsPubKey() ([]byte, error) {
	if s.tlsConn == nil {
		return nil, errors.New("TLS conn does not exist")
	}
	certs := s.tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, errors.New("no peer certificates")
	}
	switch pub := certs[0].PublicKey.(type) {
	case *rsa.PublicKey:
		return x509.MarshalPKCS1PublicKey(pub), nil
	case *ecdsa.PublicKey:
		ecdhKey, err := pub.ECDH()
		if err != nil {
			return nil, fmt.Errorf("ECDSA to ECDH conversion: %w", err)
		}
		return ecdhKey.Bytes(), nil
	case ed25519.PublicKey:
		return []byte(pub), nil
	default:
		return nil, fmt.Errorf("unsupported public key type: %T", pub)
	}
}
