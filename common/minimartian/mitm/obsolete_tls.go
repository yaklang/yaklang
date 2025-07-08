package mitm

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509/pkix"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/gmsm/sm2"
	gmx509 "github.com/yaklang/yaklang/common/gmsm/x509"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian/h2"
)

type ObsoleteTLSConfig struct {
	ca                     *gmx509.Certificate
	gmCA                   *gmx509.Certificate
	capriv                 interface{}
	gmCAPriv               interface{}
	priv                   *rsa.PrivateKey
	gmPriv                 *sm2.PrivateKey
	validity               time.Duration
	org                    string
	h2Config               *h2.Config
	getCertificate         func(*gmtls.ClientHelloInfo) (*gmtls.Certificate, error)
	roots                  *gmx509.CertPool
	skipVerify             bool
	handshakeErrorCallback func(*http.Request, error)

	certmu sync.RWMutex
	// RSA TLS
	certs map[string]*gmtls.Certificate
	// 分离两种GMTLS证书的缓存
	signingCerts    map[string]*gmtls.Certificate // 签名证书缓存
	encryptionCerts map[string]*gmtls.Certificate // 加密证书缓存

	disableMimicGMServer bool // 是否关闭MITM充当中间人国密服务器功能
}

func NewObsoleteTLSConfig(ca, gmCA *gmx509.Certificate, privateKey, gmPrivateKey any) (*ObsoleteTLSConfig, error) {
	var gmPriv *sm2.PrivateKey
	var err error
	disableMimicGMServer := false
	roots := gmx509.NewCertPool()

	if gmCA == nil || gmPrivateKey == nil {
		disableMimicGMServer = true
		log.Error("MITM mimic GM Server feature disabled due to GM certificates error")
	}

	_, ok := privateKey.(crypto.Signer)
	if !ok {
		return nil, errors.New("ca private key does not implement crypto.Signer")
	}
	roots.AddCert(ca)

	if !disableMimicGMServer {
		_, ok = gmPrivateKey.(crypto.Signer)
		if ok {
			roots.AddCert(gmCA)
			gmPriv, err = sm2.GenerateKey(rand.Reader)
			if err != nil {
				log.Errorf("sm2.GenerateKey failed when initializing ObsoleteTLSConfig: %s", err)
				disableMimicGMServer = true
			}
		} else {
			log.Error("gm ca private key does not implement crypto.Signer")
			disableMimicGMServer = true
		}
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	return &ObsoleteTLSConfig{
		ca:                   ca,
		gmCA:                 gmCA,
		capriv:               privateKey,
		gmCAPriv:             gmPrivateKey,
		priv:                 priv,
		gmPriv:               gmPriv,
		validity:             time.Hour,
		org:                  "MITMServer",
		certs:                make(map[string]*gmtls.Certificate),
		signingCerts:         make(map[string]*gmtls.Certificate),
		encryptionCerts:      make(map[string]*gmtls.Certificate),
		roots:                roots,
		disableMimicGMServer: disableMimicGMServer,
	}, nil
}

// 生成签名证书（仅用于数字签名和身份验证）
func (c *ObsoleteTLSConfig) getSigningCert(hostname string) (*gmtls.Certificate, error) {
	if c.disableMimicGMServer {
		return nil, errors.New("failed to obtain signing certificate for host due to gmCA cert not provided or malformed")
	}
	host, _, err := net.SplitHostPort(hostname)
	if err == nil {
		hostname = host
	}

	c.certmu.RLock()
	tlsc, ok := c.signingCerts[hostname]
	c.certmu.RUnlock()

	if ok {
		log.Debugf("mitm: signing cert cache hit for %s", hostname)
		if _, err := tlsc.Leaf.Verify(gmx509.VerifyOptions{
			DNSName: hostname,
			Roots:   c.roots,
		}); err == nil {
			return tlsc, nil
		}
	}

	log.Debugf("mitm: generating signing cert for %s", hostname)

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
		// 只用于数字签名，不用于加密
		KeyUsage:              gmx509.KeyUsageDigitalSignature | gmx509.KeyUsageCertSign,
		ExtKeyUsage:           []gmx509.ExtKeyUsage{gmx509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		NotBefore:             time.Now().Add(-c.validity),
		NotAfter:              time.Now().Add(c.validity),
		SignatureAlgorithm:    gmx509.SM2WithSM3,
	}

	if ip := net.ParseIP(hostname); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{hostname}
	}

	raw, err := gmx509.CreateCertificate(tmpl, c.gmCA, c.gmPriv.Public(), c.gmCAPriv.(crypto.Signer))
	if err != nil {
		return nil, err
	}

	x509c, err := gmx509.ParseCertificate(raw)
	if err != nil {
		return nil, err
	}

	tlsc = &gmtls.Certificate{
		Certificate: [][]byte{raw},
		PrivateKey:  c.gmPriv, // 使用SM2私钥
		Leaf:        x509c,
	}

	c.certmu.Lock()
	if c.signingCerts == nil {
		c.signingCerts = make(map[string]*gmtls.Certificate)
	}
	c.signingCerts[hostname] = tlsc
	c.certmu.Unlock()

	return tlsc, nil
}

