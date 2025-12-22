package totpproxy

// Exports 导出给 Yak 脚本使用的函数和变量
var Exports = map[string]any{
	// 服务器
	"Serve":        Serve,
	"ServeWithTLS": ServeWithTLS,

	// 服务器配置选项
	"totpSecret":   WithTOTPSecret,
	"totpHeader":   WithTOTPHeader,
	"targetTLS":    WithTargetTLS,
	"allowedPaths": WithAllowedPaths,
	"timeout":      WithTimeout,
	"debug":        WithDebug,

	// TLS 配置选项
	"enableTLS": WithEnableTLS,
	"tlsCert":   WithTLSCert,

	// TOTP 工具函数
	"GetTOTPCode":    GetTOTPCode,
	"VerifyTOTPCode": VerifyTOTPCode,

	// 常量
	"DefaultTOTPHeader": DefaultTOTPHeader,
	"DefaultTimeout":    DefaultTimeout,
}

// 便捷函数，用于快速创建并启动服务器
// 示例:
//
//	server = totpproxy.Serve("0.0.0.0:8443", "127.0.0.1:21002", totpproxy.totpSecret("my-secret"))
//	defer server.Stop()
func Serve(listenAddr, targetAddr string, opts ...ServerOption) (*Server, error) {
	allOpts := []ServerOption{
		WithListenAddr(listenAddr),
		WithTargetAddr(targetAddr),
	}
	allOpts = append(allOpts, opts...)

	server := NewServer(allOpts...)
	if err := server.Start(); err != nil {
		return nil, err
	}
	return server, nil
}

// ServeWithTLS 启动带 TLS 的反向代理服务器
func ServeWithTLS(listenAddr, targetAddr string, cert, key []byte, opts ...ServerOption) (*Server, error) {
	allOpts := []ServerOption{
		WithListenAddr(listenAddr),
		WithTargetAddr(targetAddr),
		WithEnableTLS(true),
		WithTLSCert(cert, key),
	}
	allOpts = append(allOpts, opts...)

	server := NewServer(allOpts...)
	if err := server.Start(); err != nil {
		return nil, err
	}
	return server, nil
}
