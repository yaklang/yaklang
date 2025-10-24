package yaklib

import (
	"github.com/yaklang/yaklang/common/log"
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

// WithSSHUsername sets the SSH username
func WithSSHUsername(username string) SSHOption {
	return func(c *SSHConfig) {
		c.Username = username
	}
}

// WithSSHPassword sets the SSH password
func WithSSHPassword(password string) SSHOption {
	return func(c *SSHConfig) {
		c.Password = password
	}
}

// WithSSHPrivateKey sets the path to SSH private key
func WithSSHPrivateKey(keyPath string) SSHOption {
	return func(c *SSHConfig) {
		c.PrivateKey = keyPath
	}
}

// WithSSHKeyPassphrase sets the passphrase for encrypted private key
func WithSSHKeyPassphrase(passphrase string) SSHOption {
	return func(c *SSHConfig) {
		c.KeyPassphrase = passphrase
	}
}

// WithSSHPort sets the SSH port
func WithSSHPort(port int) SSHOption {
	return func(c *SSHConfig) {
		c.Port = port
	}
}

// WithSSHTimeout sets the connection timeout in seconds
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

// SSHConnect establishes an SSH connection with flexible options
// Example:
//
//	client, err = ssh.Connect("example.com:22", ssh.username("root"), ssh.password("pass"))
//	client, err = ssh.Connect("example.com", ssh.username("admin"), ssh.privateKey("/path/to/key"))
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

// SSHConnectWithKey connects to SSH server using private key
// Example:
//
//	client, err = ssh.ConnectWithKey("example.com:22", "root", "/path/to/id_rsa")
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

// SSHConnectWithPasswd connects to SSH server using password
// Example:
//
//	client, err = ssh.ConnectWithPasswd("example.com:22", "root", "password")
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

func init() {
	log.Info("SSH library initialized")
}
