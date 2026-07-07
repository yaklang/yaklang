package tlsutils

import (
	"bytes"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"github.com/yaklang/yaklang/common/utils"
	"math/big"
	"net"
	"time"
)

type SelfSignConfig struct {
	NotAfter       time.Time
	NotBefore      time.Time
	SignTo         []string
	PrivateKey     *rsa.PrivateKey
	EnableAuth     bool
	AlternativeDNS []string
	AlternativeIP  []string
	Org            string
	// SignatureAlgorithm forces a specific signature algorithm on the generated
	// certificate. Leave as the zero value (x509.UnknownSignatureAlgorithm) to
	// let Go pick the default (SHA-256 for RSA keys). Set to x509.SHA1WithRSA to
	// mint a SHA-1 signed root for legacy Windows (7/2008R2) compatibility, where
	// the system crypto stack cannot reliably validate SHA-256 roots without
	// KB3033929.
	SignatureAlgorithm x509.SignatureAlgorithm
}

type SelfSignConfigOpt func(*SelfSignConfig)

func WithSelfSign_NotAfter(t time.Time) SelfSignConfigOpt {
	return func(c *SelfSignConfig) {
		c.NotAfter = t
	}
}

func WithSelfSign_NotBefore(t time.Time) SelfSignConfigOpt {
	return func(c *SelfSignConfig) {
		c.NotBefore = t
	}
}

func WithSelfSign_SignTo(s ...string) SelfSignConfigOpt {
	return func(c *SelfSignConfig) {
		c.SignTo = s
	}
}

func WithSelfSign_PrivateKey(p *rsa.PrivateKey) SelfSignConfigOpt {
	return func(c *SelfSignConfig) {
		c.PrivateKey = p
	}
}
func WithSelfSign_Organization(s string) SelfSignConfigOpt {
	return func(c *SelfSignConfig) {
		c.Org = s
	}
}

func WithSelfSign_EnableAuth(b bool) SelfSignConfigOpt {
	return func(c *SelfSignConfig) {
		c.EnableAuth = b
	}
}

// WithSelfSign_SignatureAlgorithm forces a specific signature algorithm on the
// generated certificate. Pass x509.UnknownSignatureAlgorithm (0) to keep the Go
// default. This is used to emit SHA-1 signed roots for legacy Windows
// (7/2008R2) compatibility where SHA-256 roots are not validated reliably.
func WithSelfSign_SignatureAlgorithm(algo x509.SignatureAlgorithm) SelfSignConfigOpt {
	return func(c *SelfSignConfig) {
		c.SignatureAlgorithm = algo
	}
}

func SelfSignCACertificateAndPrivateKey(common string, opts ...SelfSignConfigOpt) ([]byte, []byte, error) {
	config := &SelfSignConfig{
		NotAfter:  time.Now().AddDate(10, 0, 0),
		NotBefore: time.Now(),
		SignTo:    []string{},
	}

	for _, opt := range opts {
		opt(config)
	}

	var (
		priv         = config.PrivateKey
		commonName   = common
		auth         = config.EnableAuth
		alternateIPs []net.IP
		alternateDNS = config.AlternativeDNS
	)

	for _, signto := range append(config.SignTo, config.AlternativeIP...) {
		if utils.IsIPv4(signto) {
			alternateIPs = append(alternateIPs, net.ParseIP(signto))
		} else {
			alternateDNS = append(alternateDNS, signto)
		}
	}

	var err error
	if priv == nil {
		priv, err = rsa.GenerateKey(cryptorand.Reader, 2048)
		if err != nil {
			return nil, nil, err
		}
	}

	if commonName == "" {
		return nil, nil, utils.Errorf("empty common name")
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	sid, err := cryptorand.Int(cryptorand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, err
	}

	var notBeforeYear = time.Now().Add(-20 * 24 * time.Hour)
	template := x509.Certificate{
		SerialNumber:       sid,
		SignatureAlgorithm: config.SignatureAlgorithm,
		Subject: pkix.Name{
			Country:            []string{config.Org},
			Province:           []string{config.Org},
			Locality:           []string{config.Org},
			Organization:       []string{config.Org},
			OrganizationalUnit: []string{config.Org},
			CommonName:         commonName,
		},
		Issuer: pkix.Name{
			Country:            []string{config.Org},
			Province:           []string{config.Org},
			Locality:           []string{config.Org},
			Organization:       []string{config.Org},
			OrganizationalUnit: []string{config.Org},
			CommonName:         commonName,
		},
		NotBefore: notBeforeYear,
		NotAfter:  time.Now().Add(time.Hour * 24 * 365 * 10),
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	if !auth {
		template.ExtKeyUsage = nil
	}

	template.IPAddresses = append(template.IPAddresses, alternateIPs...)
	template.DNSNames = append(template.DNSNames, alternateDNS...)

	derBytes, err := x509.CreateCertificate(cryptorand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}

	// Generate cert
	certBuffer := bytes.Buffer{}
	if err := pem.Encode(&certBuffer, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return nil, nil, err
	}

	// Generate key
	keyBuffer := bytes.Buffer{}
	if err := pem.Encode(&keyBuffer, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
		return nil, nil, err
	}

	return certBuffer.Bytes(), keyBuffer.Bytes(), nil
}

func newPEMBlock(title string, bytes []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: title, Bytes: bytes})
}
