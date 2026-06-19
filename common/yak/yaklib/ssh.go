package yaklib

import (
	"github.com/yaklang/yaklang/common/utils"
)

// SSHExports is the export table for SSH client operations
var SSHExports = map[string]interface{}{
	// Connection functions
	"Connect":           SSHConnect,
	"ConnectWithKey":    SSHConnectWithKey,
	"ConnectWithPasswd": SSHConnectWithPasswd,

	// Options for Connect
	"username":      WithSSHUsername,
	"password":      WithSSHPassword,
	"privateKey":    WithSSHPrivateKey,
	"keyPassphrase": WithSSHKeyPassphrase,
	"port":          WithSSHPort,
	"timeout":       WithSSHTimeout,
}

// SSHConfig holds SSH connection configuration
type SSHConfig struct {
	Username      string
	Password      string
	PrivateKey    string
	KeyPassphrase string
	Port          int
	Timeout       float64
}

// SSHOption is a function that configures SSHConfig
type SSHOption func(*SSHConfig)

// username 是一个 SSH 连接配置选项，用于设置登录用户名
// 参数:
//   - username: 登录用户名
//
// 返回值:
//   - 一个 SSH 连接配置选项，作为可变参数传入 ssh.Connect
//
// Example:
// ```
// // 指定用户名密码建立 SSH 连接，此处仅作示意
// client = ssh.Connect("example.com:22", ssh.username("root"), ssh.password("pass"))~
// defer client.Close()
// ```
func WithSSHUsername(username string) SSHOption {
	return func(c *SSHConfig) {
		c.Username = username
	}
}

// password 是一个 SSH 连接配置选项，用于设置登录密码
// 参数:
//   - password: 登录密码
//
// 返回值:
//   - 一个 SSH 连接配置选项，作为可变参数传入 ssh.Connect
//
// Example:
// ```
// // 指定用户名密码建立 SSH 连接，此处仅作示意
// client = ssh.Connect("example.com:22", ssh.username("root"), ssh.password("pass"))~
// defer client.Close()
// ```
func WithSSHPassword(password string) SSHOption {
	return func(c *SSHConfig) {
		c.Password = password
	}
}

// privateKey 是一个 SSH 连接配置选项，用于设置私钥文件路径以进行密钥认证
// 参数:
//   - keyPath: 私钥文件路径
//
// 返回值:
//   - 一个 SSH 连接配置选项，作为可变参数传入 ssh.Connect
//
// Example:
// ```
// // 使用私钥建立 SSH 连接，此处仅作示意
// client = ssh.Connect("example.com:22", ssh.username("root"), ssh.privateKey("/path/to/id_rsa"))~
// defer client.Close()
// ```
func WithSSHPrivateKey(keyPath string) SSHOption {
	return func(c *SSHConfig) {
		c.PrivateKey = keyPath
	}
}

// keyPassphrase 是一个 SSH 连接配置选项，用于设置加密私钥的口令
// 参数:
//   - passphrase: 私钥口令
//
// 返回值:
//   - 一个 SSH 连接配置选项，作为可变参数传入 ssh.Connect
//
// Example:
// ```
// // 使用带口令的私钥建立 SSH 连接，此处仅作示意
// client = ssh.Connect("example.com:22", ssh.username("root"), ssh.privateKey("/path/to/id_rsa"), ssh.keyPassphrase("secret"))~
// defer client.Close()
// ```
func WithSSHKeyPassphrase(passphrase string) SSHOption {
	return func(c *SSHConfig) {
		c.KeyPassphrase = passphrase
	}
}

// port 是一个 SSH 连接配置选项，用于设置 SSH 服务器端口
// 参数:
//   - port: SSH 服务器端口，默认 22
//
// 返回值:
//   - 一个 SSH 连接配置选项，作为可变参数传入 ssh.Connect
//
// Example:
// ```
// // 指定端口建立 SSH 连接，此处仅作示意
// client = ssh.Connect("example.com", ssh.port(2222), ssh.username("root"), ssh.password("pass"))~
// defer client.Close()
// ```
func WithSSHPort(port int) SSHOption {
	return func(c *SSHConfig) {
		c.Port = port
	}
}

// timeout 是一个 SSH 连接配置选项，用于设置连接超时时间（单位：秒）
// 参数:
//   - timeout: 超时时间，单位为秒，支持小数
//
// 返回值:
//   - 一个 SSH 连接配置选项，作为可变参数传入 ssh.Connect
//
// Example:
// ```
// // 设置连接超时建立 SSH 连接，此处仅作示意
// client = ssh.Connect("example.com:22", ssh.username("root"), ssh.password("pass"), ssh.timeout(5))~
// defer client.Close()
// ```
func WithSSHTimeout(timeout float64) SSHOption {
	return func(c *SSHConfig) {
		c.Timeout = timeout
	}
}

// SSHClient wraps utils.SSHClient with script-friendly methods
type SSHClient struct {
	client *utils.SSHClient
}

