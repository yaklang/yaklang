package bruteutils

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/go-sql-driver/mysql"
)

var registerDialContextOnce sync.Once

func MYSQLAuth(target, username, password string, needAuth bool) (ok, finished bool, err error) {
	registerDialContextOnce.Do(func() {
		mysql.RegisterDialContext("tcp", func(ctx context.Context, addr string) (net.Conn, error) {
			return defaultDialer.DialContext(ctx, "tcp", addr)
		})
	})

	dsn := fmt.Sprintf("tcp(%v)/mysql", target)
	if needAuth {
		dsn = fmt.Sprintf("%v:%v@%v", url.PathEscape(username), url.PathEscape(password), dsn)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return false, false, err
	}
	_, err = db.Exec("select 1")
	if err != nil {
		errStr := err.Error()
		switch true {
		case strings.Contains(errStr, "is not allowed to connect to"):
			fallthrough
		case strings.Contains(errStr, "connect: connection refused"): // connect: connection refused
			return false, true, err
		case strings.Contains(errStr, "Error 1045:"):
			return false, false, utils.Wrapf(err, "auth failed: %s/%v", username, password)
		case strings.Contains(errStr, "Error 1044:") || strings.Contains(errStr, "1044:"):
			return true, false, nil
		}

		return false, false, utils.Wrapf(err, "exec 'select 1' to mysql failed: %v, (%v:%v)", err, username, password)
	}
	return true, false, nil
}

var mysqlAuth = &DefaultServiceAuthInfo{
	ServiceName:      "mysql",
	DefaultPorts:     "3306",
	DefaultUsernames: []string{"mysql", "root", "guest", "op", "ops"},
	DefaultPasswords: append(CommonPasswords, ""),
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 3306)
		res := i.Result()

		ok, finished, err := MYSQLAuth(i.Target, "", "", false)
		if err != nil {
			log.Errorf("mysql unauth verify failed: %v", err)
		}
		res.Ok = ok
		res.Finished = finished

		return i.Result()
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 3306)
		res := i.Result()

		ok, finished, err := MYSQLAuth(i.Target, i.Username, i.Password, true)
		if err != nil {
			log.Errorf("mysql brute pass failed: %v", err)
		}
		res.Ok = ok
		res.Finished = finished

		return res
	},
}
