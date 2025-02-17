package bruteutils

import (
	"errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/smb"
)

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

func smbBrutePass(i *BruteItem) (*BruteItemResult, error) {
	i.Target = appendDefaultPort(i.Target, 445)
	host, port, _ := utils.ParseStringToHostPort(i.Target)
	rdb := smb.Options{
		Host:     host,
		Port:     port,
		User:     i.Username,
		Password: i.Password,
		Dialer:   defaultDialer,
		Context:  i.Context,
	}
	session, err := smb.NewSession(rdb, false)
	res := i.Result()

	if err != nil && errors.Is(err, dialError) {
		res.Finished = true
		return res, err
	}
	if session.IsAuthenticated {
		res.Ok = true
	}
	return res, nil
}

var smbAuth = &DefaultServiceAuthInfo{
	ServiceName:      "smb",
	DefaultPorts:     "445",
	DefaultUsernames: []string{"administrator", "admin", "test", "user", "manager", "webadmin", "guest", "db2admin", "system", "root", "sa"},
	DefaultPasswords: utils.ParseStringToLines(smbPasswd),
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		result, err := smbBrutePass(i)
		if err != nil {
			log.Errorf("smb un-auth verify failed: %s", err)
		}
		return result
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		result, err := smbBrutePass(i)
		if err != nil {
			log.Errorf("smb brute pass failed: %s", err)
		}
		return result
	},
}
