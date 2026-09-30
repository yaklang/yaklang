package yakgrpc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestGRPCMUSTPASS_MITMV2_SNI_DomainNotIP tests the SNI fix for Proxifier/SOCKS5 scenario
// This test verifies that when connecting to an IP address, the SNI is still set to the domain name
func TestGRPCMUSTPASS_MITMV2_SNI_DomainNotIP(t *testing.T) {
	testDomain := "api.test.example.com"
	sniReceived := make(chan string, 1)

	// Create self-signed certificate for test domain
	cert, key := generateTestCertificate(t, testDomain)
	tlsCert, err := tls.X509KeyPair(cert, key)
	require.NoError(t, err)

	// Start TLS server that captures SNI
	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer tcpListener.Close()

	serverAddr := tcpListener.Addr().String()
	serverIP, serverPort, err := utils.ParseStringToHostPort(serverAddr)
	require.NoError(t, err)

	log.Infof("TLS mock server (with SNI capture) started on %s", serverAddr)

	// Server goroutine - captures SNI from ClientHello
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				conn, err := tcpListener.Accept()
				if err != nil {
					return
				}

				go func(rawConn net.Conn) {
					defer rawConn.Close()

					tlsConfig := &tls.Config{
						Certificates: []tls.Certificate{tlsCert},
						GetConfigForClient: func(info *tls.ClientHelloInfo) (*tls.Config, error) {
							log.Infof("✅ Server captured SNI from ClientHello: %s", info.ServerName)
							select {
							case sniReceived <- info.ServerName:
							default:
							}
							return &tls.Config{
								Certificates: []tls.Certificate{tlsCert},
							}, nil
						},
					}

					tlsConn := tls.Server(rawConn, tlsConfig)
					_ = tlsConn.Handshake()

					// Send a simple response
					tlsConn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"))
				}(conn)
			}
		}
	}()

	// Start MITM server
	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	require.NoError(t, err)

	stream, err := client.MITMV2(ctx)
	require.NoError(t, err)

	err = stream.Send(&ypb.MITMV2Request{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})
	require.NoError(t, err)

	mitmStarted := false

	// The bound-listener event starts the request; completion joins the worker.
	requestDone := make(chan error, 1)
	go func() {
		defer cancel()
		for {
			data, err := stream.Recv()
			if err != nil {
				break
			}
			if data.GetMessage().GetIsMessage() {
				msg := string(data.GetMessage().GetMessage())
				log.Info(msg)
				if strings.Contains(msg, "starting mitm server") && !mitmStarted {
					mitmStarted = true

					// CRITICAL TEST: Send HTTPS request through MITM
					// We connect to IP but use domain in Host header
					// This simulates Proxifier/SOCKS5 resolving DNS and giving us IP
					log.Infof("CRITICAL TEST: Sending HTTPS request to IP %s with Host: %s", serverIP, testDomain)

					// Use poc.Do to send request through MITM proxy
					_, err := yak.Execute(
						`
// CRITICAL: Connect to IP address but set Host header to domain
// This simulates the Proxifier/SOCKS5 scenario where DNS is already resolved
target = f"https://${serverIP}:${serverPort}/"

log.info("Sending HTTPS request through MITM proxy")
log.info(f"Target: ${target}, Host header: ${testDomain}")

// Send request through MITM with domain Host header (simulates Proxifier scenario)
rsp, req = poc.Get(
	target,
	poc.proxy(mitmProxy),
	poc.replaceHeader("Host", testDomain),
	poc.timeout(5),
)~

assert str.Contains(rsp.RawPacket, "200 OK")
`,
						map[string]any{
							"serverIP":   serverIP,
							"serverPort": serverPort,
							"testDomain": testDomain,
							"mitmProxy":  fmt.Sprintf("http://127.0.0.1:%d", mitmPort),
						})

					requestDone <- err
					return
				}
			}
		}
	}()

	select {
	case err := <-requestDone:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("MITM request did not complete")
	}
	var receivedSNI string
	select {
	case receivedSNI = <-sniReceived:
	default:
		t.Fatal("TLS server did not capture a ClientHello")
	}

	// CRITICAL ASSERTIONS
	require.True(t, mitmStarted, "MITM server should have started")
	require.NotEmpty(t, receivedSNI, "Server should have captured SNI from ClientHello")

	// MAIN TEST: Verify SNI is the DOMAIN, not the IP
	require.Equal(t, testDomain, receivedSNI,
		"FAIL: Server received wrong SNI! Expected domain %s, got %s", testDomain, receivedSNI)

	// Double check SNI is not an IP address
	isIP := net.ParseIP(receivedSNI) != nil
	require.False(t, isIP,
		"FAIL: SNI must be domain, not IP! Got: %s", receivedSNI)

	require.NotEqual(t, serverIP, receivedSNI,
		"FAIL: SNI should be domain %s, not the IP %s", testDomain, serverIP)

	log.Infof("✅✅✅ SUCCESS: Complete E2E Test Passed!")
	log.Infof("  Connected to IP: %s", serverIP)
	log.Infof("  Host header: %s", testDomain)
	log.Infof("  TLS SNI received by server: %s (DOMAIN, not IP!)", receivedSNI)
	log.Infof("  This proves the SNI fix is working correctly through MITM proxy!")
}

// generateTestCertificate creates a self-signed certificate for testing
func generateTestCertificate(t *testing.T, domain string) (certPEM, keyPEM []byte) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Yaklang Test"},
			CommonName:   domain,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{domain},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	return certPEM, keyPEM
}
