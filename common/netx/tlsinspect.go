package netx

import (
	"bytes"
	"context"
	"crypto/x509"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	gmx509 "github.com/yaklang/yaklang/common/gmsm/x509"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
)

type TLSInspectResult struct {
	Version     uint16
	CipherSuite uint16
	ServerName  string

	Protocol        string
	Description     string
	Raw             []byte
	RelativeDomains []string
	RelativeEmail   []string
	RelativeAccount []string
	RelativeURIs    []string
}

func (t TLSInspectResult) String() string {
	return t.Description
}

func (t TLSInspectResult) Show() {
	fmt.Println(t.Description)
}

func TLSInspectTimeout(addr string, seconds float64, proto ...string) ([]*TLSInspectResult, error) {
	return TLSInspectContext(utils.TimeoutContextSeconds(seconds), addr, proto...)
}

// parseCertificateToResult parses a certificate and connection state info into TLSInspectResult
func parseCertificateToResult(cert *x509.Certificate, version, cipherSuite uint16, serverName, protocol string) *TLSInspectResult {
	if cert == nil {
		return nil
	}
	var domains []string

	var urls []string
	for _, u := range cert.URIs {
		urls = append(urls, u.String())
		host, _, _ := utils.ParseStringToHostPort(u.Hostname())
		if host == "" {
			host = u.Hostname()
		}
		if host == "" {
			continue
		}
		domains = append(domains, host)
	}

	domains = append(domains, cert.ExcludedURIDomains...)
	domains = append(domains, cert.PermittedURIDomains...)

	var emails []string
	domains = append(domains, cert.DNSNames...)
	domains = append(domains, cert.PermittedDNSDomains...)
	domains = append(domains, cert.ExcludedDNSDomains...)
	emails = append(emails, cert.EmailAddresses...)
	emails = append(emails, cert.PermittedEmailAddresses...)
	emails = append(emails, cert.ExcludedEmailAddresses...)
	emails = utils.RemoveRepeatStringSlice(emails)
	var accounts []string
	for _, e := range emails {
		if strings.Contains(e, "@") {
			r := strings.Split(e, "@")
			domains = append(domains, r[1])
			accounts = append(accounts, r[0])
		} else {
			accounts = append(accounts, e)
		}
	}
	domains = utils.RemoveRepeatStringSlice(domains)
	text, err := tlsutils.CertificateText(cert)
	if err != nil {
		return nil
	}

	return &TLSInspectResult{
		Version:         version,
		CipherSuite:     cipherSuite,
		ServerName:      serverName,
		Protocol:        protocol,
		Description:     text,
		Raw:             cert.Raw,
		RelativeDomains: domains,
		RelativeEmail:   emails,
		RelativeAccount: utils.RemoveRepeatStringSlice(accounts),
		RelativeURIs:    utils.RemoveRepeatStringSlice(urls),
	}
}

// parseGMCertificateToResult parses a GM certificate and connection state info into TLSInspectResult
func parseGMCertificateToResult(cert *gmx509.Certificate, version, cipherSuite uint16, serverName, protocol string) *TLSInspectResult {
	if cert == nil {
		return nil
	}
	var domains []string

	// gmsm/x509.Certificate doesn't have URIs field, skip URI parsing

	var emails []string
	domains = append(domains, cert.DNSNames...)
	domains = append(domains, cert.PermittedDNSDomains...)
	emails = append(emails, cert.EmailAddresses...)
	emails = utils.RemoveRepeatStringSlice(emails)
	var accounts []string
	for _, e := range emails {
		if strings.Contains(e, "@") {
			r := strings.Split(e, "@")
			domains = append(domains, r[1])
			accounts = append(accounts, r[0])
		} else {
			accounts = append(accounts, e)
		}
	}
	domains = utils.RemoveRepeatStringSlice(domains)

	// try to parse the certificate using tlsutils.CertificateText
	// first convert to standard x509.Certificate if possible
	stdCert, err := x509.ParseCertificate(cert.Raw)
	var text string
	if err == nil {
		text, err = tlsutils.CertificateText(stdCert)
		if err != nil {
			// fallback to basic info
			text = fmt.Sprintf("Subject: %s\nIssuer: %s\nNot Before: %s\nNot After: %s",
				cert.Subject.String(), cert.Issuer.String(),
				cert.NotBefore.Format(time.RFC3339), cert.NotAfter.Format(time.RFC3339))
		}
	} else {
		// fallback to basic info for GM certificates that can't be parsed by standard x509
		text = fmt.Sprintf("Subject: %s\nIssuer: %s\nNot Before: %s\nNot After: %s",
			cert.Subject.String(), cert.Issuer.String(),
			cert.NotBefore.Format(time.RFC3339), cert.NotAfter.Format(time.RFC3339))
	}

	return &TLSInspectResult{
		Version:         version,
		CipherSuite:     cipherSuite,
		ServerName:      serverName,
		Protocol:        protocol,
		Description:     text,
		Raw:             cert.Raw,
		RelativeDomains: domains,
		RelativeEmail:   emails,
		RelativeAccount: utils.RemoveRepeatStringSlice(accounts),
		RelativeURIs:    []string{},
	}
}

// deduplicateResults removes duplicate results based on Raw certificate bytes
func deduplicateResults(results []*TLSInspectResult) []*TLSInspectResult {
	seen := make(map[string]bool)
	var deduplicated []*TLSInspectResult
	for _, r := range results {
		if r == nil {
			continue
		}
		key := string(r.Raw)
		if !seen[key] {
			seen[key] = true
			deduplicated = append(deduplicated, r)
		}
	}
	return deduplicated
}

