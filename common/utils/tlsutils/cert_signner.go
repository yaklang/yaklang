package tlsutils

import (
	"bytes"
	"crypto"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"math/big"
	"net"
	"os"
	"time"
)

// CertConfig 包含了生成证书所需的所有可配置属性。
type CertConfig struct {
	// Subject (主题) 信息
	Country            string
	Province           string
	Locality           string
	Organization       string
	OrganizationalUnit string
	CommonName         string

	// 有效期
	NotBefore time.Time
	NotAfter  time.Time

	// 主体备用名称 (SAN)
	AlternativeDNS []string
	AlternativeIPs []net.IP

	// 密钥用途
	KeyUsage    x509.KeyUsage
	ExtKeyUsage []x509.ExtKeyUsage

	// 要用于证书的私钥。如果为 nil, 将会自动生成一个新的 2048 位 RSA 私钥。
	PrivateKey crypto.Signer
}

// CertOption 是一个用于配置 CertConfig 的函数类型。
type CertOption func(*CertConfig)

// -----------------------------------------------------------------------------
// Section: Certificate Configuration Options (With... functions)
// -----------------------------------------------------------------------------

// WithCommonName 设置证书的通用名称 (Common Name)。
func WithCommonName(cn string) CertOption {
	return func(c *CertConfig) {
		c.CommonName = cn
	}
}

// WithOrganization 设置证书的组织 (Organization)。
func WithOrganization(org string) CertOption {
	return func(c *CertConfig) {
		c.Organization = org
	}
}

func WithCountry(country string) CertOption {
	return func(c *CertConfig) {
		c.Country = country
	}
}

func WithProvince(province string) CertOption {
	return func(c *CertConfig) {
		c.Province = province
	}
}

func WithLocality(locality string) CertOption {
	return func(c *CertConfig) {
		c.Locality = locality
	}
}

func WithOrganizationalUnit(ou string) CertOption {
	return func(c *CertConfig) {
		c.OrganizationalUnit = ou
	}
}

// WithValidity 设置证书的有效期（从现在开始，持续时间为 duration）。
func WithValidity(duration time.Duration) CertOption {
	return func(c *CertConfig) {
		c.NotBefore = time.Now().Add(-5 * time.Minute) // 提前5分钟生效以避免时间同步问题
		c.NotAfter = c.NotBefore.Add(duration)
	}
}

// WithNotAfter 设置证书的过期时间。
func WithNotAfter(t time.Time) CertOption {
	return func(c *CertConfig) {
		c.NotAfter = t
	}
}

// WithNotBefore 设置证书的生效时间。
func WithNotBefore(t time.Time) CertOption {
	return func(c *CertConfig) {
		c.NotBefore = t
	}
}

// WithAlternativeDNS 添加一个或多个 DNS 备用名称 (SAN)。
func WithAlternativeDNS(dnsNames ...string) CertOption {
	return func(c *CertConfig) {
		c.AlternativeDNS = append(c.AlternativeDNS, dnsNames...)
	}
}

// WithAlternativeIPs 添加一个或多个 IP 备用名称 (SAN)。
func WithAlternativeIPs(ips ...net.IP) CertOption {
	return func(c *CertConfig) {
		c.AlternativeIPs = append(c.AlternativeIPs, ips...)
	}
}

// WithAlternativeIPStrings 添加一个或多个字符串格式的 IP 备用名称 (SAN)。
// 无效的 IP 字符串将被忽略。
func WithAlternativeIPStrings(ipStrings ...string) CertOption {
	return func(c *CertConfig) {
		for _, ipStr := range ipStrings {
			if ip := net.ParseIP(ipStr); ip != nil {
				c.AlternativeIPs = append(c.AlternativeIPs, ip)
			}
		}
	}
}

// WithPrivateKey 使用一个已有的私钥来生成证书请求，而不是自动创建新私钥。
// 私钥必须实现 crypto.Signer 接口。
func WithPrivateKey(key crypto.Signer) CertOption {
	return func(c *CertConfig) {
		c.PrivateKey = key
	}
}

