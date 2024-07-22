package bruteutils

import (
	"github.com/go-ldap/ldap"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
)

func testLdap(target string, username string, password string) bool {
	conn, err := LdapLogin(target, Ldap_Username(username), Ldap_Password(password))
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}

var ldapAuth = &DefaultServiceAuthInfo{
	ServiceName:  "ldap",
	DefaultPorts: "389",
	DefaultUsernames: []string{
		"admin",
	},
	DefaultPasswords: []string{
		"admin",
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		result := i.Result()
		result.Ok = testLdap(i.Target, i.Username, i.Password)
		return result
	},
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		result := i.Result()
		result.Ok = testLdap(i.Target, "", "")
		return result
	},
}

type ldapClientConfig struct {
	// port 389
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string
}

func optLdap_Port(i int) func(config *ldapClientConfig) {
	return func(config *ldapClientConfig) {
		config.Port = i
	}
}

func optLdap_Username(i string) func(config *ldapClientConfig) {
	return func(config *ldapClientConfig) {
		config.Username = i
	}
}

func optLdap_Password(i string) func(config *ldapClientConfig) {
	return func(config *ldapClientConfig) {
		config.Password = i
	}
}

var LdapLogin = _login
var Ldap_Username = optLdap_Username
var Ldap_Password = optLdap_Password
var Ldap_Port = optLdap_Port

func _login(addr string, opts ...func(config *ldapClientConfig)) (*ldap.Conn, error) {
	config := &ldapClientConfig{Host: addr}
	for _, i := range opts {
		i(config)
	}

	host, port, _ := utils.ParseStringToHostPort(addr)
	if port > 0 {
		config.Port = port
		config.Host = host
	}

	conn, err := ldapDial(utils.HostPort(config.Host, config.Port))
	if err != nil {
		return nil, err
	}

	if config.Username == "anonymous" || (config.Username == "" && config.Password == "") {
		err = conn.UnauthenticatedBind("")
	} else {
		err = conn.Bind(config.Username, config.Password)
	}
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func ldapDial(target string, opts ...netx.DialXOption) (*ldap.Conn, error) {
	c, err := netx.DialX(target, opts...)
	if err != nil {
		return nil, utils.Wrap(err, "dial ldap server failed")
	}
	conn := ldap.NewConn(c, false)
	conn.Start()
	return conn, nil
}
