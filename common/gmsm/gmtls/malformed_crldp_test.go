package gmtls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"net"
	"testing"
	"time"

	gmx509 "github.com/yaklang/yaklang/common/gmsm/x509"
)

func TestClientHandshakeWithMalformedNonCriticalCRLDistributionPoints(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}

	template := &gmx509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "malformed-crldp.example"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtraExtensions: []pkix.Extension{{
			Id:    asn1.ObjectIdentifier{2, 5, 29, 31},
			Value: []byte{0x30, 0x03, 0x30},
		}},
	}
	der, err := gmx509.CreateCertificate(template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}

	serverNetConn, clientNetConn := net.Pipe()
	defer serverNetConn.Close()
	defer clientNetConn.Close()
	deadline := time.Now().Add(5 * time.Second)
	if err := serverNetConn.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	if err := clientNetConn.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}

	server := Server(serverNetConn, &Config{
		MaxVersion: VersionTLS12,
		Certificates: []Certificate{{
			Certificate: [][]byte{der},
			PrivateKey:  privateKey,
		}},
	})
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Handshake()
	}()

	client := Client(clientNetConn, &Config{
		MaxVersion:         VersionTLS12,
		InsecureSkipVerify: true,
	})
	if err := client.Handshake(); err != nil {
		t.Fatalf("client handshake rejected a certificate with a malformed non-critical CRL distribution points extension: %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server handshake failed: %v", err)
	}
}