// WithKeyUsage 设置证书的密钥用途 (Key Usage)。
func WithKeyUsage(usage x509.KeyUsage) CertOption {
	return func(c *CertConfig) {
		c.KeyUsage = usage
	}
}

// WithExtKeyUsage 设置证书的扩展密钥用途 (Extended Key Usage)。
func WithExtKeyUsage(usage ...x509.ExtKeyUsage) CertOption {
	return func(c *CertConfig) {
		c.ExtKeyUsage = usage
	}
}

func WithPrivateKeyFromFile(path string) CertOption {
	return func(c *CertConfig) {
		keyData, err := os.ReadFile(path)
		if err != nil {
			log.Errorf("failed to read private key from file: %v", err)
			return
		}
		privateKey, err := ParsePrivateKey(keyData)
		if err != nil {
			return
		}
		c.PrivateKey = privateKey
	}
}

func WithPrivateKeyFromBytes(key []byte) CertOption {
	return func(c *CertConfig) {
		privateKey, err := ParsePrivateKey(key)
		if err != nil {
			return
		}
		c.PrivateKey = privateKey
	}
}

// -----------------------------------------------------------------------------
// Section: Core Certificate Generation Functions
// -----------------------------------------------------------------------------

// GenerateCA 创建一个新的自签名 CA 证书和私钥。
// 返回 PEM 编码的证书和私钥。
func GenerateCA(opts ...CertOption) ([]byte, []byte, error) {
	// 为 CA 设置默认的 KeyUsage
	opts = append(opts,
		WithKeyUsage(x509.KeyUsageCertSign|x509.KeyUsageCRLSign|x509.KeyUsageDigitalSignature),
	)

	config, err := newCertConfig(opts...)
	if err != nil {
		return nil, nil, err
	}

	// 创建 CA 证书模板
	template, err := createTemplate(config, true) // isCA = true
	if err != nil {
		return nil, nil, err
	}

	// 自签名，所以父证书就是模板本身
	return createCertificate(template, template, config.PrivateKey.Public(), config.PrivateKey)
}

// GenerateServerCert 使用给定的 CA 签发一个服务器证书。
// caCertPEM 和 caKeyPEM 是 PEM 编码的 CA 证书和私钥。
func GenerateServerCert(caCertPEM, caKeyPEM []byte, opts ...CertOption) ([]byte, []byte, error) {
	// 为服务器证书设置默认的 KeyUsage 和 ExtKeyUsage
	defaultOpts := []CertOption{
		WithKeyUsage(x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment),
		WithExtKeyUsage(x509.ExtKeyUsageServerAuth),
	}
	opts = append(defaultOpts, opts...)

	return generateSignedCert(caCertPEM, caKeyPEM, false, opts...) // isCA = false
}

// GenerateClientCert 使用给定的 CA 签发一个客户端证书。
// caCertPEM 和 caKeyPEM 是 PEM 编码的 CA 证书和私钥。
func GenerateClientCert(caCertPEM, caKeyPEM []byte, opts ...CertOption) ([]byte, []byte, error) {
	// 为客户端证书设置默认的 KeyUsage 和 ExtKeyUsage
	defaultOpts := []CertOption{
		WithKeyUsage(x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment),
		WithExtKeyUsage(x509.ExtKeyUsageClientAuth),
	}
	opts = append(defaultOpts, opts...)

	return generateSignedCert(caCertPEM, caKeyPEM, false, opts...) // isCA = false
}

// -----------------------------------------------------------------------------
// Section: Internal Helper Functions
// -----------------------------------------------------------------------------

// newCertConfig 根据选项创建一个带有默认值的 CertConfig
func newCertConfig(opts ...CertOption) (*CertConfig, error) {
	// 设置默认值
	config := &CertConfig{
		NotBefore: time.Now().Add(-5 * time.Minute),
		NotAfter:  time.Now().AddDate(1, 0, 0), // 默认1年有效期
	}

	for _, opt := range opts {
		opt(config)
	}

	if config.CommonName == "" || config.Organization == "" {
		return nil, errors.New("CommonName and Organization fields are required")
	}

	// 如果未提供私钥，则生成一个
	if config.PrivateKey == nil {
		var err error
		config.PrivateKey, err = rsa.GenerateKey(cryptorand.Reader, 2048)
		if err != nil {
			return nil, fmt.Errorf("failed to generate private key: %w", err)
		}
	}

	return config, nil
}

