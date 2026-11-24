// Copyright 2015 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package mitm provides tooling for MITMing TLS connections. It provides
// tooling to create CA certs and generate TLS configs that can be used to MITM
// a TLS connection with a provided CA certificate.
package mitm

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	gmx509 "github.com/yaklang/yaklang/common/gmsm/x509"
	"github.com/yaklang/yaklang/common/minimartian/h2"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/log"
)

// MaxSerialNumber is the upper boundary that is used to create unique serial
// numbers for the certificate. This can be any unsigned integer up to 20
// bytes (2^(8*20)-1).
var MaxSerialNumber = big.NewInt(0).SetBytes(bytes.Repeat([]byte{255}, 20))

// Config is a set of configuration values that are used to build TLS configs
// capable of MITM.
type Config struct {
	ca                     *x509.Certificate
	caChain                [][]byte
	capriv                 interface{}
	priv                   *rsa.PrivateKey
	keyID                  []byte
	validity               time.Duration
	org                    string
	h2Config               *h2.Config
	getCertificate         func(*tls.ClientHelloInfo) (*tls.Certificate, error)
	roots                  *x509.CertPool
	skipVerify             bool
	handshakeErrorCallback func(*http.Request, error)

	certmu sync.RWMutex
	certs  map[string]*tls.Certificate

	obsoleteConfig *ObsoleteTLSConfig
}

// NewAuthority creates a new CA certificate and associated
// private key.
func NewAuthority(name, organization string, validity time.Duration) (*x509.Certificate, *rsa.PrivateKey, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	pub := priv.Public()

	// Subject Key Identifier support for end entity certificate.
	// https://www.ietf.org/rfc/rfc3280.txt (section 4.2.1.2)
	pkixpub, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, nil, err
	}
	h := sha1.New()
	h.Write(pkixpub)
	keyID := h.Sum(nil)

	// TODO: keep a map of used serial numbers to avoid potentially reusing a
	// serial multiple times.
	serial, err := rand.Int(rand.Reader, MaxSerialNumber)
	if err != nil {
		return nil, nil, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   name,
			Organization: []string{organization},
		},
		SubjectKeyId:          keyID,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		NotBefore:             time.Now().Add(-validity),
		NotAfter:              time.Now().Add(validity),
		DNSNames:              []string{name},
		IsCA:                  true,
	}

	raw, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		return nil, nil, err
	}

	// Parse certificate bytes so that we have a leaf certificate.
	x509c, err := x509.ParseCertificate(raw)
	if err != nil {
		return nil, nil, err
	}

	return x509c, priv, nil
}

type ConfigOption func(*Config) error

// WithCertificateChain adds the issuer certificates following the signing CA,
// in order. The signing CA itself is included automatically.
func WithCertificateChain(issuers ...[]byte) ConfigOption {
	return func(c *Config) error {
		chain := [][]byte{c.ca.Raw}
		previous := c.ca
		for i, der := range issuers {
			issuer, err := x509.ParseCertificate(der)
			if err != nil {
				return fmt.Errorf("parse CA chain certificate %d: %w", i+1, err)
			}
			if err := previous.CheckSignatureFrom(issuer); err != nil {
				return fmt.Errorf("verify CA chain certificate %d: %w", i+1, err)
			}
			chain = append(chain, append([]byte(nil), der...))
			previous = issuer
		}
		c.caChain = chain
		return nil
	}
}

func WithObsoleteTLS(ca, gmCA *gmx509.Certificate, privateKey, gmPrivateKey interface{}) ConfigOption {
	return func(c *Config) error {
		config, err := NewObsoleteTLSConfig(ca, gmCA, privateKey, gmPrivateKey)
		if err != nil {
			return err
		}
		c.obsoleteConfig = config
		return nil
	}
}

// NewConfig creates a MITM config using the CA certificate and
// private key to generate on-the-fly certificates.
func NewConfig(ca *x509.Certificate, privateKey interface{}, opts ...ConfigOption) (*Config, error) {
	roots := x509.NewCertPool()
	roots.AddCert(ca)

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	pub := priv.Public()

	// Subject Key Identifier support for end entity certificate.
	// https://www.ietf.org/rfc/rfc3280.txt (section 4.2.1.2)
	pkixpub, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}
	h := sha1.New()
	h.Write(pkixpub)
	keyID := h.Sum(nil)

	config := &Config{
		ca:       ca,
		caChain:  [][]byte{ca.Raw},
		capriv:   privateKey,
		priv:     priv,
		keyID:    keyID,
		validity: time.Hour,
		org:      "Martian Proxy",
		certs:    make(map[string]*tls.Certificate),
		roots:    roots,
	}

	for _, opt := range opts {
		err := opt(config)
		if err != nil {
			return nil, err
		}
	}
	if config.obsoleteConfig != nil {
		config.obsoleteConfig.caChain = config.caChain
	}
	return config, nil
}

