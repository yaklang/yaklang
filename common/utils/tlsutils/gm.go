package tlsutils

import (
	"bytes"
	cryptoRand "crypto/rand"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/gmsm/sm2"

	cryptorand "crypto/rand"

	"github.com/yaklang/yaklang/common/gmsm/x509"
)

func GetX509GMServerTlsConfigWithAuth(ca, server, serverKey []byte, auth bool) (*gmtls.Config, error) {
	p := x509.NewCertPool()
	if !p.AppendCertsFromPEM(ca) {
		return nil, errors.New("append ca pem error")
	}

	signCert, err := gmtls.X509KeyPair(server, serverKey)
	if err != nil {
		return nil, err
	}

	encryptCert, err := gmtls.X509KeyPair(server, serverKey)
	if err != nil {
		return nil, err
	}

	config := gmtls.Config{
		GMSupport:    &gmtls.GMSupport{WorkMode: gmtls.ModeAutoSwitch},
		Certificates: []gmtls.Certificate{signCert, encryptCert},
		ClientCAs:    p,
	}
	if auth {
		config.ClientAuth = gmtls.RequireAndVerifyClientCert
	}

	return &config, nil
}

func GetX509GMServerTlsConfigWithOnly(ca, server, serverKey []byte, auth bool) (*gmtls.Config, error) {
	// 生成签名证书和密钥
	signCert, signKey, err := GenerateGMSelfSignedCertKey("SignCert")
	if err != nil {
		return nil, err
	}

	// 生成加密证书和密钥
	encipherCert, encipherKey, err := GenerateGMSelfSignedCertKey("EncipherCert")
	if err != nil {
		return nil, err
	}

	p := x509.NewCertPool()
	if !p.AppendCertsFromPEM(ca) {
		return nil, errors.New("append ca pem error")
	}

	// 创建签名证书配对
	signCertPair, err := gmtls.X509KeyPair(signCert, signKey)
	if err != nil {
		return nil, err
	}

	// 创建加密证书配对
	encipherCertPair, err := gmtls.X509KeyPair(encipherCert, encipherKey)
	if err != nil {
		return nil, err
	}

	_, _ = signCertPair, encipherCertPair
	//rsaKey, err := gmtls.X509KeyPair(server, serverKey)

	config := gmtls.Config{
		GMSupport:          &gmtls.GMSupport{},
		Certificates:       []gmtls.Certificate{signCertPair, encipherCertPair},
		ClientCAs:          p,
		InsecureSkipVerify: false,
	}
	if auth {
		config.ClientAuth = gmtls.RequireAndVerifyClientCert
	}
	return &config, nil
	//return gmtls.NewBasicAutoSwitchConfig(&signCertPair, &encipherCertPair, &rsaKey)

}
func SignGMServerCrtNKeyWithParams(ca []byte, privateKey []byte, cn string, notAfter time.Time, auth bool) ([]byte, []byte, error) {
	return signGMServerCrtNKeyWithParams(ca, privateKey, cn, notAfter, auth, true)
}

func SignGMClientCrtNKeyWithParams(ca []byte, privateKey []byte, cn string, notAfter time.Time, auth bool) ([]byte, []byte, error) {
	return signGMServerCrtNKeyWithParams(ca, privateKey, cn, notAfter, auth, false)
}

func signGMServerCrtNKeyWithParams(ca []byte, privateKey []byte, cn string, notAfter time.Time, auth bool, isServerAuth bool) ([]byte, []byte, error) {
	// 解析 ca 的 key
	var pkey *sm2.PrivateKey
	var err error
	if bytes.HasPrefix(privateKey, []byte("---")) {
		pkey, err = x509.ReadPrivateKeyFromPem(privateKey, nil)
	} else {
		pkey, err = x509.ReadPrivateKeyFromHex(string(privateKey))
	}
	if err != nil {
		return nil, nil, errors.Wrap(err, "read sm2.privateKey")
	}

	// 服务端证书生成 key
	sPriv, err := sm2.GenerateKey(cryptorand.Reader)
	if err != nil {
		return nil, nil, err
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

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  false,
		SignatureAlgorithm:    x509.SM2WithSM3,
	}

	if auth {
		if isServerAuth {
			template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		} else {
			template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
		}
	} else {
		template.ExtKeyUsage = nil
	}

	caCertBlock, _ := pem.Decode(ca)
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, nil, errors.Errorf("parse ca error: %s", err)
	}

	sCrt, err := x509.CreateCertificate(&template, caCert, sPriv.Public().(*sm2.PublicKey), pkey)
	if err != nil {
		return nil, nil, err
	}

	// Generate cert
	certBuffer := bytes.Buffer{}
	if err := pem.Encode(&certBuffer, &pem.Block{Type: "CERTIFICATE", Bytes: sCrt}); err != nil {
		return nil, nil, errors.Errorf("pem encode crt error: %s", err)
	}

	sPrivBytes, err := x509.WritePrivateKeyToPem(sPriv, nil)
	return certBuffer.Bytes(), sPrivBytes, nil
}

func generateGMSelfSignedCertKey(commonName string) ([]byte, []byte, error) {
	pkey, err := sm2.GenerateKey(cryptoRand.Reader)
	if err != nil {
		return nil, nil, errors.Wrap(err, "sm2.GenerateKey(cryptoRand.Reader)")
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	sid, err := cryptorand.Int(cryptorand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: sid,
		Subject: pkix.Name{
			Country:            []string{commonName},
			Province:           []string{commonName},
			Locality:           []string{commonName},
			Organization:       []string{commonName},
			OrganizationalUnit: []string{commonName},
			CommonName:         commonName,
		},
		NotBefore: time.Unix(946656000, 0),
		NotAfter:  time.Now().Add(time.Hour * 24 * 365 * 99),

		KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
		IsCA:                  true,
		SignatureAlgorithm:    x509.SM2WithSM3,
	}
	derBytes, err := x509.CreateCertificate(&template, &template, pkey.Public().(*sm2.PublicKey), pkey)
	if err != nil {
		return nil, nil, err
	}

	var certBuf bytes.Buffer
	if err := pem.Encode(&certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return nil, nil, err
	}

	pKeyBytes, err := x509.WritePrivateKeyToPem(pkey, nil)
	if err != nil {
		return nil, nil, err
	}
	return certBuf.Bytes(), pKeyBytes, nil
}

var certGenMutex sync.Mutex

func GenerateGMSelfSignedCertKey(commonName string) ([]byte, []byte, error) {
	certGenMutex.Lock()
	defer certGenMutex.Unlock()

	var ca, key []byte
	var err error
	for i := 0; i < 5; i++ {
		// Attempt to generate a self-signed certificate and key
		ca, key, err = generateGMSelfSignedCertKey(commonName)
		if err != nil {
			return nil, nil, errors.Wrap(err, "generateGMSelfSignedCertKey")
		}
		_, verifyErr := gmtls.X509KeyPair(ca, key)
		if verifyErr != nil {
			continue
		}
		return ca, key, nil
	}
	return nil, nil, errors.Wrap(err, "generateGMSelfSignedCertKey max retries exceeded")
}
