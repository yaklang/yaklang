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
	"math/big"
	"net"
	"net/http"
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

// TLS returns a *tls.Config that will generate certificates on-the-fly using
// the SNI extension in the TLS ClientHello.
func (c *Config) TLS() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: c.skipVerify,
		GetCertificate: func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			if clientHello.ServerName == "" {
				if clientHello.Conn != nil && clientHello.Conn.LocalAddr() != nil {
					host := utils.ExtractHost(clientHello.Conn.LocalAddr().String())
					return c.cert(host)
				}
				return nil, errors.New("mitm: SNI not provided, failed to build certificate")
			}

			return c.cert(clientHello.ServerName)
		},
		NextProtos: []string{"http/1.1"},
	}
}

// TLSForHost returns a *tls.Config that will generate certificates on-the-fly
// using SNI from the connection, or fall back to the provided hostname.
func (c *Config) TLSForHost(hostname string, h2Verify bool) *tls.Config {
	nextProtos := []string{"http/1.1"}
	if c.h2AllowedHost(hostname) && h2Verify {
		nextProtos = []string{"h2", "http/1.1"}
	}
	return &tls.Config{
		InsecureSkipVerify: c.skipVerify,
		GetCertificate: func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			host := clientHello.ServerName
			if host == "" {
				host = hostname
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
			host := clientHello.ServerName
			if host == "" {
				host = hostname
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
			host := clientHello.ServerName
			if host == "" {
				host = hostname
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
		Certificate: [][]byte{raw, c.ca.Raw},
		PrivateKey:  c.priv,
		Leaf:        x509c,
	}

	c.certmu.Lock()
	c.certs[hostname] = tlsc
	c.certmu.Unlock()

	return tlsc, nil
}