// tlsInspectWithGMTLS tries to inspect TLS certificates using gmtls with optional GMSupport
func tlsInspectWithGMTLS(ctx context.Context, addr string, host string, port int, dialTimeout time.Duration, gmOnly bool, proto ...string) []*TLSInspectResult {
	conn, err := DialTCPTimeout(dialTimeout, utils.HostPort(host, port))
	if err != nil {
		log.Debugf("TLSInspect(gmtls, gmOnly=%v): dial error: %s", gmOnly, err)
		return nil
	}
	defer conn.Close()

	inspectCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	gmtlsConfig := &gmtls.Config{
		ServerName:         host,
		InsecureSkipVerify: true,
		MinVersion:         gmtls.VersionSSL30, // nolint[:staticcheck]
		MaxVersion:         gmtls.VersionTLS13,
		NextProtos:         []string{"h2", "http/1.1"},
	}

	if gmOnly {
		gmtlsConfig.GMSupport = &gmtls.GMSupport{WorkMode: gmtls.ModeGMSSLOnly}
	}

	if len(proto) > 0 {
		gmtlsConfig.NextProtos = proto
	}

	gmtlsConn := gmtls.Client(conn, gmtlsConfig)
	err = gmtlsConn.HandshakeContext(inspectCtx)
	if err != nil {
		log.Debugf("TLSInspect(gmtls, gmOnly=%v): handshake error: %s", gmOnly, err)
		return nil
	}

	// After successful handshake, get connection state and extract certificates
	state := gmtlsConn.ConnectionState()
	var results []*TLSInspectResult
	for _, cert := range state.PeerCertificates {
		result := parseGMCertificateToResult(cert, state.Version, state.CipherSuite, state.ServerName, state.NegotiatedProtocol)
		if result != nil {
			results = append(results, result)
		}
	}

	log.Debugf("TLSInspect(gmtls, gmOnly=%v): got %d results", gmOnly, len(results))
	return results
}

func TLSInspectContext(ctx context.Context, addr string, proto ...string) ([]*TLSInspectResult, error) {
	host, port, _ := utils.ParseStringToHostPort(addr)
	if port <= 0 {
		port = 443
	}
	if host == "" {
		host = addr
	}

	if ctx == nil {
		ctx = context.Background()
	}

	// calculate dial timeout based on context deadline
	dialTimeout := 5 * time.Second
	ddl, ok := ctx.Deadline()
	if ok {
		dialTimeout = ddl.Sub(time.Now())
		if dialTimeout <= 0 {
			dialTimeout = 5 * time.Second
		}
		// split the timeout for multiple attempts
		dialTimeout = dialTimeout / 2
		if dialTimeout < time.Second {
			dialTimeout = time.Second
		}
	}

	var allResults []*TLSInspectResult

	// try GMTLS Only mode first (for servers that only support GM TLS)
	gmResults := tlsInspectWithGMTLS(ctx, addr, host, port, dialTimeout, true, proto...)
	allResults = append(allResults, gmResults...)

	// try standard TLS mode (using gmtls library without GMSupport, which supports standard TLS)
	stdResults := tlsInspectWithGMTLS(ctx, addr, host, port, dialTimeout, false, proto...)
	allResults = append(allResults, stdResults...)

	// deduplicate results based on certificate Raw bytes
	deduplicated := deduplicateResults(allResults)

	// if we got any results, return them
	if len(deduplicated) > 0 {
		return deduplicated, nil
	}

	// if no results, return empty slice with no error (handshake might have failed but we still tried)
	return []*TLSInspectResult{}, nil
}

// Inspect 检查目标地址的TLS证书，并返回其证书信息与错误
// 支持检测普通TLS和国密TLS(GMTLS)证书，自动尝试多种TLS握手方式并去重返回结果
// Example:
// ```
// cert, err := tls.Inspect("yaklang.io:443")
// ```
func TLSInspect(addr string) ([]*TLSInspectResult, error) {
	return TLSInspectTimeout(addr, 10)
}

// InspectForceHttp2 检查目标地址的TLS证书，并返回其证书信息与错误，强制使用HTTP/2协议
// 支持检测普通TLS和国密TLS(GMTLS)证书
// Example:
// ```
// cert, err := tls.InspectForceHttp2("yaklang.io:443")
// ```
func TLSInspectForceHttp2(addr string) ([]*TLSInspectResult, error) {
	return TLSInspectTimeout(addr, 10, "h2")
}

// InspectForceHttp1_1 检查目标地址的TLS证书，并返回其证书信息与错误，强制使用HTTP/1.1协议
// 支持检测普通TLS和国密TLS(GMTLS)证书
// Example:
// ```
// cert, err := tls.InspectForceHttp1_1("yaklang.io:443")
// ```
func TLSInspectForceHttp1_1(addr string) ([]*TLSInspectResult, error) {
	return TLSInspectTimeout(addr, 10, "http/1.1")
}

// IsGMTLS checks if the TLSInspectResult is from a GM TLS connection
// GM TLS typically uses version 0x0101 (VersionGMSSL = 0x0101)
func (t *TLSInspectResult) IsGMTLS() bool {
	return t.Version == gmtls.VersionGMSSL
}

// IsSM2Certificate checks if the certificate uses SM2 algorithm
func (t *TLSInspectResult) IsSM2Certificate() bool {
	if len(t.Raw) == 0 {
		return false
	}
	// SM2 signature algorithm OID is 1.2.156.10197.1.501
	// Check if the certificate raw bytes contain SM2 related OID
	// OID bytes: 06 08 2A 81 1C CF 55 01 83 75 (SM2WithSM3)
	return bytes.Contains(t.Raw, []byte{0x06, 0x08, 0x2A, 0x81, 0x1C, 0xCF, 0x55, 0x01, 0x83, 0x75})
}
