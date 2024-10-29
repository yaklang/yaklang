package tlsutils

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
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

func TestExternalSHA256Sign(t *testing.T) {
	base64sign := `d6z/xMf9ytAEm1Fe3rEQXdYns45nmbnfRunN1cytqrEMkPZVbrneYwajcnKSWCO8e8zzy0vdAaoe0y/7vk7u6SL4iJBMynSlbdO7djc+QasUZjeoe/63qaGTLhOPdvcyvl5UIwa8J2UJMg6rxzAqq0F8hXUBwPAdqOVcnSlNUMjEgDBRLCOCUL2o5WJWwmEwF33cGejrOyeke1IQFVZ0XfRKf78XnaBahTRx4TPA5WsLUXuKVJvm+oxE1yJg+ySlmCowJjdqrRm7MZWlHyHjs+aUegwj9Nd2OonInKR1jymzXB9WUZOcBKn3aV+aSle1zb4FILbi5BQVbAGX7kvDSQ==`
	sign, _ := codec.DecodeBase64(base64sign)
	pubKey := `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAhUQvsgfH5Z6E4dG/yCb4rFAcV95DuFHYdq/Bw1k7aDW3IDBQ
FoiU66LVDNYEmNmF6EBU80eLM8v6rqFsM8SNgP8WePGSwwdbsQYgiSOPw9QgS0UG
RaD5aDjZLTF49BRTi05HMEVb3SjXNHVFC+Hoc5aJv/FSfurxz8gnAS103Z7oGwLT
/h1SU5ZahGfxQ73HB2swjwkPAtEpy81IlISPD7sxyFBityv3u/dyuNfuN6djAnA6
yksxV6q1zd6Zs00RoRoCZ9vUVjiVaqeSZs/F6Ty0Q5WEplWJOBwnsCLesA+0doGw
WHhnFfpREntMsjVn0IukJ6CznKXaoteJB+wJvwIDAQABAoIBAGSAV7fbRlVUhsIG
fKtlOIQ6piVd6ZRHpQdc5LN9x99/IuuTg9J6jlRmKGXVwQHEicftPCN8AO6/Ff48
nm0r/csalMgA5r1N/0gxZrgFqZX1k6UwGNrJ201OEfqTJLRt39Ne5TDyHaVb93AI
QFoFtFf3X0rxo1UzuckJGOE6drfq1vEaEjKeSqXnm2/2eR5BzunTh++0+Fe5mVso
rUHn5H55U3ac6Ww5Yw7MSoTRiTFg6AHq3dCR9NBcEM4hW2/BwwhOsC0C4ufM+6jM
eS1Lw4XQnvEww3XyA3EOG6TppR7Akj2SRec6SJXiA8Fy5Li+GHbH4BujSPZm6h+5
Ru59iAECgYEAzKfOXz8zk3Hu8JqM/AwVl88tUkPZxtgeYIiQ+OyWF4fQyWprkCF2
WFmyUPcE2d5Alu90qEvb+JCnxiIHsn4xm4yMw7bafecfeuSkckphxtvUQWYpjV/V
THGcW32rEjkRVcMQ/Atz5BoO5zNtdlQssMtS93AAa4siG0a5e75s38ECgYEAprNY
Paa2C8R4arP6El1kKLH40xa9s6wjfX2RoOa5T6Kc07CZ0h+D+/7S4drfhWAP5VYY
qaO6sVkdczk2aHx6FviH+tbc9MCQghZBRW/b4kljSStoi0FAYBBqWaYNAQv5D5Cx
Puaql/NUHgXjSmoegTWeU4ahK1f1cuo625rbSX8CgYBOIyuaFgldHDz8RCXb/cko
wwMsy5cUYmOGu92ODNZpeYNvw1/6EaybovOAEjAZ9s92UUqbDwuXZbOI5GlH7wKF
vy3nc6MMOvg79ZwLvvaB9GCf75+hyJspqp7mF57/QCasNeQAN2cyCfjysSHz8cN2
ZMryiiK+7MpC28fpxRTQgQKBgCt8TWCHxKV9MwxitrFju2UCSC6ImCPum7N4tiyL
A3xKpy5xuy6dGgj6iHhyaCyayorA0t4t392zqYMNQawwDIlBe/drZWhTc177/zrl
2y5EhqnnsPXip7Bnl9abAnlrbiUpUZNfCNFqoF7Uml4nIJ4EJrETRafQ4i5/+6qd
0uZxAoGBALa8jNhzmIHI28KM5F+3gyP96TSjWOn17oucWwxcBTWaXSaGWpt4lWca
H5rgOZwuv0y5Wc0doob8EZAUS0oidgLEJyB850ELlhme/lEO7AVFfY/rmlYwKccx
A6hp+zcbljG8JVldcWDuzUOORV7OTbDIL4sPi/ugBblDiHOz1TiJ
-----END RSA PRIVATE KEY-----`
	origin := []byte("a")
	results, err := PemSignSha256WithRSA([]byte(pubKey), origin)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, sign, results)
}

func TestExternalSHA256Verified(t *testing.T) {
	base64sign := `d6z/xMf9ytAEm1Fe3rEQXdYns45nmbnfRunN1cytqrEMkPZVbrneYwajcnKSWCO8e8zzy0vdAaoe0y/7vk7u6SL4iJBMynSlbdO7djc+QasUZjeoe/63qaGTLhOPdvcyvl5UIwa8J2UJMg6rxzAqq0F8hXUBwPAdqOVcnSlNUMjEgDBRLCOCUL2o5WJWwmEwF33cGejrOyeke1IQFVZ0XfRKf78XnaBahTRx4TPA5WsLUXuKVJvm+oxE1yJg+ySlmCowJjdqrRm7MZWlHyHjs+aUegwj9Nd2OonInKR1jymzXB9WUZOcBKn3aV+aSle1zb4FILbi5BQVbAGX7kvDSQ==`
	sign, _ := codec.DecodeBase64(base64sign)
	pubKey := `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAhUQvsgfH5Z6E4dG/yCb4
rFAcV95DuFHYdq/Bw1k7aDW3IDBQFoiU66LVDNYEmNmF6EBU80eLM8v6rqFsM8SN
gP8WePGSwwdbsQYgiSOPw9QgS0UGRaD5aDjZLTF49BRTi05HMEVb3SjXNHVFC+Ho
c5aJv/FSfurxz8gnAS103Z7oGwLT/h1SU5ZahGfxQ73HB2swjwkPAtEpy81IlISP
D7sxyFBityv3u/dyuNfuN6djAnA6yksxV6q1zd6Zs00RoRoCZ9vUVjiVaqeSZs/F
6Ty0Q5WEplWJOBwnsCLesA+0doGwWHhnFfpREntMsjVn0IukJ6CznKXaoteJB+wJ
vwIDAQAB
-----END PUBLIC KEY-----`
	origin := []byte("a")
	err := PemVerifySignSha256WithRSA([]byte(pubKey), origin, sign)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPemSignSha256WithRSA(t *testing.T) {
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
	sign, err := PemSignSha256WithRSA([]byte(priKey), []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	err = PemVerifySignSha256WithRSA([]byte(pubKey), []byte("hello"), sign)
	if err != nil {
		t.Fatal(err)
	}
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