// Run executes a single command on the remote server
func (s *SSHClient) Run(command string) (string, error) {
	script := s.client.Cmd(command)
	output, err := script.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// RunScript executes a bash script on the remote server
func (s *SSHClient) RunScript(scriptContent string) (string, error) {
	script := s.client.Script(scriptContent)
	output, err := script.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// RunScriptFile executes a local script file on the remote server
func (s *SSHClient) RunScriptFile(scriptPath string) (string, error) {
	script := s.client.ScriptFile(scriptPath)
	output, err := script.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// UploadFile uploads a local file to remote server
func (s *SSHClient) UploadFile(localPath, remotePath string) error {
	return s.client.CopyLocalFileToRemote(localPath, remotePath)
}

// DownloadFile downloads a file from remote server to local
func (s *SSHClient) DownloadFile(remotePath, localPath string) error {
	return s.client.CopyRemoteFileToLocal(localPath, remotePath)
}

// Close closes the SSH connection
func (s *SSHClient) Close() error {
	return s.client.Close()
}

// Connect 使用灵活的配置选项建立一个 SSH 连接，返回可执行命令与传输文件的客户端
// 参数:
//   - host: 目标地址，格式为 host 或 host:port，未指定端口时默认 22
//   - opts: 可选配置，例如 ssh.username、ssh.password、ssh.privateKey、ssh.port、ssh.timeout
//
// 返回值:
//   - SSH 客户端对象，可调用 Run/RunScript/UploadFile 等方法
//   - 错误信息，连接或认证失败时返回非空
//
// Example:
// ```
// // 建立 SSH 连接并执行命令，依赖目标服务，此处仅作示意
// client = ssh.Connect("example.com:22", ssh.username("root"), ssh.password("pass"), ssh.timeout(5))~
// defer client.Close()
// output = client.Run("whoami")~
// println(output)
// ```
func SSHConnect(host string, opts ...SSHOption) (*SSHClient, error) {
	config := &SSHConfig{
		Username: "root",
		Port:     22,
		Timeout:  10,
	}

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	// Parse host and port
	parsedHost, parsedPort, err := utils.ParseStringToHostPort(host)
	if err != nil {
		// If parsing fails, treat it as hostname without port
		parsedHost = host
		parsedPort = config.Port
	} else if parsedPort > 0 {
		// Use parsed port if available
		config.Port = parsedPort
	}

	addr := utils.HostPort(parsedHost, config.Port)

	var client *utils.SSHClient

	// Determine authentication method
	if config.Password != "" {
		// Password authentication
		client, err = utils.SSHDialWithPasswd(addr, config.Username, config.Password)
	} else if config.PrivateKey != "" {
		// Private key authentication
		if config.KeyPassphrase != "" {
			client, err = utils.SSHDialWithKeyWithPassphrase(addr, config.Username, config.PrivateKey, config.KeyPassphrase)
		} else {
			client, err = utils.SSHDialWithKey(addr, config.Username, config.PrivateKey)
		}
	} else {
		return nil, utils.Error("either password or privateKey must be provided")
	}

	if err != nil {
		return nil, err
	}

	return &SSHClient{client: client}, nil
}

// ConnectWithKey 使用私钥认证连接到 SSH 服务器，返回 SSH 客户端
// 参数:
//   - host: 目标地址，格式为 host 或 host:port，未指定端口时默认 22
//   - username: 登录用户名
//   - keyPath: 私钥文件路径
//
// 返回值:
//   - SSH 客户端对象，可调用 Run/RunScript/UploadFile 等方法
//   - 错误信息，连接或认证失败时返回非空
//
// Example:
// ```
// // 使用私钥连接 SSH 并执行命令，依赖目标服务，此处仅作示意
// client = ssh.ConnectWithKey("example.com:22", "root", "/path/to/id_rsa")~
// defer client.Close()
// output = client.Run("uname -a")~
// println(output)
// ```
func SSHConnectWithKey(host, username, keyPath string) (*SSHClient, error) {
	parsedHost, parsedPort, err := utils.ParseStringToHostPort(host)
	if err != nil {
		parsedHost = host
		parsedPort = 22
	}

	addr := utils.HostPort(parsedHost, parsedPort)

	client, err := utils.SSHDialWithKey(addr, username, keyPath)
	if err != nil {
		return nil, err
	}

	return &SSHClient{client: client}, nil
}

// ConnectWithPasswd 使用密码认证连接到 SSH 服务器，返回 SSH 客户端
// 参数:
//   - host: 目标地址，格式为 host 或 host:port，未指定端口时默认 22
//   - username: 登录用户名
//   - password: 登录密码
//
// 返回值:
//   - SSH 客户端对象，可调用 Run/RunScript/UploadFile 等方法
//   - 错误信息，连接或认证失败时返回非空
//
// Example:
// ```
// // 使用密码连接 SSH 并执行命令，依赖目标服务，此处仅作示意
// client = ssh.ConnectWithPasswd("example.com:22", "root", "password")~
// defer client.Close()
// output = client.Run("id")~
// println(output)
// ```
func SSHConnectWithPasswd(host, username, password string) (*SSHClient, error) {
	parsedHost, parsedPort, err := utils.ParseStringToHostPort(host)
	if err != nil {
		parsedHost = host
		parsedPort = 22
	}

	addr := utils.HostPort(parsedHost, parsedPort)

	client, err := utils.SSHDialWithPasswd(addr, username, password)
	if err != nil {
		return nil, err
	}

	return &SSHClient{client: client}, nil
}