// 生成加密证书（仅用于密钥交换和数据加密）
func (c *ObsoleteTLSConfig) getEncryptionCert(hostname string) (*gmtls.Certificate, error) {
	if c.disableMimicGMServer {
		return nil, errors.New("failed to obtain encryption certificate for host due to gmCA cert not provided or malformed")
	}
	host, _, err := net.SplitHostPort(hostname)
	if err == nil {
		hostname = host
	}

	c.certmu.RLock()
	tlsc, ok := c.encryptionCerts[hostname]
	c.certmu.RUnlock()

	if ok {
		log.Debugf("mitm: encryption cert cache hit for %s", hostname)
		if _, err := tlsc.Leaf.Verify(gmx509.VerifyOptions{
			DNSName: hostname,
			Roots:   c.roots,
		}); err == nil {
			return tlsc, nil
		}
	}

	log.Debugf("mitm: generating encryption cert for %s", hostname)

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
		// 只用于加密，不用于签名
		KeyUsage:              gmx509.KeyUsageKeyEncipherment | gmx509.KeyUsageDataEncipherment,
		ExtKeyUsage:           []gmx509.ExtKeyUsage{gmx509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		NotBefore:             time.Now().Add(-c.validity),
		NotAfter:              time.Now().Add(c.validity),
		SignatureAlgorithm:    gmx509.SM2WithSM3,
	}

	if ip := net.ParseIP(hostname); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{hostname}
	}

	raw, err := gmx509.CreateCertificate(tmpl, c.gmCA, c.gmPriv.Public(), c.gmCAPriv.(crypto.Signer))
	if err != nil {
		return nil, err
	}

	x509c, err := gmx509.ParseCertificate(raw)
	if err != nil {
		return nil, err
	}

	tlsc = &gmtls.Certificate{
		Certificate: [][]byte{raw},
		PrivateKey:  c.gmPriv, // 使用不同的SM2私钥
		Leaf:        x509c,
	}

	c.certmu.Lock()
	if c.encryptionCerts == nil {
		c.encryptionCerts = make(map[string]*gmtls.Certificate)
	}
	c.encryptionCerts[hostname] = tlsc
	c.certmu.Unlock()

	return tlsc, nil
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

	raw, err := gmx509.CreateCertificate(tmpl, c.ca, c.priv.Public(), c.capriv.(crypto.Signer))
	if err != nil {
		return nil, err
	}

	// Parse certificate bytes so that we have a leaf certificate.
	x509c, err := gmx509.ParseCertificate(raw)
	if err != nil {
		return nil, err
	}

	tlsc = &gmtls.Certificate{
		Certificate: [][]byte{raw},
		PrivateKey:  c.priv,
		Leaf:        x509c,
	}

	c.certmu.Lock()
	c.certs[hostname] = tlsc
	c.certmu.Unlock()

	return tlsc, nil
}
