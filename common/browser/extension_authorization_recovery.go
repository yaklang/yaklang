package browser

import (
	"strings"
)

func extensionAuthorizationRecoveryForError(
	err error,
) *ExtensionAuthorizationRecovery {
	recovery := &ExtensionAuthorizationRecovery{
		Code:      "rebuild-workspace",
		Scope:     "workspace",
		Message:   "当前测试证据已失效，请重新建立身份工作区",
		Automatic: false,
	}
	if err == nil {
		return recovery
	}

	reason := strings.ToLower(err.Error())
	switch {
	case strings.Contains(reason, "device") ||
		strings.Contains(reason, "connection") ||
		strings.Contains(reason, "offline") ||
		strings.Contains(reason, "paired"):
		recovery.Code = "reconnect-device"
		recovery.Message = "浏览器连接已变化，请刷新设备或重新配对后再建立工作区"
	case strings.Contains(reason, "document") ||
		strings.Contains(reason, "target") ||
		strings.Contains(reason, "origin"):
		recovery.Code = "reselect-document"
		recovery.Message = "页面文档已变化，请重新选择当前页面并建立身份工作区"
	case strings.Contains(reason, "transform profile") ||
		strings.Contains(reason, "callable") ||
		strings.Contains(reason, "logical binding") ||
		strings.Contains(reason, "profile"):
		recovery.Code = "rebind-transform"
		recovery.Scope = "transform"
		recovery.Message = "页面转换能力已变化，请重新验证明文网关并编译测试计划"
	case strings.Contains(reason, "baseline") ||
		strings.Contains(reason, "wire request"):
		recovery.Code = "recapture-baselines"
		recovery.Scope = "baseline"
		recovery.Message = "请求基线已变化，请使用当前身份重新录制 A/B 基线"
	case strings.Contains(reason, "context") ||
		strings.Contains(reason, "cookie store") ||
		strings.Contains(reason, "isolation") ||
		strings.Contains(reason, "authentication") ||
		strings.Contains(reason, "grant"):
		recovery.Code = "rebuild-identity-proof"
		recovery.Scope = "identity"
		recovery.Message = "身份隔离上下文已变化，请重新建立 A/B 身份证明"
	}
	return recovery
}
