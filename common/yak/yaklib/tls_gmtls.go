package yaklib

import "github.com/yaklang/yaklang/common/gmsm/gmtls"

// 国密 TLCP/GMSSL ClientHello 套件 ID（与 common/gmsm/gmtls 一致）。
// 在 poc 中与 poc.gmTLSCipherSuite(tls.GMTLS_*) 配合使用；未指定时由引擎默认四套单次握手，或 poc.gmTLSCompatMode 分轮兼容。
const (
	// GMTLS_ECC_SM4_CBC_SM3 静态 ECC + SM4-CBC + SM3（0xe013）。
	// 密钥协商使用证书中的 SM2，无需 ECDHE ServerKeyExchange；部分国密网关优先或仅支持此套件。
	GMTLS_ECC_SM4_CBC_SM3 = int(gmtls.GMTLS_ECC_SM4_CBC_SM3)

	// GMTLS_ECC_SM4_GCM_SM3 静态 ECC + SM4-GCM + SM3（0xe053）。与上一套件同一类密钥协商，仅分组模式为 GCM。
	GMTLS_ECC_SM4_GCM_SM3 = int(gmtls.GMTLS_ECC_SM4_GCM_SM3)

	// GMTLS_ECDHE_SM4_CBC_SM3 临时 ECDHE + SM4-CBC + SM3（0xe011）。需要完整的 ECDHE ServerKeyExchange 握手。
	GMTLS_ECDHE_SM4_CBC_SM3 = int(gmtls.GMTLS_ECDHE_SM4_CBC_SM3)

	// GMTLS_ECDHE_SM4_GCM_SM3 临时 ECDHE + SM4-GCM + SM3（0xe051）。
	GMTLS_ECDHE_SM4_GCM_SM3 = int(gmtls.GMTLS_ECDHE_SM4_GCM_SM3)
)
