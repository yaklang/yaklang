package bruteutils

import (
	"strings"
	"yaklang.io/yaklang/common/utils"
)

func fixToTarget(target string, defaultPort int) string {
	host, port, _ := utils.ParseStringToHostPort(target)
	if port <= 0 {
		target = utils.HostPort(target, defaultPort)
	} else {
		target = utils.HostPort(host, port)
	}
	return target
}

func GetBruteFuncByType(t string) (BruteCallback, error) {
	service := strings.TrimSpace(strings.ToLower(t))
	f, ok := authFunc[service]
	if !ok {
		return nil, utils.Errorf("no brute type[%s] fetched", t)
	}
	return f.GetBruteHandler(), nil
}

func GetBuildinAvailableBruteType() []string {
	var res []string
	for i := range authFunc {
		res = append(res, i)
	}
	return res
}

// rdp https://palm/common/utils/bruteutils/grdp
var authFunc = map[string]*DefaultServiceAuthInfo{
	"ssh":       sshAuth,
	"ftp":       ftpAuth,
	"tomcat":    tomcat,
	"vnc":       vncAuth,
	"postgres":  postgresAuth,
	"mysql":     mysqlAuth,
	"redis":     redisAuth,
	"mssql":     mssqlAuth,
	"rdp":       rdpAuth,
	"memcached": memcachedAuth,
	"mongodb":   mongoAuth,
	"smb":       smbAuth,
	//"oracle": func(item *BruteItem) *BruteItemResult {
	//
	//},
	//"telnet": func(item *BruteItem) *BruteItemResult {
	//
	//},
}

func GetUsernameListFromBruteType(t string) []string {
	i, ok := authFunc[t]
	if !ok {
		return CommonUsernames
	}
	if len(i.DefaultUsernames) > 0 {
		return i.DefaultUsernames
	}
	return CommonUsernames
}

func GetPasswordListFromBruteType(t string) []string {
	i, ok := authFunc[t]
	if !ok {
		return CommonPasswords
	}
	if len(i.DefaultPasswords) > 0 {
		return i.DefaultPasswords
	}
	return CommonPasswords
}
