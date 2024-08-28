package bruteutils

import (
	"strings"
	"time"

	"github.com/go-pg/pg/v10"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func postgresqlUnAuthCheck(host string, port int) (bool, error) {
	sendData := []byte{58, 0, 0, 0, 167, 65, 0, 0, 0, 0, 0, 0, 212, 7, 0, 0, 0, 0, 0, 0, 97, 100, 109, 105, 110, 46, 36, 99, 109, 100, 0, 0, 0, 0, 0, 255, 255, 255, 255, 19, 0, 0, 0, 16, 105, 115, 109, 97, 115, 116, 101, 114, 0, 1, 0, 0, 0, 0}
	getlogData := []byte{72, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 212, 7, 0, 0, 0, 0, 0, 0, 97, 100, 109, 105, 110, 46, 36, 99, 109, 100, 0, 0, 0, 0, 0, 1, 0, 0, 0, 33, 0, 0, 0, 2, 103, 101, 116, 76, 111, 103, 0, 16, 0, 0, 0, 115, 116, 97, 114, 116, 117, 112, 87, 97, 114, 110, 105, 110, 103, 115, 0, 0}
	conn, err := defaultDialer.DialContext(utils.TimeoutContext(defaultTimeout), "tcp", utils.HostPort(host, port))
	if err != nil {
		return false, err
	}
	defer conn.Close()
	err = conn.SetReadDeadline(time.Now().Add(defaultTimeout))
	if err != nil {
		return false, err
	}
	_, err = conn.Write(sendData)
	if err != nil {
		return false, err
	}
	buf := make([]byte, 1024)
	count, err := conn.Read(buf)
	if err != nil {
		return false, err
	}
	text := string(buf[0:count])
	if strings.Contains(text, "ismaster") == false {
		return false, err
	}
	_, err = conn.Write(getlogData)
	if err != nil {
		return false, err
	}
	count, err = conn.Read(buf)
	if err != nil {
		return false, err
	}
	text = string(buf[0:count])
	if strings.Contains(text, "totalLinesWritten") == false {
		return false, err
	}
	return true, err
}

var postgresAuth = &DefaultServiceAuthInfo{
	ServiceName:      "postgres",
	DefaultPorts:     "5432",
	DefaultUsernames: append([]string{"postgres"}, CommonUsernames...),
	DefaultPasswords: CommonPasswords,
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 5432)

		result := i.Result()
		conn, err := defaultDialer.DialContext(utils.TimeoutContext(defaultTimeout), "tcp", i.Target)
		if err != nil {
			result.Finished = true
			return result
		}
		conn.Close()

		host, port, _ := utils.ParseStringToHostPort(i.Target)
		r, _ := postgresqlUnAuthCheck(host, port)
		if r {
			result.Ok = true
			return result
		}

		return result
	},
	BrutePass: func(item *BruteItem) *BruteItemResult {
		// 173.254.29.192/24
		item.Target = appendDefaultPort(item.Target, 5432)
		result := item.Result()

		db := pg.Connect(&pg.Options{
			Addr:     item.Target,
			User:     item.Username,
			Password: item.Password,
			Dialer:   defaultDialer.DialContext,
			Database: "postgres",
		})
		_, err := db.Exec("select 1")
		if err != nil {
			result.Ok = false
			switch true {
			case strings.Contains(err.Error(), "connect: connection refused"):
				fallthrough
			case strings.Contains(err.Error(), "no pg_hba.conf entry for host"):
				fallthrough
			case strings.Contains(err.Error(), "network unreachable"):
				fallthrough
			case strings.Contains(err.Error(), "i/o timeout"):
				result.Finished = true
				return result
			}
			log.Errorf("exec select 1 failed: %v", err)
			return result
		}
		result.Ok = true
		return result
	},
}
