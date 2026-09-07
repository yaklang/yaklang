package bruteutils

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// RedactPassword 把密码替换为固定长度掩码（空密码标记为 <empty>）。
// 所有会进入日志、结果字符串与远程事件的路径都必须使用它。
func RedactPassword(password string) string {
	if password == "" {
		return "<empty>"
	}
	sum := sha256.Sum256([]byte(password))
	return "<sha256:" + hex.EncodeToString(sum[:4]) + ">"
}

// sanitizePanicMessage 清洗 panic 值中的敏感信息。
// panic 值可能是携带凭证的任意错误，只保留有限长度的描述。
func sanitizePanicMessage(r interface{}) string {
	msg := fmt.Sprintf("%v", r)
	if len(msg) > 120 {
		msg = msg[:120]
	}
	return msg
}

// itemCtx 返回 BruteItem 的 context，nil 时退回 Background
// （旧 API 允许直接调用 BrutePass，此时没有调度器注入 ctx）。
func itemCtx(i *BruteItem) context.Context {
	if i != nil && i.Context != nil {
		return i.Context
	}
	return context.Background()
}

// RedactText 把文本中出现的明文密码替换为掩码。
// 供旧协议实现清洗可能携带凭证的错误信息使用。
func RedactText(text, password string) string {
	if password == "" {
		return text
	}
	return strings.ReplaceAll(text, password, "<redacted>")
}
