package tools

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"math"
	"time"

	"github.com/yaklang/yaklang/common/utils/bruteutils"
)

func oracleConfig(b *yakBruter) *bruteutils.OracleConfig {
	if b.oracleConfig == nil {
		b.oracleConfig = &bruteutils.OracleConfig{}
	}
	return b.oracleConfig
}

// oracleService 指定 Oracle SERVICE_NAME 候选，不再猜测默认服务名。
// Example:
// ```
// b = brute.New("oracle", brute.oracleService("SALES_PDB"))~
// ```
func yakBruteOpt_OracleService(services ...string) BruteOpt {
	names := append([]string(nil), services...)
	return func(b *yakBruter) { c := oracleConfig(b); c.Services = append([]string(nil), names...); c.SID = false }
}

// oracleSID 使用指定 SID 连接 Oracle，替代 SERVICE_NAME。
// Example:
// ```
// b = brute.New("oracle", brute.oracleSID("ORCL"))~
// ```
func yakBruteOpt_OracleSID(sid string) BruteOpt {
	return func(b *yakBruter) { c := oracleConfig(b); c.Services = []string{sid}; c.SID = true }
}

// oracleSysDBA 显式选择 SYSDBA 模式；默认仅用户名 SYS 自动启用。
// Example:
// ```
// b = brute.New("oracle", brute.oracleSysDBA(false))~
// ```
func yakBruteOpt_OracleSysDBA(enabled bool) BruteOpt {
	return func(b *yakBruter) { v := enabled; oracleConfig(b).SysDBA = &v }
}

// oracleEncryption 设置原生加密策略：accepted/rejected/requested/required。
// required 要求原生加密，不会静默降级；此选项独立于 TCPS。
// Example:
// ```
// enc = brute.oracleEncryption("required")~
// b = brute.New("oracle", enc)~
// ```
func yakBruteOpt_OracleEncryption(policy string) (BruteOpt, error) {
	if err := (&bruteutils.OracleConfig{Encryption: policy}).Validate(); err != nil {
		return nil, err
	}
	return func(b *yakBruter) { oracleConfig(b).Encryption = policy }, nil
}

// oracleTLS 强制使用 TCPS 并校验证书，失败不会退回明文。
// serverName 为空时按目标主机校验；可选 caPEM 指定信任的 PEM CA，省略时用系统 CA。
// Example:
// ```
// tlsOpt = brute.oracleTLS("db.example.com")~
// b = brute.New("oracle", brute.oracleService("SALES"), tlsOpt)~
// ```
func yakBruteOpt_OracleTLS(serverName string, caPEM ...string) (BruteOpt, error) {
	config := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName}
	if len(caPEM) > 0 {
		config.RootCAs = x509.NewCertPool()
		for _, pem := range caPEM {
			if !config.RootCAs.AppendCertsFromPEM([]byte(pem)) {
				return nil, errors.New("oracle: invalid CA PEM")
			}
		}
	}
	return func(b *yakBruter) { oracleConfig(b).TLS = config.Clone() }, nil
}

// oracleTimeout 设置整个凭证验证的秒数预算，所有服务和重试共享，最多 20 秒。
// Example:
// ```
// timeoutOpt = brute.oracleTimeout(15)~
// b = brute.New("oracle", timeoutOpt)~
// ```
func yakBruteOpt_OracleTimeout(seconds float64) (BruteOpt, error) {
	if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return nil, errors.New("oracle: timeout must be positive and finite")
	}
	timeout := time.Duration(math.Min(seconds, 20) * float64(time.Second))
	if timeout <= 0 {
		return nil, errors.New("oracle: timeout is too small")
	}
	return func(b *yakBruter) { oracleConfig(b).Timeout = timeout }, nil
}
