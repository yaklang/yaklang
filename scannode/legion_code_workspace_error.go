package scannode

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"syscall"
)

// sourceWorkspaceFailureMessage preserves actionable causes without forwarding
// raw Git/HTTP errors: those may contain credentials, signed URLs or local paths.
// Keep the existing event codes and payload shape for older Legion consumers.
func sourceWorkspaceFailureMessage(err error) string {
	message := strings.ToLower(err.Error())
	contains := func(parts ...string) bool {
		for _, part := range parts {
			if strings.Contains(message, part) {
				return true
			}
		}
		return false
	}
	var dnsErr *net.DNSError
	var netErr net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return "源码准备已取消，请重新发起任务。"
	case errors.As(err, &dnsErr), contains("no such host", "temporary failure in name resolution"):
		return "源码准备失败：无法解析源服务域名，请检查执行节点的 DNS 和网络配置。"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout(), contains("i/o timeout", "context deadline exceeded", "connection timed out", "tls handshake timeout"):
		return "源码准备失败：访问源服务超时，请检查执行节点到仓库或文件服务的网络及代理；Git 仓库也可改为上传源码 ZIP 后重试。"
	case errors.Is(err, syscall.ENOSPC), contains("insufficient free space", "no space left on device", "disk quota exceeded", "required_minimum_bytes=", "available_inodes=0"):
		return "源码准备失败：执行节点磁盘空间不足，请释放工作区磁盘空间后重试。"
	case errors.Is(err, os.ErrPermission):
		return "源码准备失败：执行节点无法读写本地工作区，请检查工作目录权限。"
	case contains("authentication required", "authentication failed", "authorization failed", "permission denied (publickey)", "unable to authenticate", "status=401", "status=403"):
		return "源码准备失败：源服务认证失败或访问被拒绝，请检查仓库凭据和读取权限；上传文件请检查节点会话及文件访问权限。"
	case contains("repository not found", "remote repository is empty", "reference not found", "couldn't find remote ref", "status=404"):
		return "源码准备失败：仓库、分支或源文件不存在或不可访问，请检查源地址、分支和读取权限；上传文件可重新上传后重试。"
	case contains("connection refused", "network is unreachable", "no route to host", "connection reset by peer", "unexpected eof"):
		return "源码准备失败：无法连接源服务或连接中断，请检查执行节点到仓库或文件服务的网络及代理后重试。"
	case contains("x509:", "certificate verify failed", "host key mismatch", "knownhosts: key"):
		return "源码准备失败：源服务证书或 SSH 主机身份校验失败，请检查执行节点的信任配置。"
	case contains("sha256 mismatch", "revision mismatch"):
		return "源码准备失败：源码版本或校验值与任务记录不一致，请确认源内容后重新发起任务。"
	case contains("zip: not a valid zip file", "zip: checksum error"):
		return "源码准备失败：源码 ZIP 无效或已损坏，请重新打包并上传后重试。"
	default:
		return "源码准备失败，尚未开始 AI 分析。请检查源地址、分支、读取权限及执行节点工作区；Git 仓库可改为上传源码 ZIP 后重试。"
	}
}
