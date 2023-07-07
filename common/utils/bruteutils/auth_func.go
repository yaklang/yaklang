package bruteutils

import (
	"github.com/yaklang/yaklang/common/utils"
	"strings"
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
	"ssh":            sshAuth,
	"ftp":            ftpAuth,
	"tomcat":         tomcat,
	"vnc":            vncAuth,
	"postgres":       postgresAuth,
	"mysql":          mysqlAuth,
	"redis":          redisAuth,
	"mssql":          mssqlAuth,
	"rdp":            rdpAuth,
	"memcached":      memcachedAuth,
	"mongodb":        mongoAuth,
	"smb":            smbAuth,
	"telnet":         telnetAuth,
	"snmpv2":         snmp_v2Auth,
	"snmpv3_md5":     snmpV3Auth_MD5,
	"snmpv3_sha":     snmpV3Auth_SHA,
	"snmpv3_sha-224": snmpV3Auth_SHA_224,
	"snmpv3_sha-256": snmpV3Auth_SHA_256,
	"snmpv3_sha-384": snmpV3Auth_SHA_384,
	"snmpv3_sha-512": snmpV3Auth_SHA_512,
	//"oracle": func(item *BruteItem) *BruteItemResult {
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
