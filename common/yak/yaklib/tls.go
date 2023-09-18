package yaklib

import (
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"time"
)

func rsaWithBitSize(i int) func() ([]byte, []byte, error) {
	return func() ([]byte, []byte, error) {
		return tlsutils.RSAGenerateKeyPair(i)
	}
}

var TlsExports = map[string]interface{}{
	"GenerateRSAKeyPair":     tlsutils.RSAGenerateKeyPair,
	"GenerateRSA1024KeyPair": rsaWithBitSize(1024),
	"GenerateRSA2048KeyPair": rsaWithBitSize(2048),
	"GenerateRSA4096KeyPair": rsaWithBitSize(4096),
	"GenerateSM2KeyPair":     tlsutils.SM2GenerateKeyPair,
	"GenerateRootCA": func(commonName string) (ca []byte, key []byte, _ error) {
		return tlsutils.GenerateSelfSignedCertKeyWithCommonName(commonName, "", nil, nil)
	},
	"SignX509ServerCertAndKey": tlsutils.SignServerCrtNKey,
	"SignX509ClientCertAndKey": tlsutils.SignClientCrtNKey,
	"SignServerCertAndKey": func(ca []byte, key []byte) (cert []byte, sKey []byte, _ error) {
		return tlsutils.SignServerCrtNKeyWithParams(ca, key, "Server", time.Now().Add(time.Hour*24*365*99), false)
	},
	"SignClientCertAndKey": func(ca []byte, key []byte) (cert []byte, sKey []byte, _ error) {
		return tlsutils.SignClientCrtNKeyWithParams(ca, key, "Server", time.Now().Add(time.Hour*24*365*99), false)
	},
	"Inspect":             netx.TLSInspect,
	"EncryptWithPkcs1v15": tlsutils.PemPkcs1v15Encrypt,
	"DecryptWithPkcs1v15": tlsutils.PemPkcs1v15Decrypt,
}