// SetValidity sets the validity window around the current time that the
// certificate is valid for.
func (c *Config) SetValidity(validity time.Duration) {
	c.validity = validity
}

// SkipTLSVerify skips the TLS certification verification check.
func (c *Config) SkipTLSVerify(skip bool) {
	c.skipVerify = skip
}

// SetOrganization sets the organization of the certificate.
func (c *Config) SetOrganization(org string) {
	c.org = org
	// 同时设置国密 TLS 的 organization
	if c.obsoleteConfig != nil {
		c.obsoleteConfig.SetOrganization(org)
	}
}

// SetH2Config configures processing of HTTP/2 streams.
func (c *Config) SetH2Config(h2Config *h2.Config) {
	c.h2Config = h2Config
}

// H2Config returns the current HTTP/2 configuration.
func (c *Config) H2Config() *h2.Config {
	return c.h2Config
}

// SetHandshakeErrorCallback sets the handshakeErrorCallback function.
func (c *Config) SetHandshakeErrorCallback(cb func(*http.Request, error)) {
	c.handshakeErrorCallback = cb
}

// HandshakeErrorCallback calls the handshakeErrorCallback function in this
// Config, if it is non-nil. Request is the connect request that this handshake
// is being executed through.
func (c *Config) HandshakeErrorCallback(r *http.Request, err error) {
	if c.handshakeErrorCallback != nil {
		c.handshakeErrorCallback(r, err)
	}
}

// certHostnameForHandshake resolves the hostname used to mint the leaf
// certificate of an inbound (client facing) TLS handshake.
//
// SNI always wins: a client that sends SNI validates the certificate against
// that name. IP-literal targets must not send SNI (RFC 6066 section 3), so a
// client that CONNECTs to https://<ip>:<port> leaves ServerName empty. In that
// case the only trustworthy source of truth is the upstream target the proxy
// is tunnelling to (fallbackHost). Minting the certificate for anything else
// (the proxy's own listen address, a hardcoded loopback) produces a leaf the
// client rejects with "certificate is valid for X, not <ip>".
func certHostnameForHandshake(serverName, fallbackHost string, conn net.Conn) (string, error) {
	if serverName != "" {
		return serverName, nil
	}
	host := strings.TrimSpace(fallbackHost)
	if host != "" {
		host = utils.ExtractHost(host)
		log.Debugf("mitm: client TLS handshake without SNI, minting certificate for upstream host %s", host)
		return host, nil
	}
	// Last resort for callers that do not know the upstream target. Transparent
	// (TProxy/redirect) entries may still expose the original destination here.
	if conn != nil && conn.LocalAddr() != nil {
		if localHost := utils.ExtractHost(conn.LocalAddr().String()); localHost != "" {
			log.Debugf("mitm: client TLS handshake without SNI, minting certificate for local address host %s", localHost)
			return localHost, nil
		}
	}
	return "", errors.New("mitm: SNI not provided and no upstream host is known, failed to build certificate")
}

// TLS returns a *tls.Config that will generate certificates on-the-fly using
// the SNI extension in the TLS ClientHello.
func (c *Config) TLS() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: c.skipVerify,
		GetCertificate: func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			host, err := certHostnameForHandshake(clientHello.ServerName, "", clientHello.Conn)
			if err != nil {
				return nil, err
			}
			return c.cert(host)
		},
		NextProtos: []string{"http/1.1"},
	}
}

// TLSForHost returns a *tls.Config that will generate certificates on-the-fly
// using SNI from the connection, or fall back to the provided hostname.
//
// hostname must be the upstream target of the connection (CONNECT authority,
// SOCKS5 destination, ...): it is what a client connecting to an IP literal
// validates the generated certificate against.
func (c *Config) TLSForHost(hostname string, h2Verify bool) *tls.Config {
	nextProtos := []string{"http/1.1"}
	if c.h2AllowedHost(hostname) && h2Verify {
		nextProtos = []string{"h2", "http/1.1"}
	}
	return &tls.Config{
		InsecureSkipVerify: c.skipVerify,
		GetCertificate: func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			host, err := certHostnameForHandshake(clientHello.ServerName, hostname, clientHello.Conn)
			if err != nil {
				return nil, err
			}
			return c.cert(host)
		},
		NextProtos: nextProtos,
	}
}

