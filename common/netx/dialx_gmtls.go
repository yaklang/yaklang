package netx

import (
	"net"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	utls "github.com/refraction-networking/utls"
)

// 国密四套件的密钥协商分两类：静态 ECC（证书 SM2）与 ECDHE（需 ServerKeyExchange）。
var (
	eccGMTLSCipherSuites = []uint16{
		gmtls.GMTLS_ECC_SM4_CBC_SM3, // 0xe013，Tongsuo: ECC-SM2-SM4-CBC-SM3
		gmtls.GMTLS_ECC_SM4_GCM_SM3, // 0xe053
	}
	ecdheGMTLSCipherSuites = []uint16{
		gmtls.GMTLS_ECDHE_SM4_CBC_SM3, // 0xe011
		gmtls.GMTLS_ECDHE_SM4_GCM_SM3, // 0xe051
	}
	allGMTLSCipherSuites = []uint16{
		gmtls.GMTLS_ECC_SM4_CBC_SM3,
		gmtls.GMTLS_ECC_SM4_GCM_SM3,
		gmtls.GMTLS_ECDHE_SM4_CBC_SM3,
		gmtls.GMTLS_ECDHE_SM4_GCM_SM3,
	}
)

// gmtlsCipherSuiteDialAttempts 返回国密握手时的套件重试顺序（最多三轮）：
// 1) 仅 ECC 静态  2) 仅 ECDHE  3) 四套全开（兼容只接受「全列表」协商的站点）。
func gmtlsCipherSuiteDialAttempts(base *gmtls.Config) [][]uint16 {
	if len(base.CipherSuites) > 0 {
		return [][]uint16{base.CipherSuites}
	}
	return [][]uint16{
		eccGMTLSCipherSuites,
		ecdheGMTLSCipherSuites,
		allGMTLSCipherSuites,
	}
}

func shouldRetryGMTLSWithOtherCipherSuites(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "missing ServerKeyExchange message") ||
		strings.Contains(msg, "SM2 verification failure") ||
		strings.Contains(msg, "processServerKeyExchange") ||
		strings.Contains(msg, "server chose an unconfigured cipher suite") ||
		strings.Contains(msg, "handshake failure") ||
		strings.Contains(msg, "unconfigured cipher suite")
}

func dialTLSWithGMTLSCipherFallback(
	target string,
	config *dialXConfig,
	baseConfig *gmtls.Config,
	sni string,
	tlsTimeout time.Duration,
	clientHelloSpec *utls.ClientHelloSpec,
) (net.Conn, error) {
	attempts := gmtlsCipherSuiteDialAttempts(baseConfig)
	var lastErr error

	for i, cipherSuites := range attempts {
		tempTlsConfig := baseConfig.Clone()
		tempTlsConfig.GMSupport = &gmtls.GMSupport{WorkMode: gmtls.ModeGMSSLOnly}
		tempTlsConfig.CipherSuites = cipherSuites

		if config.Debug {
			log.Infof("dial %v gmtls cipher attempt %d/%d: cipher suites %v", target, i+1, len(attempts), cipherSuites)
		}

		conn, err := dialPlainTCPConnWithRetry(target, config)
		if err != nil {
			return nil, err
		}

		tlsConn, err := UpgradeToTLSConnectionWithTimeout(conn, sni, tempTlsConfig, tlsTimeout, clientHelloSpec, config.TLSNextProto...)
		if err == nil {
			return tlsConn, nil
		}
		lastErr = err
		_ = conn.Close()

		// 错误与套件无关（如连接被拒）时不必再换套件重连
		if !shouldRetryGMTLSWithOtherCipherSuites(err) {
			break
		}
		if config.Debug {
			log.Infof("dial %v gmtls cipher attempt %d failed: %v, retrying with next cipher set", target, i+1, err)
		}
	}

	return nil, lastErr
}
