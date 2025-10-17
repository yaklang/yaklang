package tlsutils

import (
	"bytes"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/gmsm/sm2"
	x509gm "github.com/yaklang/yaklang/common/gmsm/x509"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	cryptorand "crypto/rand"
)

func GenerateSelfSignedCertKey(host string, alternateIPs []net.IP, alternateDNS []string) ([]byte, []byte, error) {
	// return GenerateSelfSignedCertKeyWithCommonName("Yakit MITM Root CA", host, alternateIPs, alternateDNS)
	return GenerateSelfSignedCertKeyWithCommonName("Yakit MITM Root CA", host, alternateIPs, alternateDNS)
}

var defaultTLSServerConfig *tls.Config
var ttlTLSServerConfig = utils.NewTTLCache[*tls.Config](time.Minute * 20)

func NewDefaultTLSServer(conn net.Conn) *tls.Conn {
	if defaultTLSServerConfig == nil {
		certRaw, key, _ := GenerateSelfSignedCertKey("hacking.io", nil, nil)
		if certRaw != nil && key != nil {
			serverCert, serverKey, _ := SignServerCrtNKeyWithParams(certRaw, key, "facades-server.io", time.Now().Add(time.Hour*24*365), false)
			if serverCert != nil && serverKey != nil {
				defaultTLSServerConfig, _ = GetX509ServerTlsConfig(certRaw, serverCert, serverKey)
			}
		}
	}

	if defaultTLSServerConfig != nil {
		return tls.Server(conn, defaultTLSServerConfig)
	} else {
		return tls.Server(conn, utils.NewDefaultTLSConfig())
	}
}

func ParsePEMCRL(ca []byte) ([]pkix.RevokedCertificate, error) {
	caCertBlock, _ := pem.Decode(ca)
	revokedCertList, err := x509.ParseCRL(caCertBlock.Bytes)
	if err != nil {
		return nil, errors.Errorf("parse crl error: %s", err)
	}
	return revokedCertList.TBSCertList.RevokedCertificates, nil
}

func ParsePEMCRLRaw(ca []byte) (*pkix.CertificateList, error) {
	caCertBlock, _ := pem.Decode(ca)
	return x509.ParseCRL(caCertBlock.Bytes)
}

func ParsePEMCert(crt []byte) (*x509.Certificate, error) {
	crtBlock, _ := pem.Decode(crt)
	return x509.ParseCertificate(crtBlock.Bytes)
}

func GenerateCRLWithExistedList(ca, key []byte, existedRevoked ...pkix.RevokedCertificate) ([]byte, error) {
	caCertBlock, _ := pem.Decode(ca)
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, errors.Errorf("parse ca error: %s", err)
	}

	caKeyBlock, _ := pem.Decode(key)

	// 首先尝试 PKCS1 格式
	caKey, err := x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		// 如果 PKCS1 失败，尝试 PKCS8 格式
		parsedKey, err2 := x509.ParsePKCS8PrivateKey(caKeyBlock.Bytes)
		if err2 != nil {
			return nil, errors.Errorf("parse private key error (tried both PKCS1 and PKCS8): PKCS1: %s, PKCS8: %s", err, err2)
		}

		// 检查是否为 RSA 私钥
		var ok bool
		caKey, ok = parsedKey.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.Errorf("parsed key is not an RSA private key, got %T", parsedKey)
		}
	}

	now := time.Now()

	//crlBytes, err := caCert.CreateCRL(
	//	cryptorand.Reader, caKey, existedRevoked, now.Add(- 24*time.Hour), now.Add(24*time.Hour*365),
	//)
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	sid, err := cryptorand.Int(cryptorand.Reader, serialNumberLimit)
	if err != nil {
		return nil, err
	}
	crlBytes, err := x509.CreateRevocationList(
		cryptorand.Reader, &x509.RevocationList{
			SignatureAlgorithm:  0,
			RevokedCertificates: existedRevoked,
			Number:              sid,
			ThisUpdate:          now,
			NextUpdate:          now.Add(24 * time.Hour),
			// ExtraExtensions:     nil,
		}, caCert, caKey,
	)
	if err != nil {
		return nil, utils.Errorf("create crl failed: %s", err)
	}

	crlPemBlock := &pem.Block{
		Type:  "X509 CRL",
		Bytes: crlBytes,
	}
	var crlBuffer bytes.Buffer
	err = pem.Encode(&crlBuffer, crlPemBlock)
	if err != nil {
		return nil, utils.Errorf("pem encode crl failed: %s", err)
	}
	return crlBuffer.Bytes(), nil
}