func (c *Config) ObsoleteTLS(hostname string, h2Verify bool) *gmtls.Config {
	if c.obsoleteConfig == nil {
		return nil
	}
	nextProtos := []string{"http/1.1"}
	if c.h2AllowedHost(hostname) && h2Verify {
		nextProtos = []string{"h2", "http/1.1"}
	}
	return &gmtls.Config{
		InsecureSkipVerify: c.skipVerify,
		GetCertificate: func(clientHello *gmtls.ClientHelloInfo) (*gmtls.Certificate, error) {
			host, err := certHostnameForHandshake(clientHello.ServerName, hostname, clientHello.Conn)
			if err != nil {
				return nil, err
			}
			gmFlag := false
			// 检查支持协议中是否包含GMSSL
			for _, v := range clientHello.SupportedVersions {
				if v == gmtls.VersionGMSSL {
					gmFlag = true
					break
				}
			}
			if gmFlag && !c.obsoleteConfig.disableMimicGMServer {
				return c.obsoleteConfig.getSigningCert(host)
			} else {
				return c.obsoleteConfig.cert(host)
			}
		},
		GetKECertificate: func(clientHello *gmtls.ClientHelloInfo) (*gmtls.Certificate, error) {
			host, err := certHostnameForHandshake(clientHello.ServerName, hostname, clientHello.Conn)
			if err != nil {
				return nil, err
			}

			return c.obsoleteConfig.getEncryptionCert(host)
		},
		NextProtos: nextProtos,
	}
}

func (c *Config) h2AllowedHost(host string) bool {
	return true
	//temporarily disable this feature since upstream yak not implemented this option
	//return c.h2Config != nil &&
	//	c.h2Config.AllowedHostsFilter != nil &&
	//	c.h2Config.AllowedHostsFilter(host)
}

func (c *Config) GetCertificateByHostname(hostname string) (*tls.Certificate, error) {
	return c.cert(hostname)
}

func (c *Config) cert(hostname string) (*tls.Certificate, error) {
	// Remove the port if it exists.
	host, _, err := net.SplitHostPort(hostname)
	if err == nil {
		hostname = host
	}

	c.certmu.RLock()
	tlsc, ok := c.certs[hostname]
	c.certmu.RUnlock()

	if ok {
		log.Debugf("mitm: cache hit for %s", hostname)

		// Check validity of the certificate for hostname match, expiry, etc. In
		// particular, if the cached certificate has expired, create a new one.
		if _, err := tlsc.Leaf.Verify(x509.VerifyOptions{
			DNSName: hostname,
			Roots:   c.roots,
		}); err == nil {
			return tlsc, nil
		}

		log.Debugf("mitm: invalid certificate in cache for %s", hostname)
	}

	log.Debugf("mitm: cache miss for %s", hostname)

	serial, err := rand.Int(rand.Reader, MaxSerialNumber)
	if err != nil {
		return nil, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Country:            []string{c.org},
			Province:           []string{c.org},
			Locality:           []string{c.org},
			Organization:       []string{c.org},
			OrganizationalUnit: []string{c.org},
			CommonName:         hostname,
		},
		SubjectKeyId:          c.keyID,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		NotBefore:             time.Now().Add(-c.validity),
		NotAfter:              time.Now().Add(c.validity),
	}

	if ip := net.ParseIP(hostname); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{hostname}
	}

	raw, err := x509.CreateCertificate(rand.Reader, tmpl, c.ca, c.priv.Public(), c.capriv)
	if err != nil {
		return nil, err
	}

	// Parse certificate bytes so that we have a leaf certificate.
	x509c, err := x509.ParseCertificate(raw)
	if err != nil {
		return nil, err
	}

	tlsc = &tls.Certificate{
		Certificate: append([][]byte{raw}, c.caChain...),
		PrivateKey:  c.priv,
		Leaf:        x509c,
	}

	c.certmu.Lock()
	c.certs[hostname] = tlsc
	c.certmu.Unlock()

	return tlsc, nil
}
