package scannode

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"
)

func TestSourceWorkspaceFailureMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"cancelled", context.Canceled, "已取消"},
		{"deadline", context.DeadlineExceeded, "访问源服务超时"},
		{"wrapped URL timeout", &url.Error{Op: "Get", URL: "https://user:secret@private.invalid/repo?token=secret", Err: context.DeadlineExceeded}, "访问源服务超时"},
		{"DNS", &net.DNSError{Err: "no such host", Name: "private.invalid"}, "无法解析源服务域名"},
		{"string timeout", errors.New("dial tcp private.invalid: i/o timeout"), "访问源服务超时"},
		{"disk full", &os.PathError{Op: "write", Path: "/private/workspace", Err: syscall.ENOSPC}, "磁盘空间不足"},
		{"preflight space", errors.New("SSA Git workdir preflight failed: directory=/private/workspace available_bytes=1 required_minimum_bytes=100; free disk space"), "磁盘空间不足"},
		{"preflight inodes", errors.New("SSA Git workdir preflight failed: available_inodes=0; free inodes"), "磁盘空间不足"},
		{"local permission", &os.PathError{Op: "mkdir", Path: "/private/workspace", Err: os.ErrPermission}, "本地工作区"},
		{"git auth", errors.New("authentication required: https://user:secret@private.invalid"), "认证失败或访问被拒绝"},
		{"SSH auth", errors.New("Permission denied (publickey)"), "认证失败或访问被拒绝"},
		{"payload permission", errors.New("download managed source payload failed: status=403"), "认证失败或访问被拒绝"},
		{"missing repo", errors.New("repository not found"), "不存在或不可访问"},
		{"missing branch", errors.New("reference not found"), "不存在或不可访问"},
		{"connection", errors.New("dial tcp: connection refused"), "无法连接源服务或连接中断"},
		{"certificate", errors.New("x509: certificate signed by unknown authority"), "身份校验失败"},
		{"revision", errors.New("source_workspace revision mismatch: expected=secret actual=other"), "校验值"},
		{"bad zip", errors.New("zip: not a valid zip file"), "ZIP 无效或已损坏"},
		{"unknown", errors.New("unrecognized failure: Bearer secret\n/private/workspace https://private.invalid/?token=secret"), "尚未开始 AI 分析"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := sourceWorkspaceFailureMessage(fmt.Errorf("prepare source workspace: %w", tt.err))
			if !strings.Contains(message, tt.want) {
				t.Fatalf("message = %q, want %q", message, tt.want)
			}
			for _, secret := range []string{"secret", "private.invalid", "/private/workspace", "Bearer", "\n"} {
				if strings.Contains(message, secret) {
					t.Fatalf("message leaked input %q: %q", secret, message)
				}
			}
		})
	}
}
