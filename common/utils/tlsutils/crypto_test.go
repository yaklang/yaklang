package tlsutils

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestPemPkcsOAEPEncryptWithHash(t *testing.T) {
	pubKey := `
-----BEGIN RSA PUBLIC KEY-----
MIIBCgKCAQEAs1pvFYNQpPSPbshg6F7ZaR31s14iwKacmG5vvQp8Xq38tBJaC8WP
Pcuv0/66hFe5zfE5pl+yUK37mjBaRMmcO2C0ommeVcAm3yKZ3x6FHVaj/YM6z+F3
0aNHsDmR1Ihf9LqFWr8mXdMBnLay+Uzfz1s+kW1eDwEF8xsD5gmzyS2EtOVQIgVm
geIses5HOQ0aGIlqCpoefPfhPEOQs92zCEztXno1o+eBFjScVzA+M9jwyOFT+dz6
6L973Ns26pj4E1zWfgZviH2XHUq7/POlokri+BGAE3IDZwMgZjYBJTs4R9mQJsrD
wheJGTb5CZB35OXZDx2cxtq8nxY4L71q7QIDAQAB
-----END RSA PUBLIC KEY-----`
	priKey := `
-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAs1pvFYNQpPSPbshg6F7ZaR31s14iwKacmG5vvQp8Xq38tBJa
C8WPPcuv0/66hFe5zfE5pl+yUK37mjBaRMmcO2C0ommeVcAm3yKZ3x6FHVaj/YM6
z+F30aNHsDmR1Ihf9LqFWr8mXdMBnLay+Uzfz1s+kW1eDwEF8xsD5gmzyS2EtOVQ
IgVmgeIses5HOQ0aGIlqCpoefPfhPEOQs92zCEztXno1o+eBFjScVzA+M9jwyOFT
+dz66L973Ns26pj4E1zWfgZviH2XHUq7/POlokri+BGAE3IDZwMgZjYBJTs4R9mQ
JsrDwheJGTb5CZB35OXZDx2cxtq8nxY4L71q7QIDAQABAoIBAFjVFegF3k+VgeVR
Ag6VzAEwgZ2Rpozc+PrW2Ck9pFQQwPU/kbH66/OjizbpF+Cswq6qJ++rvloPkmrQ
QCWJ5gPS5iT7Qx0dyyMBtEy6hRv+6cKK2PpVpk8DHGLAYOZvlXdVWu+TdaFK/aVt
KEAqP0Ao5ViKXuf3jcbXPpsVeyLMvZo6Ncb3RaKx9AFkwv+bSyBUboM6hUzFRmef
nMTRBAfL9+pHJFFZbadFm2xljKhiP8sdlopgF8rEtLBFeg74FTF52/Z4ydODkZzn
JHRUEuoS2ZYM6dd3kBiGIFgQnDFXAq/pDxV3YX3NUZUGsJC7oDhJ39FvO8rTckT0
GtHyitECgYEAtYmtAI2K4M1jLQerJDPI1A7pFKbmfmijqSkdsrQQTVbxpclrE1ZD
4bESJVDbU24eTLX2Vev2BpT/JuveRjmJani7F4FqcARb1tRrjfvXzNGeFZOsY8Du
4JiZGntmpeXydQafhsxUvqonvKttT1NF3uUtxY5lLqss9pBmYlpobocCgYEA/Otf
DMlnx2akfEDAXI77EGlXiIdvizOCGBxwMAxgFPV3K0RvA9NRzIBFPU9y6NUyCwft
yXdvoN0yD7BlLt3G3mHjGV1rchc45boF8I5OjFRWFWfgfWtkMTjt4Xa2FeyTVqYN
J12gtecEd2R6uJM1zQ0kL21Llb7wI47fiRCAo+sCgYB6PM8qHSTThFjwfEZn5Rqo
d7XIey2fFpSFFjNyHj8P5KhoSrz301F4CgQ+7jgQ8IgkfS324yDRg8hfC9mqjZmT
AOJxzGnALZ8tg/E8NMU1nDwHKV2d+c6fmwEUzNzsfm6JEEGgwbuaevaw2vmKvXbB
xK3SZbSJ/ScUi1z1gwzoxwKBgQCBKMH1ibURw30kZvzVR7829lTZSDDSaY96OKui
He/DREeDNQNsdLJFOQwi7zvDY3yW3Ym1ZOUAxXUXRgGmGWPBlUOgZHDGZs2Lo5/8
5O+AAmGjtNSTuBAGgwgYJ8N9Fr93dH0rKUk1G7DQN+Pj9ml3OcrM3YfIBSYlQoUt
Pdwz2QKBgGgS4ZmRBGOddVy62zy92a6e8q6HC5dKyzVuzT+GboBaDMeKxJiaENWU
wj+bgQuXkumftd+a30BV5VpXh9M8tHkqw3emWXLPLcu9oyvJVWfD+Wm57i4O7mQR
sUT/btRaRcGKtGXKLpSU0513RZc1mrsAOwblzfKcSjT8hv4sztAC
-----END RSA PRIVATE KEY-----`
	rsaCiphertext, err := PemPkcsOAEPEncrypt([]byte(pubKey), []byte("hello"))
	require.NoError(t, err)
	rsaPlaintext, err := PemPkcsOAEPDecrypt([]byte(priKey), rsaCiphertext)
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), rsaPlaintext)

	rsaCiphertext, err = PemPkcs1v15Encrypt([]byte(pubKey), []byte("hello"))
	require.NoError(t, err)
	rsaPlaintext, err = PemPkcs1v15Decrypt([]byte(priKey), rsaCiphertext)
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), rsaPlaintext)

}
