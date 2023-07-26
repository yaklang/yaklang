package bruteutils

import (
	"github.com/jlaffaye/ftp"
	"github.com/yaklang/yaklang/common/log"
	"time"
)

var ftpUser = []string{
	"ftp", "www", "anonymous", "admin", "root", "db", "wwwroot", "data", "web",
}

// https://github.com/lowliness9/pocs-collection/tree/e22f0b4075a39ff217547613698991dca3273b30/poc-xunfeng
var ftpAuth = &DefaultServiceAuthInfo{
	ServiceName:      "ftp",
	DefaultPorts:     "21",
	DefaultUsernames: ftpUser,
	DefaultPasswords: []string{
		"admin", "admin123", "root@123", "123456",
	},
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 21)
		var res = i.Result()

		conn, err := DefaultDailer.Dial("tcp", i.Target)
		if err != nil {
			res.Finished = true
			return res
		}
		conn.Close()

		target := i.Target
		c, err := ftp.Dial(target, ftp.DialWithTimeout(5*time.Second))
		if err != nil {
			res.Finished = true
			return res
		}

		err = c.Login("anonymous", "anonymous")
		if err != nil {
			return res
		}
		_ = c.Logout()

		return res
	},
	BrutePass: func(item *BruteItem) *BruteItemResult {
		item.Target = appendDefaultPort(item.Target, 21)
		target := item.Target
		result := item.Result()

		c, err := ftp.Dial(target, ftp.DialWithTimeout(5*time.Second))
		if err != nil {
			log.Errorf("dial ftp failed: %s", err)
			result.Finished = true
			return result
		}
		defer c.Quit()

		err = c.Login(item.Username, item.Password)
		if err != nil {
			log.Errorf("login failed: %s", err)
			return result
		}
		_ = c.Logout()

		result.Ok = true
		return result
	},
}