func GenerateCRL(ca, key []byte, revokingCert []byte, existedRevoked ...pkix.RevokedCertificate) ([]byte, error) {
	revokingCertBlock, _ := pem.Decode(revokingCert)
	revokingCertInstance, err := x509.ParseCertificate(revokingCertBlock.Bytes)
	if err != nil {
		return nil, errors.Errorf("parse revoking-cert error: %s", err)
	}

	now := time.Now()
	revokedCerts := append([]pkix.RevokedCertificate{
		{
			SerialNumber:   revokingCertInstance.SerialNumber,
			RevocationTime: now,
		},
	}, existedRevoked...)

	return GenerateCRLWithExistedList(ca, key, revokedCerts...)
}

func ParsePEMCertificateAndKey(ca, key []byte) (*x509.Certificate, *rsa.PrivateKey, error) {
	caCertBlock, _ := pem.Decode(ca)
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, nil, errors.Errorf("parse ca error: %s", err)
	}

	caKeyBlock, _ := pem.Decode(key)

	// 首先尝试 PKCS1 格式
	caKey, err := x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		// 如果 PKCS1 失败，尝试 PKCS8 格式
		parsedKey, err2 := x509.ParsePKCS8PrivateKey(caKeyBlock.Bytes)
		if err2 != nil {
			return nil, nil, errors.Errorf("parse private key error (tried both PKCS1 and PKCS8): PKCS1: %s, PKCS8: %s", err, err2)
		}

		// 检查是否为 RSA 私钥
		var ok bool
		caKey, ok = parsedKey.(*rsa.PrivateKey)
		if !ok {
			return nil, nil, errors.Errorf("parsed key is not an RSA private key, got %T", parsedKey)
		}
	}

	return caCert, caKey, nil
}

func ParsePEMCertificateAndKeyForGM(ca, key []byte) (*x509gm.Certificate, *sm2.PrivateKey, error) {
	caCertBlock, _ := pem.Decode(ca)
	caCert, err := x509gm.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, nil, errors.Errorf("parse ca error: %s", err)
	}

	caKeyBlock, _ := pem.Decode(key)
	// 尝试 PKCS8 格式
	caKey, err := x509gm.ParsePKCS8UnecryptedPrivateKey(caKeyBlock.Bytes)
	if err == nil {
		return caCert, caKey, nil
	}
	caKey, err = x509gm.ParseSm2PrivateKey(caKeyBlock.Bytes)
	if err == nil {
		return caCert, caKey, nil
	}
	log.Errorf("all strategry failed while trying to unmarshal GM private key asn1 data")
	return nil, nil, err
}

func ParsePEMCertificate(ca []byte) (*x509.Certificate, error) {
	caCertBlock, _ := pem.Decode(ca)
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, errors.Errorf("parse ca error: %s", err)
	}
	return caCert, nil
}

func ParseGMPEMCertificate(ca []byte) (*x509gm.Certificate, error) {
	caCertBlock, _ := pem.Decode(ca)
	caCert, err := x509gm.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, errors.Errorf("parse gm ca error: %s", err)
	}
	return caCert, nil
}

func GenerateSelfSignedCertKeyWithCommonName(commonName, host string, alternateIPs []net.IP, alternateDNS []string) ([]byte, []byte, error) {
	// 默认使用commonName作为organization
	return GenerateSelfSignedCertKeyWithCommonNameWithPrivateKeyWithOrg(commonName, commonName, host, alternateIPs, alternateDNS, nil)
}

