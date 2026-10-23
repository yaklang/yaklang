package tlsutils

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
	"sync"
)

// Publicly compromised EM OV TLS CA - G3C and cross-signed emSign Root TLS CA - G3.
// The P-384 signing key has the publicly known private scalar D = 4.
// This published key is test material, not a secret. Updated trust stores reject it.
// Resources are gzip-compressed PEM with no timestamp or filename metadata.
//
//go:embed certs/emudhra-chain.pem.gz
var emudhraCertificateChainGZIP []byte

//go:embed certs/emudhra-key.pem.gz
var emudhraPrivateKeyGZIP []byte

var loadEMudhraCAAndKey = sync.OnceValues(func() ([]byte, []byte) {
	decode := func(compressed []byte) []byte {
		reader, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			panic("invalid embedded eMudhra gzip resource: " + err.Error())
		}
		defer reader.Close()
		data, err := io.ReadAll(reader)
		if err != nil {
			panic("cannot decompress embedded eMudhra resource: " + err.Error())
		}
		return data
	}
	return decode(emudhraCertificateChainGZIP), decode(emudhraPrivateKeyGZIP)
})

// GetEMudhraCAAndKey returns copies of the publicly compromised CA chain and key.
// These are test fixtures; this function neither generates nor installs a root CA.
func GetEMudhraCAAndKey() ([]byte, []byte) {
	chain, key := loadEMudhraCAAndKey()
	return bytes.Clone(chain), bytes.Clone(key)
}

// GenerateEMudhraServerCert 使用公开的 eMudhra 中间 CA 签发测试服务器证书。
// 返回 PEM 完整证书链、新生成的服务器私钥和错误；不安装证书或启动代理。
// 已封禁该 CA 或要求证书透明度的客户端仍会拒绝此证书。
//
// Example:
// ```
// chain, key = tls.GenerateEMudhraServerCert(tls.commonName("example.test"), tls.organization("TLS Test"), tls.alternativeDNS("example.test"))~
// ```
func GenerateEMudhraServerCert(opts ...CertOption) ([]byte, []byte, error) {
	ca, caKey := GetEMudhraCAAndKey()
	cert, key, err := GenerateServerCert(ca, caKey, opts...)
	if err != nil {
		return nil, nil, err
	}
	return append(cert, ca...), key, nil
}
