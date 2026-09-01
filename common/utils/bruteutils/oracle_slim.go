//go:build yakslim

// yakslim 构建：Oracle 驱动（go-ora）被排除，oracle 类型不可用。
// 该构建用于证明核心调度器与最小探针集合不依赖数据库驱动。
package bruteutils

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var oracleAuth = &DefaultServiceAuthInfo{
	ServiceName:      "oracle",
	DefaultPorts:     "1521",
	DefaultUsernames: []string{"sys", "system", "oracle"},
	DefaultPasswords: []string{"sys", "sys123", "system", "password", "123qwe", "123456", "oracle", "oracle001", "oracle.com", "admin123..", "adminroot123", "admin", "root"},
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		return i.Result()
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		log.Warnf("oracle brute is not available in yakslim build (target=%s)", i.Target)
		res := i.Result()
		res.Finished = true
		return res
	},
}

var _ = utils.Error
