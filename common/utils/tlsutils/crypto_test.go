package tlsutils

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestEncrypt(t *testing.T) {
	text := `

		return nil, nil, errors.Errorf("parse private key error: %s", err)
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


`

	test := assert.New(t)

	pri, pub, err := GeneratePrivateAndPublicKeyPEM()
	if err != nil {
		test.FailNow(err.Error())
		return
	}

	results, err := Encrypt([]byte(text), pub)
	if err != nil {
		test.FailNow(err.Error())
	}

	println(results)
	raw, err := Decrypt(results, pri)
	if err != nil {
		test.FailNow(err.Error())
		return
	}

	test.Equal(string(raw), text)
}