// generateSignedCert 是一个通用的函数，用于创建由 CA 签名的证书。
func generateSignedCert(caCertPEM, caKeyPEM []byte, isCA bool, opts ...CertOption) ([]byte, []byte, error) {
	caCert, err := ParseCertificate(caCertPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse ca certificate: %w", err)
	}

	caKey, err := ParsePrivateKey(caKeyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse ca private key: %w", err)
	}

	config, err := newCertConfig(opts...)
	if err != nil {
		return nil, nil, err
	}

	template, err := createTemplate(config, isCA)
	if err != nil {
		return nil, nil, err
	}

	return createCertificate(template, caCert, config.PrivateKey.Public(), caKey)
}

func getDefaultedValue(inputValue, defaultValue string) string {
	if inputValue != "" {
		return inputValue
	}
	return defaultValue
}

// createTemplate 是改造后的核心函数
func createTemplate(config *CertConfig, isCA bool) (*x509.Certificate, error) {
	// 1. 强制校验必需字段
	if config.CommonName == "" || config.Organization == "" {
		return nil, errors.New("CommonName and Organization fields are required")
	}
	// 生成序列号 (这部分逻辑不变)
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := cryptorand.Int(cryptorand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}
	// 2. 使用辅助函数填充 Subject
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Country:            []string{getDefaultedValue(config.Country, config.Organization)},
			Province:           []string{getDefaultedValue(config.Province, config.Organization)},
			Locality:           []string{getDefaultedValue(config.Locality, config.Organization)},
			Organization:       []string{config.Organization}, // 必填字段，直接使用
			OrganizationalUnit: []string{getDefaultedValue(config.OrganizationalUnit, config.Organization)},
			CommonName:         config.CommonName, // 必填字段，直接使用
		},
		NotBefore:             config.NotBefore,
		NotAfter:              config.NotAfter,
		KeyUsage:              config.KeyUsage,
		ExtKeyUsage:           config.ExtKeyUsage,
		BasicConstraintsValid: true,
		IsCA:                  isCA,
		DNSNames:              config.AlternativeDNS,
		IPAddresses:           config.AlternativeIPs,
	}
	return template, nil
}

// createCertificate 执行实际的证书创建和编码。
func createCertificate(template, parent *x509.Certificate, pub interface{}, priv crypto.Signer) ([]byte, []byte, error) {
	// 创建证书的 DER 编码
	derBytes, err := x509.CreateCertificate(cryptorand.Reader, template, parent, pub, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// PEM 编码证书
	certPEM := new(bytes.Buffer)
	if err := pem.Encode(certPEM, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return nil, nil, fmt.Errorf("failed to encode certificate to PEM: %w", err)
	}

	// PEM 编码私钥 (使用 PKCS#8 格式, 更具通用性)
	keyPEM := new(bytes.Buffer)
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	if err := pem.Encode(keyPEM, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		return nil, nil, fmt.Errorf("failed to encode private key to PEM: %w", err)
	}

	return certPEM.Bytes(), keyPEM.Bytes(), nil
}

// -----------------------------------------------------------------------------
// Section: Parsing Utility Functions
// -----------------------------------------------------------------------------

// ParseCertificate 从 PEM 编码的字节中解析 x509.Certificate。
func ParseCertificate(pemBytes []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("failed to decode PEM block containing certificate")
	}
	return x509.ParseCertificate(block.Bytes)
}

// ParsePrivateKey 从 PEM 编码的字节中解析 crypto.Signer（私钥）。
// 它会尝试解析 PKCS#8 和 PKCS#1 格式的私钥。
func ParsePrivateKey(pemBytes []byte) (crypto.Signer, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block containing private key")
	}

	// 优先尝试 PKCS#8, 因为它更通用
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// 如果失败，尝试作为 PKCS#1 RSA 私钥解析（用于向后兼容）
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key as PKCS#8 or PKCS#1: %v", err)
		}
	}

	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("parsed key does not implement crypto.Signer interface")
	}
	return signer, nil
}
