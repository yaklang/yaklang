package netx

import (
	"net"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	utls "github.com/refraction-networking/utls"
)

// 国密套件按密钥协商分两类：静态 ECC 与 ECDHE。CBC/GCM 为 SM4 不同模式，同族套件可放在同一轮 ClientHello。
var (
	eccGMTLSCipherSuitesCompat = []uint16{
		gmtls.GMTLS_ECC_SM4_CBC_SM3,
		gmtls.GMTLS_ECC_SM4_GCM_SM3,
	}
	ecdheGMTLSCipherSuitesCompat = []uint16{
		gmtls.GMTLS_ECDHE_SM4_CBC_SM3,
		gmtls.GMTLS_ECDHE_SM4_GCM_SM3,
	}
	allGMTLSCipherSuites = []uint16{
		gmtls.GMTLS_ECC_SM4_CBC_SM3,
		gmtls.GMTLS_ECC_SM4_GCM_SM3,
		gmtls.GMTLS_ECDHE_SM4_CBC_SM3,
		gmtls.GMTLS_ECDHE_SM4_GCM_SM3,
	}
)

// gmtlsCipherSuiteDialAttempts 返回国密握手每轮 ClientHello 的套件列表。
// - 已设置 CipherSuites：仅一轮（用户指定）
// - 默认：一轮、四套全开（与 gmtls 默认 getCipherSuites 一致）
// - 兼容模式：三轮 ECC×2 → ECDHE×2 → 四套（部分站点需避免首轮同时提供 ECDHE）
func gmtlsCipherSuiteDialAttempts(base *gmtls.Config, compatMode bool) [][]uint16 {
	if len(base.CipherSuites) > 0 {
		return [][]uint16{base.CipherSuites}
	}
	if compatMode {
		return [][]uint16{
			eccGMTLSCipherSuitesCompat,
			ecdheGMTLSCipherSuitesCompat,
			allGMTLSCipherSuites,
		}
	}
	return [][]uint16{allGMTLSCipherSuites}
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
	attempts := gmtlsCipherSuiteDialAttempts(baseConfig, config.GMTLSCompatMode)
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
