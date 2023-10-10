package bruteutils

import (
	"database/sql"
	"fmt"

	go_ora "github.com/sijms/go-ora/v2"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var oracleServiceNames = []string{
	"orcl",
	"xe",
	"oracle",
}

var oracleAuth = &DefaultServiceAuthInfo{
	ServiceName:      "oracle",
	DefaultPorts:     "1521",
	DefaultUsernames: []string{"sys"},
	DefaultPasswords: []string{"sys", "123456", "oracle", "oracle001", "oracle.com", "admin123..", "adminroot123", "admin", "root"},
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		return i.Result()
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 1521)
		res := i.Result()
		urlOptions := map[string]string{
			"CONNECTION TIMEOUT": fmt.Sprintf("%v", defaultTimeout),
		}
		ip, port, err := utils.ParseStringToHostPort(i.Target)
		if err != nil {
			log.Errorf("parse target[%s] failed: %s", i.Target, err)
			return res
		}

		for _, service := range oracleServiceNames {
			dataSourceName := go_ora.BuildUrl(ip, port, service, i.Username, i.Password, urlOptions)
			db, err := sql.Open("oracle", dataSourceName)
			if err != nil {
				return res
			}
			db.SetConnMaxLifetime(defaultTimeout)
			db.SetConnMaxIdleTime(defaultTimeout)
			db.SetMaxIdleConns(0)

			err = db.Ping()
			defer db.Close()
			if err == nil {
				res.Ok = true
				res.Finished = true
				return res
			}
		}
		return res
	},
}
