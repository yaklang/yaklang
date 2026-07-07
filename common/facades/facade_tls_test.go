package facades

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

func TestFacadeServerRejects3DESCipherSuites(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := NewFacadeServer("127.0.0.1", 0)
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ServeWithContext(ctx)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for server.Port == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if server.Port == 0 {
		t.Fatal("facade server did not start")
	}

	t.Cleanup(func() {
		cancel()
		select {
		case <-errCh:
		case <-time.After(time.Second):
		}
	})

	addr := utils.HostPort("127.0.0.1", server.Port)
	conn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
		MaxVersion:         tls.VersionTLS12,
		CipherSuites:       []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
	})
	if err != nil {
		t.Fatalf("facade rejected modern TLS cipher suite: %v", err)
	}
	_ = conn.Close()

	for _, suite := range []uint16{
		tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
	} {
		conn, err := tls.Dial("tcp", addr, &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS12,
			MaxVersion:         tls.VersionTLS12,
			CipherSuites:       []uint16{suite},
		})
		if err == nil {
			_ = conn.Close()
			t.Fatalf("facade accepted 3DES cipher suite 0x%04x", suite)
		}
	}
}
