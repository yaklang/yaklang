package yaklib

import (
	"yaklang.io/yaklang/common/utils"

	"github.com/go-ldap/ldap"
)

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

var LdapExports = map[string]interface{}{
	"Login": _login,

	"port":     optLdap_Port,
	"username": optLdap_Username,
	"password": optLdap_Password,
}

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

	conn, err := ldap.Dial("tcp", utils.HostPort(config.Host, config.Port))
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
	conn.Start()

	return conn, nil
}
