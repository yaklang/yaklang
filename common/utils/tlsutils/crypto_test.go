package tlsutils

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strings"
	"testing"
)

func TestEncrypt(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestGenericRSA(t *testing.T) {
	t.Parallel()
	pemPubKey := `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAg9J6N3sz8oL2TvFcOULN
63ziWtHzflEOUHs6b6LBEhH7cZCMvQSzz80smDDTxBRC6YGd93S6VRwz95RqW6du
r+WH03NkY6pCTEO2CYvdE2yDKU24VRLPk5oLCJ1OUNQ2xQ6MFhVe8ZPuEZUhEzxL
vGYzxpVL1UlH2H/3NQ0l9LyG6FIGd9EhHYRC/XK9enqq/LWNCrBdlcPhJw0kC++X
GVPkJShm5vGQd+peq6JVgE2jSIP1rwShWK+KUsdXZmk7a42SBiflxey1cQG0oVNW
W1+oCwH6ajJjZ+y6H5tDi+f+u4I0iwOJIDB/eC1x1ilQxPxBqzTjdZN3Umi2g3qu
/QIDAQAB
-----END PUBLIC KEY-----`
	pemPriKey := `-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQCD0no3ezPygvZO
8Vw5Qs3rfOJa0fN+UQ5QezpvosESEftxkIy9BLPPzSyYMNPEFELpgZ33dLpVHDP3
lGpbp26v5YfTc2RjqkJMQ7YJi90TbIMpTbhVEs+TmgsInU5Q1DbFDowWFV7xk+4R
lSETPEu8ZjPGlUvVSUfYf/c1DSX0vIboUgZ30SEdhEL9cr16eqr8tY0KsF2Vw+En
DSQL75cZU+QlKGbm8ZB36l6rolWATaNIg/WvBKFYr4pSx1dmaTtrjZIGJ+XF7LVx
AbShU1ZbX6gLAfpqMmNn7Lofm0OL5/67gjSLA4kgMH94LXHWKVDE/EGrNON1k3dS
aLaDeq79AgMBAAECggEAGkNX6C/apKlk60t5BUuC/TTPYCrOKU9625wBg3ZYowIE
J5cWAx6puG/3P4cS2dDzl9QkJcYSzZwl2mCuY/5StiazzfQCfzzPoqQm43YDILiQ
1SzP2ds6kfyx0BCPJtlw5AfG7yto1BaV6tjcUxnDORDfpePezOqhrsen9+DbvAt9
QJ8tvirlRzs8G4c5L5Es7I13JRT89YkzbIFR7hRrcN9xGq/4U31+iPo/rJJehSSi
2e2/FE8i8kK7SCGwNlwJywok912B4yR40+QTJGgA0/eW+W0fzZXzA4c48+XKL1qy
t9BredDud1XwgkaNAjmAy7b1dYPDWdEY5mt5zEzyvQKBgQDasxKka+2qzGkNJZOs
q31KmUlpicJloaTDUwDR9Xc7ljnmmxIZ8j/5TSeP8/dZwV1SkCSeWMnTe9fK8Iep
Z1LzPVUVoGWG/jkBJ07bIf+m0qsL8GqJirRzVh3aK4eaprd9kF0RZSLwG+v3JIs8
BGyTdrQsozrZ3r9Oo20YQVQ/DwKBgQCaTiOiA+BqGX4eH8zvdlencnPtp2UEeMZh
Bmb4yuuI1hPutuffXfGu4ac1Zj3uXUHCmjTGkAFfRm3h37/g+NPkUuzJJXX1mVNw
DPrN4Yg+5WprqAKazAwcd/PXy3oj9iGzSrpqDzlZn8b6AALsOLMhyLVYSo40iQll
S9OE2lvxMwKBgQDWgGLdf7o5FopGyb9g0UZvH4+ggux3MCbhKQ0Z4W8Ts5GQvDHx
3ueeRm1yRLAriXtV2mkAIke6NLJ/qpD0t5HlXxePwaUy1S/mEL7IMT2FSwVXDXQA
+Vlp8mIPNTiol7JK5ohR4md1J322BlLGB/TSYc/wJB05yb7Li4EaFCFkQwKBgGMC
qJKY8jKiUO57cUBmKzBinEhuFL+dz40KUqBpdGDFHN0buAT3ftC8MlJtXGfKpxt7
X0nZtUexJWi97Z0pjK0BGLaottv0mjlX2saoZIgXJQYXNDSnoU3TGj/pbGIO2Oj2
lk7fnekIQODBiR6R8z9GTjZtAHptQ/4ffYXNpxlJAoGAe7aJgtwy0rcALZ39XOnC
UaK7Wv7BJwlQyO08aKcgR9IrVXxpgmeFYXY5jg7qOE50ekUnHqsAneq1Rz+z6Bmk
YGavRBwSFX1E+SjurtbCrFPT9KyGmJs6SQst8LRenetipAblazcEZ0LEstTKmWjI
btVhZFTAOegonuyb8ybhGQg=
-----END PRIVATE KEY-----`
	b64PemPubKey := `LS0tLS1CRUdJTiBQVUJMSUMgS0VZLS0tLS0KTUlJQklqQU5CZ2txaGtpRzl3MEJBUUVGQUFPQ0FROEFNSUlCQ2dLQ0FRRUFnOUo2TjNzejhvTDJUdkZjT1VMTgo2M3ppV3RIemZsRU9VSHM2YjZMQkVoSDdjWkNNdlFTeno4MHNtRERUeEJSQzZZR2Q5M1M2VlJ3ejk1UnFXNmR1CnIrV0gwM05rWTZwQ1RFTzJDWXZkRTJ5REtVMjRWUkxQazVvTENKMU9VTlEyeFE2TUZoVmU4WlB1RVpVaEV6eEwKdkdZenhwVkwxVWxIMkgvM05RMGw5THlHNkZJR2Q5RWhIWVJDL1hLOWVucXEvTFdOQ3JCZGxjUGhKdzBrQysrWApHVlBrSlNobTV2R1FkK3BlcTZKVmdFMmpTSVAxcndTaFdLK0tVc2RYWm1rN2E0MlNCaWZseGV5MWNRRzBvVk5XClcxK29Dd0g2YWpKaloreTZINXREaStmK3U0STBpd09KSURCL2VDMXgxaWxReFB4QnF6VGpkWk4zVW1pMmczcXUKL1FJREFRQUIKLS0tLS1FTkQgUFVCTElDIEtFWS0tLS0t`
	b64PemPriKey := `LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tCk1JSUV2UUlCQURBTkJna3Foa2lHOXcwQkFRRUZBQVNDQktjd2dnU2pBZ0VBQW9JQkFRQ0Qwbm8zZXpQeWd2Wk8KOFZ3NVFzM3JmT0phMGZOK1VRNVFlenB2b3NFU0VmdHhrSXk5QkxQUHpTeVlNTlBFRkVMcGdaMzNkTHBWSERQMwpsR3BicDI2djVZZlRjMlJqcWtKTVE3WUppOTBUYklNcFRiaFZFcytUbWdzSW5VNVExRGJGRG93V0ZWN3hrKzRSCmxTRVRQRXU4WmpQR2xVdlZTVWZZZi9jMURTWDB2SWJvVWdaMzBTRWRoRUw5Y3IxNmVxcjh0WTBLc0YyVncrRW4KRFNRTDc1Y1pVK1FsS0dibThaQjM2bDZyb2xXQVRhTklnL1d2QktGWXI0cFN4MWRtYVR0cmpaSUdKK1hGN0xWeApBYlNoVTFaYlg2Z0xBZnBxTW1ObjdMb2ZtME9MNS82N2dqU0xBNGtnTUg5NExYSFdLVkRFL0VHck5PTjFrM2RTCmFMYURlcTc5QWdNQkFBRUNnZ0VBR2tOWDZDL2FwS2xrNjB0NUJVdUMvVFRQWUNyT0tVOTYyNXdCZzNaWW93SUUKSjVjV0F4NnB1Ry8zUDRjUzJkRHpsOVFrSmNZU3pad2wybUN1WS81U3RpYXp6ZlFDZnp6UG9xUW00M1lESUxpUQoxU3pQMmRzNmtmeXgwQkNQSnRsdzVBZkc3eXRvMUJhVjZ0amNVeG5ET1JEZnBlUGV6T3FocnNlbjkrRGJ2QXQ5ClFKOHR2aXJsUnpzOEc0YzVMNUVzN0kxM0pSVDg5WWt6YklGUjdoUnJjTjl4R3EvNFUzMStpUG8vckpKZWhTU2kKMmUyL0ZFOGk4a0s3U0NHd05sd0p5d29rOTEyQjR5UjQwK1FUSkdnQTAvZVcrVzBmelpYekE0YzQ4K1hLTDFxeQp0OUJyZWREdWQxWHdna2FOQWptQXk3YjFkWVBEV2RFWTVtdDV6RXp5dlFLQmdRRGFzeEtrYSsycXpHa05KWk9zCnEzMUttVWxwaWNKbG9hVERVd0RSOVhjN2xqbm1teElaOGovNVRTZVA4L2Rad1YxU2tDU2VXTW5UZTlmSzhJZXAKWjFMelBWVVZvR1dHL2prQkowN2JJZittMHFzTDhHcUppclJ6VmgzYUs0ZWFwcmQ5a0YwUlpTTHdHK3YzSklzOApCR3lUZHJRc296clozcjlPbzIwWVFWUS9Ed0tCZ1FDYVRpT2lBK0JxR1g0ZUg4enZkbGVuY25QdHAyVUVlTVpoCkJtYjR5dXVJMWhQdXR1ZmZYZkd1NGFjMVpqM3VYVUhDbWpUR2tBRmZSbTNoMzcvZytOUGtVdXpKSlhYMW1WTncKRFByTjRZZys1V3BycUFLYXpBd2NkL1BYeTNvajlpR3pTcnBxRHpsWm44YjZBQUxzT0xNaHlMVllTbzQwaVFsbApTOU9FMmx2eE13S0JnUURXZ0dMZGY3bzVGb3BHeWI5ZzBVWnZINCtnZ3V4M01DYmhLUTBaNFc4VHM1R1F2REh4CjN1ZWVSbTF5UkxBcmlYdFYybWtBSWtlNk5MSi9xcEQwdDVIbFh4ZVB3YVV5MVMvbUVMN0lNVDJGU3dWWERYUUEKK1ZscDhtSVBOVGlvbDdKSzVvaFI0bWQxSjMyMkJsTEdCL1RTWWMvd0pCMDV5YjdMaTRFYUZDRmtRd0tCZ0dNQwpxSktZOGpLaVVPNTdjVUJtS3pCaW5FaHVGTCtkejQwS1VxQnBkR0RGSE4wYnVBVDNmdEM4TWxKdFhHZktweHQ3Clgwblp0VWV4SldpOTdaMHBqSzBCR0xhb3R0djBtamxYMnNhb1pJZ1hKUVlYTkRTbm9VM1RHai9wYkdJTzJPajIKbGs3Zm5la0lRT0RCaVI2Ujh6OUdUalp0QUhwdFEvNGZmWVhOcHhsSkFvR0FlN2FKZ3R3eTByY0FMWjM5WE9uQwpVYUs3V3Y3Qkp3bFF5TzA4YUtjZ1I5SXJWWHhwZ21lRllYWTVqZzdxT0U1MGVrVW5IcXNBbmVxMVJ6K3o2Qm1rCllHYXZSQndTRlgxRStTanVydGJDckZQVDlLeUdtSnM2U1FzdDhMUmVuZXRpcEFibGF6Y0VaMExFc3RUS21XakkKYnRWaFpGVEFPZWdvbnV5Yjh5YmhHUWc9Ci0tLS0tRU5EIFBSSVZBVEUgS0VZLS0tLS0=`
	b64DerPubKey := `MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAg9J6N3sz8oL2TvFcOULN
63ziWtHzflEOUHs6b6LBEhH7cZCMvQSzz80smDDTxBRC6YGd93S6VRwz95RqW6du
r+WH03NkY6pCTEO2CYvdE2yDKU24VRLPk5oLCJ1OUNQ2xQ6MFhVe8ZPuEZUhEzxL
vGYzxpVL1UlH2H/3NQ0l9LyG6FIGd9EhHYRC/XK9enqq/LWNCrBdlcPhJw0kC++X
GVPkJShm5vGQd+peq6JVgE2jSIP1rwShWK+KUsdXZmk7a42SBiflxey1cQG0oVNW
W1+oCwH6ajJjZ+y6H5tDi+f+u4I0iwOJIDB/eC1x1ilQxPxBqzTjdZN3Umi2g3qu
/QIDAQAB`
	b64DerPrivKey := `MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQCD0no3ezPygvZO
8Vw5Qs3rfOJa0fN+UQ5QezpvosESEftxkIy9BLPPzSyYMNPEFELpgZ33dLpVHDP3
lGpbp26v5YfTc2RjqkJMQ7YJi90TbIMpTbhVEs+TmgsInU5Q1DbFDowWFV7xk+4R
lSETPEu8ZjPGlUvVSUfYf/c1DSX0vIboUgZ30SEdhEL9cr16eqr8tY0KsF2Vw+En
DSQL75cZU+QlKGbm8ZB36l6rolWATaNIg/WvBKFYr4pSx1dmaTtrjZIGJ+XF7LVx
AbShU1ZbX6gLAfpqMmNn7Lofm0OL5/67gjSLA4kgMH94LXHWKVDE/EGrNON1k3dS
aLaDeq79AgMBAAECggEAGkNX6C/apKlk60t5BUuC/TTPYCrOKU9625wBg3ZYowIE
J5cWAx6puG/3P4cS2dDzl9QkJcYSzZwl2mCuY/5StiazzfQCfzzPoqQm43YDILiQ
1SzP2ds6kfyx0BCPJtlw5AfG7yto1BaV6tjcUxnDORDfpePezOqhrsen9+DbvAt9
QJ8tvirlRzs8G4c5L5Es7I13JRT89YkzbIFR7hRrcN9xGq/4U31+iPo/rJJehSSi
2e2/FE8i8kK7SCGwNlwJywok912B4yR40+QTJGgA0/eW+W0fzZXzA4c48+XKL1qy
t9BredDud1XwgkaNAjmAy7b1dYPDWdEY5mt5zEzyvQKBgQDasxKka+2qzGkNJZOs
q31KmUlpicJloaTDUwDR9Xc7ljnmmxIZ8j/5TSeP8/dZwV1SkCSeWMnTe9fK8Iep
Z1LzPVUVoGWG/jkBJ07bIf+m0qsL8GqJirRzVh3aK4eaprd9kF0RZSLwG+v3JIs8
BGyTdrQsozrZ3r9Oo20YQVQ/DwKBgQCaTiOiA+BqGX4eH8zvdlencnPtp2UEeMZh
Bmb4yuuI1hPutuffXfGu4ac1Zj3uXUHCmjTGkAFfRm3h37/g+NPkUuzJJXX1mVNw
DPrN4Yg+5WprqAKazAwcd/PXy3oj9iGzSrpqDzlZn8b6AALsOLMhyLVYSo40iQll
S9OE2lvxMwKBgQDWgGLdf7o5FopGyb9g0UZvH4+ggux3MCbhKQ0Z4W8Ts5GQvDHx
3ueeRm1yRLAriXtV2mkAIke6NLJ/qpD0t5HlXxePwaUy1S/mEL7IMT2FSwVXDXQA
+Vlp8mIPNTiol7JK5ohR4md1J322BlLGB/TSYc/wJB05yb7Li4EaFCFkQwKBgGMC
qJKY8jKiUO57cUBmKzBinEhuFL+dz40KUqBpdGDFHN0buAT3ftC8MlJtXGfKpxt7
X0nZtUexJWi97Z0pjK0BGLaottv0mjlX2saoZIgXJQYXNDSnoU3TGj/pbGIO2Oj2
lk7fnekIQODBiR6R8z9GTjZtAHptQ/4ffYXNpxlJAoGAe7aJgtwy0rcALZ39XOnC
UaK7Wv7BJwlQyO08aKcgR9IrVXxpgmeFYXY5jg7qOE50ekUnHqsAneq1Rz+z6Bmk
YGavRBwSFX1E+SjurtbCrFPT9KyGmJs6SQst8LRenetipAblazcEZ0LEstTKmWjI
btVhZFTAOegonuyb8ybhGQg=`

	rawDerPubKey, err := codec.DecodeBase64(strings.ReplaceAll(strings.TrimSpace(b64DerPubKey), "\n", ""))
	require.NoError(t, err)
	rawDerPrivKey, err := codec.DecodeBase64(strings.ReplaceAll(strings.TrimSpace(b64DerPrivKey), "\n", ""))
	require.NoError(t, err)

	mockInputPubKey := []any{pemPubKey, b64PemPubKey, b64DerPubKey, rawDerPubKey}
	mockInputPrivKey := []any{pemPriKey, b64PemPriKey, b64DerPrivKey, rawDerPrivKey}

	var pubBytes []byte
	var privBytes []byte
	for _, pub := range mockInputPubKey {
		for _, priv := range mockInputPrivKey {
			if val, ok := pub.(string); ok {
				pubBytes = []byte(val)
			}
			if val, ok := pub.([]byte); ok {
				pubBytes = val
			}
			if val, ok := priv.(string); ok {
				privBytes = []byte(val)
			}
			if val, ok := priv.([]byte); ok {
				privBytes = val
			}
			rsaCiphertext, err := PkcsOAEPEncrypt(pubBytes, []byte("go0p"))
			require.NoError(t, err)
			rsaPlaintext, err := PkcsOAEPDecrypt(privBytes, rsaCiphertext)
			require.NoError(t, err)
			require.Equal(t, []byte("go0p"), rsaPlaintext)

			rsaCiphertext, err = Pkcs1v15Encrypt(pubBytes, []byte("go0p"))
			require.NoError(t, err)
			rsaPlaintext, err = Pkcs1v15Decrypt(privBytes, rsaCiphertext)
			require.NoError(t, err)
			require.Equal(t, []byte("go0p"), rsaPlaintext)
		}
	}
}
