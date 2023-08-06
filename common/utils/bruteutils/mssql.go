package bruteutils

import (
	"database/sql"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"net/url"
	"strconv"
	"strings"
	"time"

	_ "github.com/denisenkom/go-mssqldb"
)

var mssqlAuth = &DefaultServiceAuthInfo{
	ServiceName:      "mssql",
	DefaultPorts:     "1433",
	DefaultUsernames: []string{"administrator", "admin", "root", "mssql", "manager", "sa"},
	DefaultPasswords: CommonPasswords,
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		// connect: connection refused
		i.Target = appendDefaultPort(i.Target, 1433)

		result := i.Result()

		conn, err := netx.DialTCPTimeout(defaultTimeout, i.Target)
		if err != nil {
			result.Finished = true
			return result
		}
		defer conn.Close()

		raw, _ := utils.ReadConnWithTimeout(conn, 3*time.Second)
		if raw != nil {
			println(fmt.Sprintf("banner: %s", strconv.Quote(string(raw))))
		}

		return result
	},
	BrutePass: func(item *BruteItem) *BruteItemResult {
		// 1433
		// master
		target := fixToTarget(item.Target, 1433)
		result := item.Result()

		query := url.Values{}

		query.Add("encrypt", "disable")

		u := &url.URL{
			Scheme:   "sqlserver",
			User:     url.UserPassword(item.Username, item.Password),
			Host:     target,
			RawQuery: query.Encode(),
		}
		connStr := u.String()

		db, err := sql.Open("mssql", connStr)
		if err != nil {
			log.Errorf("sqlserver conn failed: %s", err)
			return result
		}
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
				result.Finished = true
				return result
			}
			log.Errorf("mssql db ping failed: %s", err)
			return result
		}
		result.Ok = true
		return result
	},
}
