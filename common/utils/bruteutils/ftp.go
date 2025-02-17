package bruteutils

import (
	"errors"
	"github.com/jlaffaye/ftp"
)

var ftpUser = []string{
	"ftp", "www", "anonymous", "admin", "root", "db", "wwwroot", "data", "web",
}

func FTPAuth(target, username, password string) (bool, error) {
	c, err := ftp.Dial(target, ftp.DialWithTimeout(defaultTimeout), ftp.DialWithDialFunc(defaultDialer.Dial))
	if err != nil {
		return false, err
	}
	defer c.Quit()

	err = c.Login(username, password)
	if err != nil {
		return false, err
	}

	return true, nil
}

// https://github.com/lowliness9/pocs-collection/tree/e22f0b4075a39ff217547613698991dca3273b30/poc-xunfeng
var ftpAuth = &DefaultServiceAuthInfo{
	ServiceName:      "ftp",
	DefaultPorts:     "21",
	DefaultUsernames: ftpUser,
	DefaultPasswords: append([]string{
		"admin", "123456",
		"root", "password", "123123", "123", "", "1",
		"qwa123", "12345678", "test", "123qwe!@#", "p@ssw0rd",
		"123456789", "123321", "1314520", "666666", "88888888",
		"fuckyou", "000000", "woaini", "qwerty", "1qaz2wsx", "abc123",
		"abc123456", "1q2w3e4r", "123qwe", "159357", "p@55w0rd", "r00t",
		"tomcat", "apache", "system", "huawei", "zte",
		"{{param(user)}}{{param(user)}}",
		"{{param(user)}}", "{{param(user)}}1",
		"{{param(user)}}!",
	}),
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 21)
		result := i.Result()

		ok, err := FTPAuth(i.Target, "anonymous", "anonymous")
		if err != nil && errors.Is(err, dialError) {
			result.Finished = true
			return result
		}
		result.Ok = ok
		return result
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 21)
		result := i.Result()
		ok, err := FTPAuth(i.Target, i.Username, i.Password)
		if err != nil && errors.Is(err, dialError) {
			result.Finished = true
			return result
		}
		result.Ok = ok
		return result
	},
}