func init() {
	utils.RegisterDefaultTLSConfigGenerator(func() (*tls.Config, *gmtls.Config, *gmtls.Config, *tls.Config, *gmtls.Config, []byte, []byte) {
		ca, key, _ := GenerateSelfSignedCertKeyWithCommonName("test", "127.0.0.1", nil, nil)
		sCa, sKey, _ := SignServerCrtNKey(ca, key)
		cCa, cKey, _ := SignClientCrtNKey(sCa, sKey)
		stls, _ := GetX509ServerTlsConfig(ca, sCa, sKey)
		mstls, _ := GetX509MutualAuthServerTlsConfig(ca, sCa, sKey)
		gmtlsConfig, _ := GetX509GMServerTlsConfigWithAuth(ca, sCa, sKey, false)
		onlyGmtlsTestConfig, _ := GetX509GMServerTlsConfigWithOnly(ca, sCa, sKey, false)
		mgmtlsConfig, _ := GetX509GMServerTlsConfigWithAuth(ca, sCa, sKey, true)
		return stls, gmtlsConfig, onlyGmtlsTestConfig, mstls, mgmtlsConfig, cCa, cKey
	})
}

func GenerateSelfSignedCertKeyWithCommonNameWithPrivateKeyWithOrg(commonName, org, host string, alternateIPs []net.IP, alternateDNS []string, priv *rsa.PrivateKey) ([]byte, []byte, error) {
	return GenerateSelfSignedCertKeyWithCommonNameEx(commonName, org, host, alternateIPs, alternateDNS, priv, false)
}

func GenerateSelfSignedCertKeyWithCommonNameEx(commonName, org, host string, alternateIPs []net.IP, alternateDNS []string, priv *rsa.PrivateKey, auth bool) ([]byte, []byte, error) {
	var hosts []string
	if host != "" {
		hosts = append(hosts, host)
	}
	for _, i := range alternateDNS {
		if i != "" {
			hosts = append(hosts, i)
		}
	}
	for _, i := range alternateIPs {
		if i != nil {
			hosts = append(hosts, i.String())
		}
	}
	return SelfSignCACertificateAndPrivateKey(commonName, WithSelfSign_SignTo(hosts...), WithSelfSign_EnableAuth(auth), WithSelfSign_PrivateKey(priv), WithSelfSign_Organization(org))
}

// SignX509ServerCertAndKey 根据给定的CA证书和私钥，生成服务器证书和密钥，返回PEM格式的服务器证书和密钥与错误
// Example:
// ```
// ca, key, err = tls.GenerateRootCA("yaklang.io")
// cert, sKey, err = tls.SignX509ServerCertAndKey(ca, key)
// ```
func SignServerCrtNKey(ca []byte, key []byte) (cert []byte, sKey []byte, _ error) {
	return SignServerCrtNKeyWithParams(ca, key, "Server", time.Now().Add(time.Hour*24*365*99), true)
}

// SignServerCertAndKey 根据给定的CA证书和私钥，生成不包含认证的服务器证书和密钥，返回PEM格式的服务器证书和密钥与错误
// Example:
// ```
// ca, key, err = tls.GenerateRootCA("yaklang.io")
// cert, sKey, err = tls.SignServerCertAndKey(ca, key)
// ```
func SignServerCrtNKeyWithoutAuth(ca []byte, key []byte) (cert []byte, sKey []byte, _ error) {
	return SignServerCrtNKeyWithParams(ca, key, "Server", time.Now().Add(time.Hour*24*365*99), false)
}

func SignServerCrtNKeyEx(ca []byte, key []byte, commonName string, auth bool) (cert []byte, sKey []byte, _ error) {
	return SignServerCrtNKeyWithParams(ca, key, commonName, time.Now().Add(time.Hour*24*365*99), auth)
}

func SignClientCrtNKeyEx(ca []byte, key []byte, commonName string, auth bool) (cert []byte, sKey []byte, _ error) {
	return SignClientCrtNKeyWithParams(ca, key, commonName, time.Now().Add(time.Hour*24*365*99), auth)
}

