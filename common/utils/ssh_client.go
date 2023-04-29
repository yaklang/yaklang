// sshclient implements an ssh client
package utils

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type SSHRemoteScriptType byte
type SSHRemoteShellType byte

const (
	cmdLine SSHRemoteScriptType = iota
	rawScript
	scriptFile

	interactiveShell SSHRemoteShellType = iota
	nonInteractiveShell
)

type SSHClient struct {
	client *ssh.Client
}

// SSHDialWithPasswd starts a client connection to the given SSH server with passwd authmethod.
func SSHDialWithPasswd(addr, user, passwd string) (*SSHClient, error) {
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(passwd),
		},
		HostKeyCallback: ssh.HostKeyCallback(func(hostname string, remote net.Addr, key ssh.PublicKey) error { return nil }),
	}

	return SSHDial("tcp", addr, config)
}

// SSHDialWithKey starts a client connection to the given SSH server with key authmethod.
func SSHDialWithKey(addr, user, keyfile string) (*SSHClient, error) {
	key, err := ioutil.ReadFile(keyfile)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.HostKeyCallback(func(hostname string, remote net.Addr, key ssh.PublicKey) error { return nil }),
	}

	return SSHDial("tcp", addr, config)
}

// SSHDialWithKeyWithPassphrase same as SSHDialWithKey but with a passphrase to decrypt the private key
func SSHDialWithKeyWithPassphrase(addr, user, keyfile string, passphrase string) (*SSHClient, error) {
	key, err := ioutil.ReadFile(keyfile)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKeyWithPassphrase(key, []byte(passphrase))
	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.HostKeyCallback(func(hostname string, remote net.Addr, key ssh.PublicKey) error { return nil }),
	}

	return SSHDial("tcp", addr, config)
}

// SSHDial starts a client connection to the given SSH server.
// This is wrap the ssh.SSHDial
func SSHDial(network, addr string, config *ssh.ClientConfig) (*SSHClient, error) {
	client, err := ssh.Dial(network, addr, config)
	if err != nil {
		return nil, err
	}
	return &SSHClient{
		client: client,
	}, nil
}

func (c *SSHClient) Close() error {
	return c.client.Close()
}

// Cmd create a command on client
func (c *SSHClient) Cmd(cmd string) *SSHRemoteScript {
	return &SSHRemoteScript{
		_type:  cmdLine,
		client: c.client,
		script: bytes.NewBufferString(cmd + "\n"),
	}
}

// Script
func (c *SSHClient) Script(script string) *SSHRemoteScript {
	return &SSHRemoteScript{
		_type:  rawScript,
		client: c.client,
		script: bytes.NewBufferString(script + "\n"),
	}
}

// ScriptFile
func (c *SSHClient) ScriptFile(fname string) *SSHRemoteScript {
	return &SSHRemoteScript{
		_type:      scriptFile,
		client:     c.client,
		scriptFile: fname,
	}
}

