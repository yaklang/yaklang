package yaklib

import (
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
)

// GenerateRSA1024KeyPair 生成 1024 位大小的 RSA 公私钥对
// 返回值:
//   - PEM 格式的公钥
//   - PEM 格式的私钥
//   - 错误信息，生成失败时返回非空
//
// Example:
// ```
// pub, pri, err := tls.GenerateRSA1024KeyPair()
// ```
func generateRSA1024KeyPair() ([]byte, []byte, error) {
	return tlsutils.RSAGenerateKeyPair(1024)
}

// GenerateRSA2048KeyPair 生成 2048 位大小的 RSA 公私钥对
// 返回值:
//   - PEM 格式的公钥
//   - PEM 格式的私钥
//   - 错误信息，生成失败时返回非空
//
// Example:
// ```
// pub, pri, err := tls.GenerateRSA2048KeyPair()
// ```
func generateRSA2048KeyPair() ([]byte, []byte, error) {
	return tlsutils.RSAGenerateKeyPair(2048)
}

// GenerateRSA4096KeyPair 生成 4096 位大小的 RSA 公私钥对
// 返回值:
//   - PEM 格式的公钥
//   - PEM 格式的私钥
//   - 错误信息，生成失败时返回非空
//
// Example:
// ```
// pub, pri, err := tls.GenerateRSA4096KeyPair()
// ```
func generateRSA4096KeyPair() ([]byte, []byte, error) {
	return tlsutils.RSAGenerateKeyPair(4096)
}

// GenerateRootCA 根据名字生成根证书和私钥
// 参数:
//   - commonName: 证书的通用名称（CN），同时用作组织名
//   - opts: 可选的证书选项，例如 tls.validity、tls.alternativeDNS
//
// 返回值:
//   - PEM 格式的根证书
//   - PEM 格式的私钥
//   - 错误信息，生成失败时返回非空
//
// Example:
// ```
// cert, key, err := tls.GenerateRootCA("yaklang.io")
// ```
func generateRootCA(commonName string, opts ...tlsutils.CertOption) (ca []byte, key []byte, err error) {
	return tlsutils.GenerateCA(append(opts, tlsutils.WithCommonName(commonName), tlsutils.WithOrganization(commonName))...)
}

var TlsExports = map[string]interface{}{
	"GenerateRSAKeyPair":       tlsutils.RSAGenerateKeyPair,
	"GenerateRSA1024KeyPair":   generateRSA1024KeyPair,
	"GenerateRSA2048KeyPair":   generateRSA2048KeyPair,
	"GenerateRSA4096KeyPair":   generateRSA4096KeyPair,
	"GenerateSM2KeyPair":       tlsutils.SM2GenerateKeyPair,
	"SignX509ServerCertAndKey": tlsutils.SignServerCrtNKey,
	"SignX509ClientCertAndKey": tlsutils.SignClientCrtNKey,
	"SignServerCertAndKey":     tlsutils.SignServerCrtNKeyWithoutAuth,
	"SignClientCertAndKey":     tlsutils.SignClientCrtNKeyWithoutAuth,
	"Inspect":                  netx.TLSInspect,
	"InspectForceHttp2":        netx.TLSInspectForceHttp2,
	"InspectForceHttp1_1":      netx.TLSInspectForceHttp1_1,
	"EncryptWithPkcs1v15":      tlsutils.Pkcs1v15Encrypt,
	"DecryptWithPkcs1v15":      tlsutils.Pkcs1v15Decrypt,

	/*
		证书生成
	*/
	"GenerateRootCA":     generateRootCA,
	"GenerateServerCert": tlsutils.GenerateServerCert,
	"GenerateClientCert": tlsutils.GenerateClientCert,

	// --- 主体选项 ---
	"commonName":   tlsutils.WithCommonName,
	"organization": tlsutils.WithOrganization,
	"country":      tlsutils.WithCountry,
	"locality":     tlsutils.WithLocality,
	"province":     tlsutils.WithProvince,

	// --- 替代名称选项 ---
	"alternativeIP":  tlsutils.WithAlternativeIPStrings,
	"alternativeDNS": tlsutils.WithAlternativeDNS,

	// --- 有效期选项 ---
	"validity":  tlsutils.WithValidity,
	"notBefore": tlsutils.WithNotBefore,
	"notAfter":  tlsutils.WithNotAfter,

	// --- 密钥材料选项 ---
	"privateKeyFromFile": tlsutils.WithPrivateKeyFromFile,
	"privateKeyFromRaw":  tlsutils.WithPrivateKeyFromBytes,

	// --- 国密 TLS 套件（供 poc.gmTLSCipherSuite / tls.GMTLS_* 使用，见 tls_gmtls.go）---
	"GMTLS_ECC_SM4_CBC_SM3":   GMTLS_ECC_SM4_CBC_SM3,
	"GMTLS_ECC_SM4_GCM_SM3":   GMTLS_ECC_SM4_GCM_SM3,
	"GMTLS_ECDHE_SM4_CBC_SM3": GMTLS_ECDHE_SM4_CBC_SM3,
	"GMTLS_ECDHE_SM4_GCM_SM3": GMTLS_ECDHE_SM4_GCM_SM3,
}