func SignServerCrtNKeyWithParams(ca []byte, key []byte, cn string, notAfter time.Time, authClient bool) (cert []byte, sKey []byte, _ error) {
	sPriv, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	if err != nil {
		return nil, nil, errors.Errorf("generate priv key error: %s", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	sid, err := cryptorand.Int(cryptorand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: sid,
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotBefore: time.Unix(946656000, 0),
		NotAfter:  notAfter,

		KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	if !authClient {
		template.ExtKeyUsage = nil
	}

	caCertBlock, _ := pem.Decode(ca)
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, nil, errors.Errorf("parse ca error: %s", err)
	}

	caKeyBlock, _ := pem.Decode(key)

	// 首先尝试 PKCS1 格式
	caKey, err := x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		// 如果 PKCS1 失败，尝试 PKCS8 格式
		parsedKey, err2 := x509.ParsePKCS8PrivateKey(caKeyBlock.Bytes)
		if err2 != nil {
			return nil, nil, errors.Errorf("parse private key error (tried both PKCS1 and PKCS8): PKCS1: %s, PKCS8: %s", err, err2)
		}

		// 检查是否为 RSA 私钥
		var ok bool
		caKey, ok = parsedKey.(*rsa.PrivateKey)
		if !ok {
			return nil, nil, errors.Errorf("parsed key is not an RSA private key, got %T", parsedKey)
		}
	}

	sCrt, err := x509.CreateCertificate(cryptorand.Reader, &template, caCert, &sPriv.PublicKey, caKey)
	if err != nil {
		return nil, nil, errors.Errorf("create cert error: %s", err)
	}
	// Generate cert
	certBuffer := bytes.Buffer{}
	if err := pem.Encode(&certBuffer, &pem.Block{Type: "CERTIFICATE", Bytes: sCrt}); err != nil {
		return nil, nil, errors.Errorf("pem encode crt error: %s", err)
	}

	// Generate key
	keyBuffer := bytes.Buffer{}
	if err := pem.Encode(&keyBuffer, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(sPriv)}); err != nil {
		return nil, nil, errors.Errorf("pem encode priv key error: %s", err)
	}

	return certBuffer.Bytes(), keyBuffer.Bytes(), nil
}

// SignX509ClientCertAndKey 根据给定的CA证书和私钥，生成客户端证书和密钥，返回PEM格式的客户端证书和密钥与错误
// Example:
// ```
// ca, key, err = tls.GenerateRootCA("yaklang.io")
// cert, sKey, err = tls.SignX509ClientCertAndKey(ca, key)
// ```
func SignClientCrtNKey(ca, key []byte) ([]byte, []byte, error) {
	return SignClientCrtNKeyWithParams(ca, key, "Client", time.Now().Add(time.Hour*24*365*99), true)
}

// SignClientCertAndKey 根据给定的CA证书和私钥，生成不包含认证的客户端证书和密钥，返回PEM格式的客户端证书和密钥与错误
// Example:
// ```
// ca, key, err = tls.GenerateRootCA("yaklang.io")
// cert, sKey, err = tls.SignClientCertAndKey(ca, key)
// ```
func SignClientCrtNKeyWithoutAuth(ca, key []byte) ([]byte, []byte, error) {
	return SignClientCrtNKeyWithParams(ca, key, "Client", time.Now().Add(time.Hour*24*365*99), false)
}

// GenerateSM2KeyPair 生成SM2公私钥对，返回PEM格式公钥和私钥与错误
// Example:
// ```
// pub, pri, err := tls.GenerateSM2KeyPair()
// ```
func SM2GenerateKeyPair() ([]byte, []byte, error) {
	priKey, err := sm2.GenerateKey(cryptorand.Reader)
	if err != nil {
		return nil, nil, utils.Errorf("sm2 generate failed: %s", err)
	}

	pubKey := &priKey.PublicKey
	priKeyBytes, err := x509gm.MarshalSm2PrivateKey(priKey, nil)
	if err != nil {
		return nil, nil, utils.Errorf("marshal pkcs8 priKey failed: %s", err)
	}
	pemPriBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: priKeyBytes}
	var priResult bytes.Buffer
	err = pem.Encode(&priResult, pemPriBlock)
	if err != nil {
		return nil, nil, utils.Errorf("pem encode private key failed: %s", err)
	}

	pubKeyBytes, err := x509gm.MarshalSm2PublicKey(pubKey)
	if err != nil {
		return nil, nil, utils.Errorf("marshal pkix pubKey failed: %s", err)
	}

	pubKeyBlock := &pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyBytes}
	var pubResults bytes.Buffer
	err = pem.Encode(&pubResults, pubKeyBlock)
	if err != nil {
		return nil, nil, utils.Errorf("pem encode public key failed: %s", err)
	}

	return pubResults.Bytes(), priResult.Bytes(), nil
}

