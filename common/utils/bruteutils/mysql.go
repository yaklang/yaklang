package bruteutils

import (
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"

	_ "github.com/go-sql-driver/mysql"
)

var mysqlAuth = &DefaultServiceAuthInfo{
	ServiceName:      "mysql",
	DefaultPorts:     "3306",
	DefaultUsernames: append([]string{"mysql", "root", "guest", "op", "ops"}),
	DefaultPasswords: CommonPasswords,
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 3306)
		res := i.Result()

		conn, err := netx.DialTCPTimeout(defaultTimeout, i.Target)
		if err != nil {
			res.Finished = true
			return res
		}
		raw, _ := utils.ReadConnWithTimeout(conn, 3*time.Second)
		if raw != nil {
			if strings.Contains(string(raw), "is not allowed to connect") {
				res.Finished = true
				return res
			} else {
				log.Infof("fetch mysql banner: %s", strconv.Quote(string(raw)))
			}
		}
		return i.Result()
	},
	BrutePass: func(item *BruteItem) *BruteItemResult {
		item.Target = appendDefaultPort(item.Target, 3306)
		result := item.Result()
		db, err := sql.Open("mysql", fmt.Sprintf("%v:%v@tcp(%v)/mysql",
			url.PathEscape(item.Username),
			url.PathEscape(item.Password),
			item.Target))
		if err != nil {
			log.Errorf("connect to mysql failed: %v", err)
			return result
		}
		_, err = db.Exec("select 1")
		if err != nil {
			switch true {
			case strings.Contains(err.Error(), "is not allowed to connect to"):
				result.Finished = true
				return result
			case strings.Contains(err.Error(), "connect: connection refused"): // connect: connection refused
				result.Finished = true
				return result
			case strings.Contains(err.Error(), "Error 1045:"):
				log.Errorf("auth failed: %s/%v", item.Username, item.Password)
				return result
			}
			log.Errorf("exec 'select 1' to mysql failed: %v, (%v:%v)", err, item.Username, item.Password)
			return result
		}
		result.Ok = true
		return result
	},
}
