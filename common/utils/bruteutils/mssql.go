package bruteutils

import (
	"database/sql"
	"net/url"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	mssql "github.com/denisenkom/go-mssqldb"
)

func MSSQLAuth(target, username, password string, needAuth bool) (ok, finished bool, err error) {
	query := url.Values{}

	query.Add("encrypt", "disable")

	u := &url.URL{
		Scheme:   "sqlserver",
		Host:     target,
		RawQuery: query.Encode(),
	}
	if needAuth {
		u.User = url.UserPassword(username, password)
	}
	connStr := u.String()

	connector, err := mssql.NewConnector(connStr)
	connector.Dialer = defaultDialer
	if err != nil {
		return false, true, utils.Wrap(err, "sqlserver create connector failed: %v")
	}

	db := sql.OpenDB(connector)
	db.SetMaxIdleConns(0)
	defer db.Close()
	err = db.Ping()
	if err != nil {
		switch true {
		// connect: connection refused
		case strings.Contains(err.Error(), "i/o timeout"): // 超时
			fallthrough
		case strings.Contains(err.Error(), "invalid packet size"): // 不是mssql协议
			fallthrough
		case strings.Contains(err.Error(), "connect: connection refused"):
			return false, true, err
		}
		return false, false, err
	}
	return true, false, nil
}

var mssqlAuth = &DefaultServiceAuthInfo{
	ServiceName:      "mssql",
	DefaultPorts:     "1433",
	DefaultUsernames: []string{"administrator", "admin", "root", "mssql", "manager", "sa"},
	DefaultPasswords: CommonPasswords,
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 1433)
		result := i.Result()

		ok, finished, err := MSSQLAuth(i.Target, "", "", false)
		if err != nil {
			log.Errorf("mssql unauth verify failed: %s", err)
		}
		result.Ok = ok
		result.Finished = finished
		return result
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 1433)
		result := i.Result()

		ok, finished, err := MSSQLAuth(i.Target, i.Username, i.Password, true)
		if err != nil {
			log.Errorf("mssql brute pass failed: %s", err)
		}
		result.Ok = ok
		result.Finished = finished
		return result
	},
}
