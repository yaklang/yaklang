//go:build !yakslim

// Oracle 认证依赖 go-ora 的完整 TNS/TTC/O5LOGON 协议栈。
// 任务要求："对无法安全实现的复杂认证机制，宁可保留窄范围、经过审计的依赖，
// 也不要使用不完整的自研密码学实现"——在多版本 Oracle 真实环境完成
// 最小探针的兼容验证之前，默认构建保留 go-ora。
// yakslim 构建通过本标签排除 go-ora（oracle 不可用），用于验证
// 核心调度器与最小探针集合不依赖数据库驱动。
package bruteutils

import (
	"database/sql"
	"os"
	"strings"
	"sync"

	go_ora "github.com/sijms/go-ora/v2"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var oracleServiceNames = []string{
	"orcl",
	"xe",
	"oracle",
}

var setupUserEnvOnce sync.Once

// for fix some OS call user.Current() panic in go-ora.getCurrentUser(), for example: centos7
func setupUserEnv() {
	if _, ok := os.LookupEnv("USER"); !ok {
		os.Setenv("USER", "user")
	}
}

var oracleAuth = &DefaultServiceAuthInfo{
	ServiceName:      "oracle",
	DefaultPorts:     "1521",
	DefaultUsernames: []string{"sys", "system", "oracle"},
	DefaultPasswords: []string{"sys", "sys123", "system", "password", "123qwe", "123456", "oracle", "oracle001", "oracle.com", "admin123..", "adminroot123", "admin", "root"},
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		return i.Result()
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 1521)
		res := i.Result()
		urlOptions := map[string]string{
			"CONNECTION TIMEOUT": "10",
		}
		ip, port, err := utils.ParseStringToHostPort(i.Target)
		if err != nil {
			log.Errorf("parse target[%s] failed: %s", i.Target, err)
			return res
		}

		setupUserEnvOnce.Do(setupUserEnv)

		var lastErr error
		for _, service := range oracleServiceNames {
			dataSourceName := go_ora.BuildUrl(ip, port, service, i.Username, i.Password, urlOptions)
			connector := go_ora.NewConnector(dataSourceName).(*go_ora.OracleConnector)
			connector.Dialer(defaultDialer)
			db := sql.OpenDB(connector)
			if err != nil {
				return res
			}
			db.SetConnMaxLifetime(defaultTimeout)
			db.SetConnMaxIdleTime(defaultTimeout)
			db.SetMaxIdleConns(0)

			err = db.Ping()
			db.Close()
			if err == nil {
				res.Ok = true
				res.Finished = true
				return res
			}
			lastErr = err
			// 检查是否是连接错误，如果是则不再尝试其他service
			if err != nil {
				errStr := err.Error()
				switch true {
				case strings.Contains(errStr, "timeout"):
					fallthrough
				case strings.Contains(errStr, "i/o timeout"):
					fallthrough
				case strings.Contains(errStr, "dial tcp"):
					fallthrough
				case strings.Contains(errStr, "bad connection"):
					fallthrough
				case strings.Contains(errStr, "EOF"):
					fallthrough
				case strings.Contains(errStr, "connection refused"):
					fallthrough
				case strings.Contains(errStr, "no route to host"):
					res.Finished = true
					return res
				}
			}
		}
		// 所有service都尝试失败，停止爆破
		if lastErr != nil {
			log.Debugf("oracle all services failed: %v", lastErr)
			res.Finished = true
		}
		return res
	},
}
