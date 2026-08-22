package netx

import (
	utls "github.com/refraction-networking/utls"
)

// ML-DSA 签名算法码点
const (
	signatureMLDSA44 utls.SignatureScheme = 0x0904
	signatureMLDSA65 utls.SignatureScheme = 0x0905
	signatureMLDSA87 utls.SignatureScheme = 0x0906
)

// ChromeClientHelloSpec 返回对齐 Chrome 151 的 uTLS ClientHelloSpec，
// 其 JA4 为 t13d1516h2_8daaf6152771_806a8c22fdea。
// 每次调用都会重新生成 GREASE 取值并打乱扩展顺序。
func ChromeClientHelloSpec() (*utls.ClientHelloSpec, error) {
	spec, err := utls.UTLSIdToSpec(utls.HelloChrome_133)
	if err != nil {
		return nil, err
	}
	for _, ext := range spec.Extensions {
		sigExt, ok := ext.(*utls.SignatureAlgorithmsExtension)
		if !ok {
			continue
		}
		sigExt.SupportedSignatureAlgorithms = append(
			[]utls.SignatureScheme{signatureMLDSA44, signatureMLDSA65, signatureMLDSA87},
			sigExt.SupportedSignatureAlgorithms...,
		)
	}
	return &spec, nil
}
