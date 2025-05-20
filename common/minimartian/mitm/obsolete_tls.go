package mitm

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	gmx509 "github.com/yaklang/yaklang/common/gmsm/x509"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian/h2"
	"net"
	"net/http"
	"sync"
	"time"
)

type ObsoleteTLSConfig struct {
	ca                     *gmx509.Certificate
	capriv                 interface{}
	priv                   *rsa.PrivateKey
	keyID                  []byte
	validity               time.Duration
	org                    string
	h2Config               *h2.Config
	getCertificate         func(*gmtls.ClientHelloInfo) (*gmtls.Certificate, error)
	roots                  *gmx509.CertPool
	skipVerify             bool
	handshakeErrorCallback func(*http.Request, error)

	certmu sync.RWMutex
	certs  map[string]*gmtls.Certificate
}

func NewObsoleteTLSConfig(ca *gmx509.Certificate, privateKey any) (*ObsoleteTLSConfig, error) {
	roots := gmx509.NewCertPool()
	roots.AddCert(ca)

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	pub := priv.Public()

	pkixpub, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}
	h := sha1.New()
	h.Write(pkixpub)
	keyID := h.Sum(nil)

	return &ObsoleteTLSConfig{
		ca:       ca,
		capriv:   privateKey,
		priv:     priv,
		keyID:    keyID,
		validity: time.Hour,
		org:      "Martian Proxy",
		certs:    make(map[string]*gmtls.Certificate),
		roots:    roots,
	}, nil
}

func (c *ObsoleteTLSConfig) cert(hostname string) (*gmtls.Certificate, error) {
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
		if _, err := tlsc.Leaf.Verify(gmx509.VerifyOptions{
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

	tmpl := &gmx509.Certificate{
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
		KeyUsage:              gmx509.KeyUsageKeyEncipherment | gmx509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []gmx509.ExtKeyUsage{gmx509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		NotBefore:             time.Now().Add(-c.validity),
		NotAfter:              time.Now().Add(c.validity),
	}

	if ip := net.ParseIP(hostname); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{hostname}
	}

	raw, err := gmx509.CreateCertificate(tmpl, c.ca, c.priv.Public(), c.priv)
	if err != nil {
		return nil, err
	}

	// Parse certificate bytes so that we have a leaf certificate.
	x509c, err := gmx509.ParseCertificate(raw)
	if err != nil {
		return nil, err
	}

	tlsc = &gmtls.Certificate{
		Certificate: [][]byte{raw, c.ca.Raw},
		PrivateKey:  c.priv,
		Leaf:        x509c,
	}

	c.certmu.Lock()
	c.certs[hostname] = tlsc
	c.certmu.Unlock()

	return tlsc, nil
}