// GenerateRSAKeyPair 根据给定的bit大小生成RSA公私钥对，返回PEM格式公钥和私钥与错误
// Example:
// ```
// pub, pri, err := tls.GenerateRSAKeyPair(2048)
// ```
func RSAGenerateKeyPair(bitSize int) ([]byte, []byte, error) {
	p, err := rsa.GenerateKey(cryptorand.Reader, bitSize)
	if err != nil {
		return nil, nil, err
	}
	pubKey := &p.PublicKey

	priKeyBytes := x509.MarshalPKCS1PrivateKey(p)
	pemPriBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: priKeyBytes}
	var priResult bytes.Buffer
	err = pem.Encode(&priResult, pemPriBlock)
	if err != nil {
		return nil, nil, utils.Errorf("pem encode private key failed: %s", err)
	}

	pubKeyBytes, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return nil, nil, utils.Errorf("marshal pkix pubKey failed: %s", err)
	}

	pubKeyBlock := &pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyBytes}
	var pubResults bytes.Buffer
	err = pem.Encode(&pubResults, pubKeyBlock)
	if err != nil {
		return nil, nil, utils.Errorf("pem encode public key failed: %s", err)
	}

	return pubResults.Bytes(), priResult.Bytes(), nil
}

func SignClientCrtNKeyWithParams(ca, key []byte, cn string, notAfter time.Time, x509Auth bool) (cert []byte, skey []byte, _ error) {
	sPriv, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	if err != nil {
		return nil, nil, errors.Errorf("generate priv key error: %s", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	sid, err := cryptorand.Int(cryptorand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: sid,
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotBefore: time.Unix(946656000, 0),
		NotAfter:  notAfter,

		KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			// x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	if !x509Auth {
		template.ExtKeyUsage = nil
	}

	caCertBlock, _ := pem.Decode(ca)
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, nil, errors.Errorf("parse ca error: %s", err)
	}

	caKeyBlock, _ := pem.Decode(key)

	// 首先尝试 PKCS1 格式
	caKey, err := x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		// 如果 PKCS1 失败，尝试 PKCS8 格式
		parsedKey, err2 := x509.ParsePKCS8PrivateKey(caKeyBlock.Bytes)
		if err2 != nil {
			return nil, nil, errors.Errorf("parse private key error (tried both PKCS1 and PKCS8): PKCS1: %s, PKCS8: %s", err, err2)
		}

		// 检查是否为 RSA 私钥
		var ok bool
		caKey, ok = parsedKey.(*rsa.PrivateKey)
		if !ok {
			return nil, nil, errors.Errorf("parsed key is not an RSA private key, got %T", parsedKey)
		}
	}

	sCrt, err := x509.CreateCertificate(cryptorand.Reader, &template, caCert, &sPriv.PublicKey, caKey)
	if err != nil {
		return nil, nil, errors.Errorf("create cert error: %s", err)
	}

	// Generate cert
	certBuffer := bytes.Buffer{}
	if err := pem.Encode(&certBuffer, &pem.Block{Type: "CERTIFICATE", Bytes: sCrt}); err != nil {
		return nil, nil, errors.Errorf("pem encode crt error: %s", err)
	}

	// Generate key
	keyBuffer := bytes.Buffer{}
	if err := pem.Encode(&keyBuffer, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(sPriv)}); err != nil {
		return nil, nil, errors.Errorf("pem encode priv key error: %s", err)
	}

	return certBuffer.Bytes(), keyBuffer.Bytes(), nil
}

func GetX509MutualAuthServerTlsConfig(caPemRaw, serverCrt, keyPriv []byte) (*tls.Config, error) {
	return GetX509ServerTlsConfigWithAuth(caPemRaw, serverCrt, keyPriv, true)
}

