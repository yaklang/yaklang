package spec

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/schema"
)

func TestNewScanFingerprintResultPreservesServiceObservation(t *testing.T) {
	notBefore := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	notAfter := time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC)
	cert := newScanFingerprintTestCertificate(t, notBefore, notAfter)
	certFingerprint := sha256.Sum256(cert.Raw)

	result, err := NewScanFingerprintResult(&fp.MatchResult{
		Target: "gateway.local",
		Port:   8443,
		State:  fp.OPEN,
		Reason: "syn-ack",
		Fingerprint: &fp.FingerprintInfo{
			IP:             "10.10.10.10",
			Port:           8443,
			Proto:          fp.TCP,
			ServiceName:    "https",
			ProductVerbose: "Yak Gateway",
			Version:        "9.8.7",
			Hostname:       "host.gateway.local",
			DeviceType:     "reverse-proxy",
			CPEs:           []string{"cpe:2.3:a:yak:gateway:9.8.7:*:*:*:*:*:*:*"},
			CPEFromUrls: map[string][]*schema.CPE{
				"https://www.gateway.local/login": nil,
			},
			HttpFlows: []*fp.HTTPFlow{
				{
					StatusCode: 202,
					IsHTTPS:    true,
					RequestHeader: []byte("GET /login HTTP/1.1\r\n" +
						"Host: www.gateway.local\r\n\r\n"),
					ResponseHeader: []byte("HTTP/1.1 202 Accepted\r\n" +
						"Server: YakServer/1.0\r\n" +
						"Content-Type: text/html; charset=utf-8\r\n\r\n"),
					ResponseBody: []byte("<html><head><title>Console Portal</title></head><body>ok</body></html>"),
				},
			},
			CheckedTLS: true,
			TLSInspectResults: []*netx.TLSInspectResult{
				{
					Version:         tls.VersionTLS13,
					CipherSuite:     tls.TLS_AES_128_GCM_SHA256,
					ServerName:      "sni.gateway.local",
					Protocol:        "h2",
					Raw:             cert.Raw,
					RelativeDomains: []string{"api.gateway.local", "www.gateway.local"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewScanFingerprintResult returned error: %v", err)
	}
	if result.Type != ScanResult_Fingerprint {
		t.Fatalf("result type = %q, want %q", result.Type, ScanResult_Fingerprint)
	}

	var got struct {
		Host        string   `json:"host"`
		Port        int      `json:"port"`
		Reason      string   `json:"reason"`
		Product     string   `json:"product"`
		Version     string   `json:"version"`
		Hostname    string   `json:"hostname"`
		DeviceType  string   `json:"device_type"`
		Domains     []string `json:"domains"`
		ServiceName string   `json:"service_name"`
		Title       string   `json:"title"`
		HTTP        *struct {
			URL         string `json:"url"`
			Title       string `json:"title"`
			StatusCode  int    `json:"status_code"`
			IsHTTPS     bool   `json:"is_https"`
			Server      string `json:"server"`
			ContentType string `json:"content_type"`
		} `json:"http"`
		TLS *struct {
			Enabled           bool     `json:"enabled"`
			Checked           bool     `json:"checked"`
			SNI               string   `json:"sni"`
			ServerName        string   `json:"server_name"`
			Protocol          string   `json:"protocol"`
			Version           string   `json:"version"`
			CipherSuite       string   `json:"cipher_suite"`
			Issuer            string   `json:"issuer"`
			Subject           string   `json:"subject"`
			NotBefore         string   `json:"not_before"`
			NotAfter          string   `json:"not_after"`
			DNSNames          []string `json:"dns_names"`
			FingerprintSHA256 string   `json:"fingerprint_sha256"`
		} `json:"tls"`
	}
	if err := json.Unmarshal(result.Content, &got); err != nil {
		t.Fatalf("unmarshal result content: %v", err)
	}

	wantDomains := []string{
		"gateway.local",
		"host.gateway.local",
		"www.gateway.local",
		"sni.gateway.local",
		"cert.gateway.local",
		"api.gateway.local",
	}
	if got.Reason != "syn-ack" {
		t.Fatalf("reason = %q, want %q", got.Reason, "syn-ack")
	}
	if got.Product != "Yak Gateway" {
		t.Fatalf("product = %q, want %q", got.Product, "Yak Gateway")
	}
	if got.Version != "9.8.7" {
		t.Fatalf("version = %q, want %q", got.Version, "9.8.7")
	}
	if got.Hostname != "host.gateway.local" {
		t.Fatalf("hostname = %q, want %q", got.Hostname, "host.gateway.local")
	}
	if got.DeviceType != "reverse-proxy" {
		t.Fatalf("device_type = %q, want %q", got.DeviceType, "reverse-proxy")
	}
	if !reflect.DeepEqual(got.Domains, wantDomains) {
		t.Fatalf("domains = %#v, want %#v", got.Domains, wantDomains)
	}
	if got.HTTP == nil {
		t.Fatal("http summary is nil")
	}
	if got.HTTP.URL != "https://www.gateway.local/login" {
		t.Fatalf("http.url = %q, want %q", got.HTTP.URL, "https://www.gateway.local/login")
	}
	if got.HTTP.Title != "Console Portal" {
		t.Fatalf("http.title = %q, want %q", got.HTTP.Title, "Console Portal")
	}
	if got.HTTP.StatusCode != 202 {
		t.Fatalf("http.status_code = %d, want %d", got.HTTP.StatusCode, 202)
	}
	if !got.HTTP.IsHTTPS {
		t.Fatal("http.is_https = false, want true")
	}
	if got.HTTP.Server != "YakServer/1.0" {
		t.Fatalf("http.server = %q, want %q", got.HTTP.Server, "YakServer/1.0")
	}
	if got.HTTP.ContentType != "text/html; charset=utf-8" {
		t.Fatalf("http.content_type = %q, want %q", got.HTTP.ContentType, "text/html; charset=utf-8")
	}
	if got.TLS == nil {
		t.Fatal("tls summary is nil")
	}
	if !got.TLS.Enabled {
		t.Fatal("tls.enabled = false, want true")
	}
	if !got.TLS.Checked {
		t.Fatal("tls.checked = false, want true")
	}
	if got.TLS.SNI != "sni.gateway.local" {
		t.Fatalf("tls.sni = %q, want %q", got.TLS.SNI, "sni.gateway.local")
	}
	if got.TLS.ServerName != "sni.gateway.local" {
		t.Fatalf("tls.server_name = %q, want %q", got.TLS.ServerName, "sni.gateway.local")
	}
	if got.TLS.Protocol != "h2" {
		t.Fatalf("tls.protocol = %q, want %q", got.TLS.Protocol, "h2")
	}
	if got.TLS.Version != "TLS 1.3" {
		t.Fatalf("tls.version = %q, want %q", got.TLS.Version, "TLS 1.3")
	}
	if got.TLS.CipherSuite != "TLS_AES_128_GCM_SHA256" {
		t.Fatalf("tls.cipher_suite = %q, want %q", got.TLS.CipherSuite, "TLS_AES_128_GCM_SHA256")
	}
	if got.TLS.Issuer != cert.Issuer.String() {
		t.Fatalf("tls.issuer = %q, want %q", got.TLS.Issuer, cert.Issuer.String())
	}
	if got.TLS.Subject != cert.Subject.String() {
		t.Fatalf("tls.subject = %q, want %q", got.TLS.Subject, cert.Subject.String())
	}
	if got.TLS.NotBefore != notBefore.Format(time.RFC3339Nano) {
		t.Fatalf("tls.not_before = %q, want %q", got.TLS.NotBefore, notBefore.Format(time.RFC3339Nano))
	}
	if got.TLS.NotAfter != notAfter.Format(time.RFC3339Nano) {
		t.Fatalf("tls.not_after = %q, want %q", got.TLS.NotAfter, notAfter.Format(time.RFC3339Nano))
	}
	if !reflect.DeepEqual(got.TLS.DNSNames, []string{"cert.gateway.local", "www.gateway.local"}) {
		t.Fatalf("tls.dns_names = %#v, want %#v", got.TLS.DNSNames, []string{"cert.gateway.local", "www.gateway.local"})
	}
	if got.TLS.FingerprintSHA256 != hex.EncodeToString(certFingerprint[:]) {
		t.Fatalf("tls.fingerprint_sha256 = %q, want %q", got.TLS.FingerprintSHA256, hex.EncodeToString(certFingerprint[:]))
	}
}

func newScanFingerprintTestCertificate(t *testing.T, notBefore time.Time, notAfter time.Time) *x509.Certificate {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "cert.gateway.local",
			Organization: []string{"Yak Test"},
		},
		Issuer: pkix.Name{
			CommonName:   "Yak Test Root",
			Organization: []string{"Yak Test CA"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,
		DNSNames:  []string{"cert.gateway.local", "www.gateway.local"},
	}
	raw, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return cert
}
