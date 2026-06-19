package yaklib

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/smb"
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

// username 是一个 SMB 连接配置选项，用于设置认证用户名
// 参数:
//   - user: 认证用户名
//
// 返回值:
//   - 一个 SMB 连接配置选项，作为可变参数传入 smb.Connect
//
// Example:
// ```
// // 指定用户名密码连接 SMB，此处仅作示意
// session = smb.Connect("192.168.1.1:445", smb.username("administrator"), smb.password("123456"))~
// defer session.Close()
// ```
func _smbConfig_Username(user string) _smbConfigHandler {
	return func(config *_smbConfig) {
		config.Username = user
	}
}

// password 是一个 SMB 连接配置选项，用于设置认证密码
// 参数:
//   - pass: 认证密码
//
// 返回值:
//   - 一个 SMB 连接配置选项，作为可变参数传入 smb.Connect
//
// Example:
// ```
// // 指定用户名密码连接 SMB，此处仅作示意
// session = smb.Connect("192.168.1.1:445", smb.username("administrator"), smb.password("123456"))~
// defer session.Close()
// ```
func _smbConfig_Password(pass string) _smbConfigHandler {
	return func(config *_smbConfig) {
		config.Password = pass
	}
}

// workstation 是一个 SMB 连接配置选项，用于设置工作站名称
// 参数:
//   - w: 工作站名称
//
// 返回值:
//   - 一个 SMB 连接配置选项，作为可变参数传入 smb.Connect
//
// Example:
// ```
// // 指定工作站名称连接 SMB，此处仅作示意
// session = smb.Connect("192.168.1.1:445", smb.username("administrator"), smb.password("123456"), smb.workstation("WIN-PC"))~
// defer session.Close()
// ```
func _smbConfig_Workstation(w string) _smbConfigHandler {
	return func(config *_smbConfig) {
		config.Workstation = w
	}
}

// domain 是一个 SMB 连接配置选项，用于设置认证所属的域
// 参数:
//   - w: 域名称
//
// 返回值:
//   - 一个 SMB 连接配置选项，作为可变参数传入 smb.Connect
//
// Example:
// ```
// // 指定域进行 SMB 域认证，此处仅作示意
// session = smb.Connect("192.168.1.1:445", smb.username("administrator"), smb.password("123456"), smb.domain("CORP"))~
// defer session.Close()
// ```
func _smbConfig_Domain(w string) _smbConfigHandler {
	return func(config *_smbConfig) {
		config.Domain = w
	}
}

// hash 是一个 SMB 连接配置选项，用于设置 NTLM 哈希以进行哈希传递（Pass-the-Hash）认证
// 参数:
//   - w: NTLM 哈希字符串
//
// 返回值:
//   - 一个 SMB 连接配置选项，作为可变参数传入 smb.Connect
//
// Example:
// ```
// // 使用 NTLM 哈希进行哈希传递认证连接 SMB，此处仅作示意
// session = smb.Connect("192.168.1.1:445", smb.username("administrator"), smb.hash("aad3b435b51404eeaad3b435b51404ee:..."))~
// defer session.Close()
// ```
func _smbConfig_Hash(w string) _smbConfigHandler {
	return func(config *_smbConfig) {
		config.Hash = w
	}
}

// debug 是一个 SMB 连接配置选项，用于开启调试日志输出
// 参数:
//   - w: 是否开启调试模式
//
// 返回值:
//   - 一个 SMB 连接配置选项，作为可变参数传入 smb.Connect
//
// Example:
// ```
// // 开启调试模式连接 SMB，此处仅作示意
// session = smb.Connect("192.168.1.1:445", smb.username("administrator"), smb.password("123456"), smb.debug(true))~
// defer session.Close()
// ```
func _smbConfig_Debug(w bool) _smbConfigHandler {
	return func(config *_smbConfig) {
		config.Debug = w
	}
}

// Connect 建立一个 SMB 会话，返回一个可进行文件共享操作的会话对象
// 参数:
//   - addr: 目标地址，格式为 host 或 host:port，未指定端口时默认 445
//   - opts: 可选配置，例如 smb.username、smb.password、smb.domain、smb.hash
//
// 返回值:
//   - SMB 会话对象，可进行共享枚举、文件读写等操作
//   - 错误信息，连接或认证失败时返回非空
//
// Example:
// ```
// // 建立 SMB 会话，依赖目标服务，此处仅作示意
// session = smb.Connect("192.168.1.1:445", smb.username("administrator"), smb.password("123456"))~
// defer session.Close()
// ```
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
