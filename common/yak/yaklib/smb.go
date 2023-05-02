package yaklib

import (
	"github.com/stacktitan/smb/smb"
	"yaklang.io/yaklang/common/utils"
)

type _smbConfig struct {
	Username    string
	Password    string
	Workstation string
	Domain      string
	Hash        string
	Debug       bool
}

type _smbConfigHandler func(config *_smbConfig)

func _smbConfig_Username(user string) _smbConfigHandler {
	return func(config *_smbConfig) {
		config.Username = user
	}
}

func _smbConfig_Password(pass string) _smbConfigHandler {
	return func(config *_smbConfig) {
		config.Password = pass
	}
}

func _smbConfig_Workstation(w string) _smbConfigHandler {
	return func(config *_smbConfig) {
		config.Workstation = w
	}
}

func _smbConfig_Domain(w string) _smbConfigHandler {
	return func(config *_smbConfig) {
		config.Domain = w
	}
}

func _smbConfig_Hash(w string) _smbConfigHandler {
	return func(config *_smbConfig) {
		config.Hash = w
	}
}

func _smbConfig_Debug(w bool) _smbConfigHandler {
	return func(config *_smbConfig) {
		config.Debug = w
	}
}

func smbConn(addr string, opts ..._smbConfigHandler) (*smb.Session, error) {
	host, port, _ := utils.ParseStringToHostPort(addr)
	if port <= 0 {
		port = 445
	}
	if host == "" {
		host = addr
	}

	config := &_smbConfig{}
	for _, i := range opts {
		i(config)
	}

	opt := smb.Options{
		Host:        host,
		Port:        port,
		Workstation: config.Workstation,
		Domain:      config.Domain,
		User:        config.Username,
		Password:    config.Password,
		Hash:        config.Hash,
	}
	session, err := smb.NewSession(opt, config.Debug)
	if err != nil {
		return nil, utils.Errorf("create smb://%v failed: %s", addr, err)
	}
	return session, nil
}

var SambaExports = map[string]interface{}{
	"username":    _smbConfig_Username,
	"password":    _smbConfig_Password,
	"domain":      _smbConfig_Domain,
	"workstation": _smbConfig_Workstation,
	"hash":        _smbConfig_Hash,
	"debug":       _smbConfig_Debug,
	"Connect":     smbConn,
}