//Copy local file to remote
func (c *SSHClient) CopyLocalFileToRemote(srcFilePath string, dstFilePath string) error {
	sftpClient, err := sftp.NewClient(c.client)
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	if err := sftpClient.MkdirAll(path.Dir(dstFilePath)); err != nil {
		return err
	}

	dstFile, err := sftpClient.Create(dstFilePath)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	srcFile, err := os.Open(srcFilePath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	bytes, err := io.Copy(dstFile, srcFile)
	_ = bytes

	return err
}

//Copy remote file to local
func (c *SSHClient) CopyRemoteFileToLocal(dstFilePath string, srcFilePath string) error {
	sftpClient, err := sftp.NewClient(c.client)
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	if err := os.MkdirAll(path.Dir(dstFilePath), 0777); err != nil {
		return err
	}

	dstFile, err := os.Create(dstFilePath)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	srcFile, err := sftpClient.Open(srcFilePath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	bytes, err := io.Copy(dstFile, srcFile)
	_ = bytes

	err = dstFile.Sync()

	return err
}

type SSHRemoteScript struct {
	client     *ssh.Client
	_type      SSHRemoteScriptType
	script     *bytes.Buffer
	scriptFile string
	err        error

	stdout io.Writer
	stderr io.Writer
}

// Run
func (rs *SSHRemoteScript) Run() error {
	if rs.err != nil {
		fmt.Println(rs.err)
		return rs.err
	}

	if rs._type == cmdLine {
		return rs.runCmds()
	} else if rs._type == rawScript {
		return rs.runScript()
	} else if rs._type == scriptFile {
		return rs.runScriptFile()
	} else {
		return errors.New("Not supported SSHRemoteScript type")
	}
}

func (rs *SSHRemoteScript) Output() ([]byte, error) {
	if rs.stdout != nil {
		return nil, errors.New("Stdout already set")
	}
	var out bytes.Buffer
	rs.stdout = &out
	err := rs.Run()
	return out.Bytes(), err
}

func (rs *SSHRemoteScript) SmartOutput() ([]byte, error) {
	if rs.stdout != nil {
		return nil, errors.New("Stdout already set")
	}
	if rs.stderr != nil {
		return nil, errors.New("Stderr already set")
	}

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	rs.stdout = &stdout
	rs.stderr = &stderr
	err := rs.Run()
	if err != nil {
		return stderr.Bytes(), err
	}
	return stdout.Bytes(), err
}

func (rs *SSHRemoteScript) Cmd(cmd string) *SSHRemoteScript {
	_, err := rs.script.WriteString(cmd + "\n")
	if err != nil {
		rs.err = err
	}
	return rs
}

func (rs *SSHRemoteScript) SetStdio(stdout, stderr io.Writer) *SSHRemoteScript {
	rs.stdout = stdout
	rs.stderr = stderr
	return rs
}

func (rs *SSHRemoteScript) runCmd(cmd string) error {
	session, err := rs.client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	session.Stdout = rs.stdout
	session.Stderr = rs.stderr

	if err := session.Run(cmd); err != nil {
		return err
	}
	return nil
}

func (rs *SSHRemoteScript) runCmds() error {
	for {
		statment, err := rs.script.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if err := rs.runCmd(statment); err != nil {
			return err
		}
	}

	return nil
}

func (rs *SSHRemoteScript) runScript() error {
	session, err := rs.client.NewSession()
	if err != nil {
		return err
	}

	session.Stdin = rs.script
	session.Stdout = rs.stdout
	session.Stderr = rs.stderr

	if err := session.Shell(); err != nil {
		return err
	}
	if err := session.Wait(); err != nil {
		return err
	}

	return nil
}

func (rs *SSHRemoteScript) runScriptFile() error {
	var buffer bytes.Buffer
	file, err := os.Open(rs.scriptFile)
	if err != nil {
		return err
	}
	_, err = io.Copy(&buffer, file)
	if err != nil {
		return err
	}

	rs.script = &buffer
	return rs.runScript()
}

type SSHRemoteShell struct {
	client         *ssh.Client
	requestPty     bool
	terminalConfig *TerminalConfig

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

type TerminalConfig struct {
	Term   string
	Height int
	Weight int
	Modes  ssh.TerminalModes
}

// Terminal create a interactive shell on client.
func (c *SSHClient) Terminal(config *TerminalConfig) *SSHRemoteShell {
	return &SSHRemoteShell{
		client:         c.client,
		terminalConfig: config,
		requestPty:     true,
	}
}

// Shell create a noninteractive shell on client.
func (c *SSHClient) Shell() *SSHRemoteShell {
	return &SSHRemoteShell{
		client:     c.client,
		requestPty: false,
	}
}

func (rs *SSHRemoteShell) SetStdio(stdin io.Reader, stdout, stderr io.Writer) *SSHRemoteShell {
	rs.stdin = stdin
	rs.stdout = stdout
	rs.stderr = stderr
	return rs
}

// Start start a remote shell on client
func (rs *SSHRemoteShell) Start() error {
	session, err := rs.client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	if rs.stdin == nil {
		session.Stdin = os.Stdin
	} else {
		session.Stdin = rs.stdin
	}
	if rs.stdout == nil {
		session.Stdout = os.Stdout
	} else {
		session.Stdout = rs.stdout
	}
	if rs.stderr == nil {
		session.Stderr = os.Stderr
	} else {
		session.Stderr = rs.stderr
	}

	if rs.requestPty {
		tc := rs.terminalConfig
		if tc == nil {
			tc = &TerminalConfig{
				Term:   "xterm",
				Height: 40,
				Weight: 80,
			}
		}
		if err := session.RequestPty(tc.Term, tc.Height, tc.Weight, tc.Modes); err != nil {
			return err
		}
	}

	if err := session.Shell(); err != nil {
		return err
	}

	if err := session.Wait(); err != nil {
		return err
	}

	return nil
}
