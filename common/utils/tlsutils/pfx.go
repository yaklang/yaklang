package tlsutils

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/utils/tlsutils/go-pkcs12"
)

import (
	randCrypto "crypto/rand"
)

func BuildP12(certBytes, keyBytes []byte, password string, ca ...[]byte) ([]byte, error) {
	cert, key, err := ParsePEMCertificateAndKey(certBytes, keyBytes)
	if err != nil {
		return nil, err
	}

	var caCerts = make([]*x509.Certificate, 0, len(ca))
	for _, c := range ca {
		caCert, err := ParsePEMCertificate(c)
		if err != nil {
			return nil, err
		}
		caCerts = append(caCerts, caCert)
	}

	pfxData, err := pkcs12.Encode(randCrypto.Reader, key, cert, caCerts, password)
	if err != nil {
		return nil, err
	}
	return pfxData, nil
}

func LoadP12ToPEM(p12Data []byte, password string) (certBytes, keyBytes []byte, ca [][]byte, err error) {
	key, cert, caCerts, err := pkcs12.DecodeChain(p12Data, password)
	if err != nil {
		return nil, nil, nil, err
	}
	var keyBytesRaw []byte
	var keyTitle = "PRIVATE KEY"
	switch key.(type) {
	case *rsa.PrivateKey:
		keyTitle = "RSA " + keyTitle
		keyBytesRaw = x509.MarshalPKCS1PrivateKey(key.(*rsa.PrivateKey))
	case *ecdsa.PrivateKey:
		keyTitle = "EC " + keyTitle
		keyBytesRaw, err = x509.MarshalECPrivateKey(key.(*ecdsa.PrivateKey))
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "marshal ecdsa private key error")
		}
	default:
		keyBytesRaw, err = x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "marshal pkcs8 private key error")
		}
	}
	keyBytes = newPEMBlock(keyTitle, keyBytesRaw)
	certBytes = newPEMBlock("CERTIFICATE", cert.Raw)
	for _, c := range caCerts {
		ca = append(ca, newPEMBlock("CERTIFICATE", c.Raw))
	}
	return
}
