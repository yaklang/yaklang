package main

import (
	"github.com/yaklang/yaklang/common/schema"
)

// grpcPhaseI18n maps engine startup phase identifiers to bilingual labels.
// These are the values emitted in the "phase" field of ready/failed events.
var grpcPhaseI18n = map[string]*schema.I18n{
	"init":          schema.NewI18n("初始化", "Initialization"),
	"pprof":         schema.NewI18n("性能分析服务启动", "pprof server startup"),
	"database":      schema.NewI18n("数据库初始化", "Database initialization"),
	"build_server":  schema.NewI18n("gRPC 服务构建", "gRPC server build"),
	"cert":          schema.NewI18n("TLS 证书生成", "TLS certificate generation"),
	"listen":        schema.NewI18n("网络监听", "Network listen"),
	"serve":         schema.NewI18n("gRPC 服务运行", "gRPC server serve"),
	"wait_connect":  schema.NewI18n("等待服务就绪", "Waiting for server ready"),
	"dial":          schema.NewI18n("gRPC 连接", "gRPC dial"),
	"version_rpc":   schema.NewI18n("认证与版本校验", "Authentication and Version RPC"),
}

// grpcReasonI18n maps structured reason codes to bilingual user-facing hints.
// Keys cover both:
//   - bracketed prefix codes from classifyListenError (e.g. tcp_bind_in_use)
//   - check-secret reason constants (e.g. database_error, dial_failed)
var grpcReasonI18n = map[string]*schema.I18n{
	// listen error sub-categories (from classifyListenError)
	"tcp_bind_denied": schema.NewI18n(
		"端口被系统策略阻止，请以管理员身份运行 Yakit，或检查防火墙设置",
		"Permission denied: the port may require elevated privileges or is blocked by system policy. Run Yakit as administrator, or check firewall settings",
	),
	"tcp_bind_in_use": schema.NewI18n(
		"端口被另一个进程占用，请结束占用该端口的旧进程，或切换到其他端口",
		"Address already in use: another process is listening on this port. Stop the process occupying this port, or switch to another port",
	),
	"tcp_bind_failed": schema.NewI18n(
		"网络监听失败，请检查端口配置或网络环境后重试",
		"Network listen failed. Check port configuration or network environment and retry",
	),
	"listen_failed": schema.NewI18n(
		"网络监听失败，请检查端口配置或网络环境后重试",
		"Network listen failed. Check port configuration or network environment and retry",
	),

	// check-secret reason constants
	"database_error": schema.NewI18n(
		"数据库初始化失败，可点击「修复数据库」按钮修复",
		"Database initialization failed. Click the Fix Database button to repair",
	),
	"build_server_failed": schema.NewI18n(
		"gRPC 服务构建失败，请重启 Yakit 后重试，如问题持续请重新安装引擎",
		"gRPC server build failed. Restart Yakit and retry; if the problem persists, reinstall the engine",
	),
	"dial_failed": schema.NewI18n(
		"连接 gRPC 服务器失败，请确认引擎进程正常运行，或重启 Yakit 后重试",
		"Failed to dial gRPC server. Ensure the engine process is running, or restart Yakit and retry",
	),
	"version_rpc_failed": schema.NewI18n(
		"认证或版本校验失败，请确认密码正确，或重启 Yakit 后重试",
		"Authentication or Version RPC failed. Verify the password is correct, or restart Yakit and retry",
	),
	"wait_connect_failed": schema.NewI18n(
		"等待 gRPC 服务就绪超时，请重启 Yakit 后重试",
		"Timed out waiting for gRPC server to be ready. Restart Yakit and retry",
	),

	// phase-fallback reason codes (used when no [xxx] prefix in error message)
	"init_failed": schema.NewI18n(
		"引擎初始化失败，请检查启动参数后重试",
		"Engine initialization failed. Check startup parameters and retry",
	),
	"pprof_failed": schema.NewI18n(
		"性能分析服务启动失败，请检查 pprof 端口配置后重试",
		"pprof server startup failed. Check pprof port configuration and retry",
	),
	"database_failed": schema.NewI18n(
		"数据库初始化失败，可点击「修复数据库」按钮修复",
		"Database initialization failed. Click the Fix Database button to repair",
	),
	"cert_failed": schema.NewI18n(
		"TLS 证书生成失败，请重启 Yakit 后重试，如问题持续请重新安装引擎",
		"TLS certificate generation failed. Restart Yakit and retry; if the problem persists, reinstall the engine",
	),
	"serve_failed": schema.NewI18n(
		"gRPC 服务运行异常，请重启 Yakit 后重试",
		"gRPC server serve failed. Restart Yakit and retry",
	),
}

// grpcEventPhaseI18n returns the bilingual label for a phase, or nil if unknown.
func grpcEventPhaseI18n(phase string) *schema.I18n {
	return grpcPhaseI18n[phase]
}

// grpcEventReasonI18n returns the bilingual hint for a reason code, or nil if unknown.
func grpcEventReasonI18n(reasonCode string) *schema.I18n {
	return grpcReasonI18n[reasonCode]
}



