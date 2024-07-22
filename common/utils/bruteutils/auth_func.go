package bruteutils

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
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

var AuthFunctionMap = []struct {
	Name string
	Data string
}{
	{Name: "ssh", Data: "ssh"},
	{Name: "ftp", Data: "ftp"},
	{Name: "tomcat", Data: "tomcat"},
	{Name: "vnc", Data: "vnc"},
	{Name: "postgres", Data: "postgres"},
	{Name: "mysql", Data: "mysql"},
	{Name: "redis", Data: "redis"},
	{Name: "mssql", Data: "mssql"},
	{Name: "rdp", Data: "rdp"},
	{Name: "memcached", Data: "memcached"},
	{Name: "mongodb", Data: "mongodb"},
	{Name: "oracle", Data: "oracle"},
	{Name: "smb", Data: "smb"},
	{Name: "imap", Data: "imap"},
	{Name: "smtp", Data: "smtp"},
	{Name: "pop3", Data: "pop3"},
	{Name: "telnet", Data: "telnet"},
	{Name: "snmpv2", Data: "snmpv2"},
	{Name: "snmpv3/md5", Data: "snmpv3_md5"},
	{Name: "snmpv3/sha", Data: "snmpv3_sha"},
	{Name: "snmpv3/sha-224", Data: "snmpv3_sha-224"},
	{Name: "snmpv3/sha-256", Data: "snmpv3_sha-256"},
	{Name: "snmpv3/sha-384", Data: "snmpv3_sha-384"},
	{Name: "snmpv3/sha-512", Data: "snmpv3_sha-512"},
	{Name: "rtsp", Data: "rtsp"},
	{Name: "http_proxy", Data: "http_proxy"},
	{Name: "socks_proxy/v5", Data: "socks5_proxy"},
	{Name: "socks_proxy/v4", Data: "socks4_proxy"},
	{Name: "socks_proxy/v4a", Data: "socks4a_proxy"},
	{Name: "pptp", Data: "pptp"},
	{Name: "ldap", Data: "ldap"},
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
	"oracle":         oracleAuth,
	"smb":            smbAuth,
	"telnet":         telnetAuth,
	"imap":           imapAuth,
	"smtp":           smtpAuth,
	"pop3":           pop3Auth,
	"snmpv2":         snmp_v2Auth,
	"snmpv3_md5":     snmpV3BruteFactory("snmpv3_md5"),
	"snmpv3_sha":     snmpV3BruteFactory("snmpv3_sha"),
	"snmpv3_sha-224": snmpV3BruteFactory("snmpv3_sha-224"),
	"snmpv3_sha-256": snmpV3BruteFactory("snmpv3_sha-256"),
	"snmpv3_sha-384": snmpV3BruteFactory("snmpv3_sha-384"),
	"snmpv3_sha-512": snmpV3BruteFactory("snmpv3_sha-512"),
	"rtsp":           rtspAuth,
	"http_proxy":     httpProxyAuth,
	"socks5_proxy":   SocksProxyBruteAuthFactory("socks5"),
	"socks4_proxy":   SocksProxyBruteAuthFactory("socks4"),
	"socks4a_proxy":  SocksProxyBruteAuthFactory("socks4a"),
	"pptp":           pptp_Auth,
	"ldap":           ldapAuth,
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
