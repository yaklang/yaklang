package bruteutils

import (
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
)

import "github.com/stacktitan/smb/smb"

const smbPasswd = `{{params(user)}}
{{params(user)}}123
{{params(user)}}1234
{{params(user)}}123456
{{params(user)}}12345
{{params(user)}}@123
{{params(user)}}@123456
{{params(user)}}@12345
{{params(user)}}#123
{{params(user)}}#123456
{{params(user)}}#12345
{{params(user)}}_123
{{params(user)}}_123456
{{params(user)}}_12345
{{params(user)}}123!@#
{{params(user)}}!@#$
{{params(user)}}!@#
{{params(user)}}~!@
{{params(user)}}!@#123
qweasdzxc
{{params(user)}}2017
{{params(user)}}2016
{{params(user)}}2015
{{params(user)}}@2017
{{params(user)}}@2016
{{params(user)}}@2015
Passw0rd
admin123!@#
admin
admin123
admin@123
admin#123
123456
password
12345
1234
root
123
qwerty
test
1q2w3e4r
1qaz2wsx
qazwsx
123qwe
123qaz
0000
oracle
1234567
123456qwerty
password123
12345678
1q2w3e
abc123
okmnji
test123
123456789
postgres
q1w2e3r4
redhat
user
mysql
apache`

var smbAuth = &DefaultServiceAuthInfo{
	ServiceName:      "smb",
	DefaultPorts:     "445",
	DefaultUsernames: []string{"administrator", "admin", "test", "user", "manager", "webadmin", "guest", "db2admin", "system", "root", "sa"},
	DefaultPasswords: utils.ParseStringToLines(smbPasswd),
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 445)
		conn, err := DefaultDailer.Dial("tcp", i.Target)
		if err != nil {
			res := i.Result()
			res.Finished = true
			return res
		}
		defer conn.Close()
		return i.Result()
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 445)
		host, port, _ := utils.ParseStringToHostPort(i.Target)
		rdb := smb.Options{
			Host:     host,
			Port:     port,
			User:     i.Username,
			Password: i.Password,
		}
		session, err := smb.NewSession(rdb, false)
		if err != nil {
			log.Errorf("smb.NewSession failed: %s", err)
			return i.Result()
		}
		res := i.Result()
		if session.IsAuthenticated {
			res.Ok = true
			return res
		}
		return res
	},
}