func GetX509ServerTlsConfig(caPemRaw, serverCrt, keyPriv []byte) (*tls.Config, error) {
	return GetX509ServerTlsConfigWithAuth(caPemRaw, serverCrt, keyPriv, false)
}

func GetX509ServerTlsConfigWithAuth(caPemRaw, serverCrt, keyPriv []byte, auth bool) (*tls.Config, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPemRaw) {
		return nil, errors.New("append ca pem error")
	}

	serverPair, err := tls.X509KeyPair(serverCrt, keyPriv)
	if err != nil {
		return nil, errors.Errorf("cannot build server crt/key pair: %s", err)
	}

	config := tls.Config{
		Certificates: []tls.Certificate{serverPair},
		ClientCAs:    pool,
	}
	if auth {
		config.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return &config, nil
}

func ParseCertAndPriKeyAndPool(clientCrt, clientPriv []byte, caCrts ...[]byte) (tls.Certificate, *x509gm.CertPool, error) {
	pool := x509gm.NewCertPool()
	for _, crt := range caCrts {
		if !pool.AppendCertsFromPEM(crt) {
			log.Errorf("append ca pem error")
		}
	}

	pair, err := tls.X509KeyPair(clientCrt, clientPriv)
	if err != nil {
		return tls.Certificate{}, nil, errors.Errorf("cannot build client crt/key pair: %s", err)
	}
	return pair, pool, nil
}

func ParseCertAndPriKeyAndPoolForGM(clientCrt, clientPriv []byte, caCrts ...[]byte) (gmtls.Certificate, *x509gm.CertPool, error) {
	pool := x509gm.NewCertPool()
	for _, crt := range caCrts {
		if !pool.AppendCertsFromPEM(crt) {
			log.Errorf("append ca pem error for GM")
		}
	}

	pair, err := gmtls.X509KeyPair(clientCrt, clientPriv)
	if err != nil {
		return gmtls.Certificate{}, nil, errors.Errorf("cannot build client crt/key pair for GM: %s", err)
	}
	return pair, pool, nil
}

func GetX509GMMutualAuthClientTlsConfig(clientCrt, clientPriv []byte, caCrts ...[]byte) (*gmtls.Config, error) {
	pool := x509gm.NewCertPool()
	for _, crt := range caCrts {
		if !pool.AppendCertsFromPEM(crt) {
			log.Errorf("append ca pem error")
		}
	}

	pair, err := gmtls.X509KeyPair(clientCrt, clientPriv)
	if err != nil {
		return nil, errors.Errorf("cannot build client crt/key pair: %s", err)
	}

	config := gmtls.Config{
		InsecureSkipVerify: true,
		Certificates:       []gmtls.Certificate{pair},
		ClientCAs:          pool,
		GMSupport:          &gmtls.GMSupport{WorkMode: gmtls.ModeAutoSwitch},
	}

	return &config, nil
}

func GetX509MutualAuthClientTlsConfig(clientCrt, clientPriv []byte, caCrts ...[]byte) (*tls.Config, error) {
	pool := x509.NewCertPool()
	for _, crt := range caCrts {
		if !pool.AppendCertsFromPEM(crt) {
			log.Errorf("append ca pem error")
		}
	}

	pair, err := tls.X509KeyPair(clientCrt, clientPriv)
	if err != nil {
		return nil, errors.Errorf("cannot build client crt/key pair: %s", err)
	}

	config := tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{pair},
		ClientCAs:          pool,
	}

	return &config, nil
}

func GetX509MutualAuthGoClientTlsConfig(clientCrt, clientPriv []byte, caCrts ...[]byte) (*tls.Config, error) {
	pool := x509.NewCertPool()
	for _, crt := range caCrts {
		if !pool.AppendCertsFromPEM(crt) {
			log.Errorf("append ca pem error")
		}
	}

	pair, err := tls.X509KeyPair(clientCrt, clientPriv)
	if err != nil {
		return nil, errors.Errorf("cannot build client crt/key pair: %s", err)
	}

	config := tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{pair},
		ClientCAs:          pool,
	}

	return &config, nil
}
