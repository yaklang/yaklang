package yakcmds

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/crypto/ssh"
)

var SSHCommands = []*cli.Command{
	{
		Name:     "ssh",
		Usage:    "SSH client for remote server management",
		Category: "Remote Operations",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "host,H",
				Usage: "Remote host address (hostname:port or hostname)",
			},
			cli.IntFlag{
				Name:  "port,p",
				Usage: "SSH port (default: 22)",
				Value: 22,
			},
			cli.StringFlag{
				Name:  "user,u",
				Usage: "SSH username (default: root)",
				Value: "root",
			},
			cli.StringFlag{
				Name:  "password,P",
				Usage: "SSH password (if not specified, use private key authentication)",
			},
			cli.StringFlag{
				Name:  "private-key,i",
				Usage: "Path to private key file (default: ~/.ssh/id_rsa)",
			},
			cli.StringFlag{
				Name:  "key-passphrase",
				Usage: "Passphrase for encrypted private key",
			},
			cli.StringFlag{
				Name:  "command,c",
				Usage: "Execute a single command and exit",
			},
			cli.StringFlag{
				Name:  "bash-script,s",
				Usage: "Execute a bash script file and exit",
			},
			cli.StringFlag{
				Name:  "shell",
				Usage: "Shell to use (default: bash, fallback: sh)",
				Value: "bash",
			},
			cli.StringFlag{
				Name:  "upload-file",
				Usage: "Upload a file to remote server's home directory",
			},
		},
		Action: func(c *cli.Context) error {
			host := c.String("host")
			if host == "" {
				return utils.Error("host is required (use --host or -H)")
			}

			user := c.String("user")
			password := c.String("password")
			privateKey := c.String("private-key")
			keyPassphrase := c.String("key-passphrase")
			command := c.String("command")
			bashScript := c.String("bash-script")
			port := c.Int("port")
			shellType := c.String("shell")
			uploadFile := c.String("upload-file")

			// Parse host and port
			parsedHost, parsedPort, err := utils.ParseStringToHostPort(host)
			if err != nil {
				// If parsing fails, treat it as a hostname without port
				parsedHost = host
				parsedPort = port
			} else if parsedPort > 0 {
				// Use parsed port if available
				port = parsedPort
			}

			// Build final address
			addr := fmt.Sprintf("%s:%d", parsedHost, port)

			// Display banner if using root
			if user == "root" {
				log.Warn("⚠️  WARNING: Connecting as root user. Please be careful with your operations!")
			}

			log.Infof("Connecting to %s as user '%s'...", addr, user)

			// Connect to SSH server
			var client *utils.SSHClient
			if password != "" {
				// Password authentication
				log.Info("Using password authentication")
				client, err = utils.SSHDialWithPasswd(addr, user, password)
				if err != nil {
					return utils.Errorf("failed to connect with password: %s", err)
				}
			} else {
				// Private key authentication
				keyPath := privateKey
				if keyPath == "" {
					// Try default locations
					homeDir, err := os.UserHomeDir()
					if err != nil {
						return utils.Errorf("failed to get home directory: %s", err)
					}

					// Try common key files
					defaultKeys := []string{
						filepath.Join(homeDir, ".ssh", "id_rsa"),
						filepath.Join(homeDir, ".ssh", "id_ed25519"),
						filepath.Join(homeDir, ".ssh", "id_ecdsa"),
						filepath.Join(homeDir, ".ssh", "id_dsa"),
					}

					for _, key := range defaultKeys {
						if _, err := os.Stat(key); err == nil {
							keyPath = key
							log.Infof("Found private key: %s", keyPath)
							break
						}
					}

					if keyPath == "" {
						return utils.Error("no private key found in ~/.ssh/ and no password provided")
					}
				}

				log.Infof("Using private key authentication: %s", keyPath)
				if keyPassphrase != "" {
					client, err = utils.SSHDialWithKeyWithPassphrase(addr, user, keyPath, keyPassphrase)
				} else {
					client, err = utils.SSHDialWithKey(addr, user, keyPath)
				}

				if err != nil {
					return utils.Errorf("failed to connect with private key: %s", err)
				}
			}
			defer client.Close()

			log.Info("✓ SSH connection established successfully")

			// Upload file if specified
			if uploadFile != "" {
				log.Infof("Uploading file: %s", uploadFile)

				// Check if local file exists
				if _, err := os.Stat(uploadFile); os.IsNotExist(err) {
					log.Errorf("local file not found: %s", uploadFile)
					os.Exit(-1)
				}

				// Get the filename
				fileName := filepath.Base(uploadFile)

				// Get remote home directory
				homeCmd := client.Cmd("echo $HOME")
				homeOutput, err := homeCmd.Output()
				if err != nil {
					log.Errorf("failed to get remote home directory: %s", err)
					os.Exit(-1)
				}
				remoteHome := strings.TrimSpace(string(homeOutput))
				remotePath := filepath.Join(remoteHome, fileName)

				log.Infof("target remote path: %s", remotePath)

				// Check if remote file exists
				checkCmd := client.Cmd(fmt.Sprintf("test -f %s && echo exists || echo notexists", remotePath))
				checkOutput, err := checkCmd.Output()
				if err != nil {
					log.Errorf("failed to check remote file existence: %s", err)
					os.Exit(-1)
				}

				if strings.TrimSpace(string(checkOutput)) == "exists" {
					log.Errorf("remote file already exists: %s", remotePath)
					os.Exit(-1)
				}

				// Upload the file
				err = client.CopyLocalFileToRemote(uploadFile, remotePath)
				if err != nil {
					log.Errorf("failed to upload file: %s", err)
					os.Exit(-1)
				}

				log.Infof("✓ file uploaded successfully to %s", remotePath)
				os.Exit(0)
			}

			// Execute command if specified
			if command != "" {
				log.Infof("Executing command: %s", command)
				script := client.Cmd(command)
				script.SetStdio(os.Stdout, os.Stderr)
				err = script.Run()
				if err != nil {
					return utils.Errorf("failed to execute command: %s", err)
				}
				log.Info("✓ Command execution completed")
				return nil
			}

			// Execute bash script if specified
			if bashScript != "" {
				log.Infof("Executing bash script: %s", bashScript)

				// Check if file exists
				if _, err := os.Stat(bashScript); os.IsNotExist(err) {
					return utils.Errorf("bash script file not found: %s", bashScript)
				}

				script := client.ScriptFile(bashScript)
				script.SetStdio(os.Stdout, os.Stderr)
				err = script.Run()
				if err != nil {
					return utils.Errorf("failed to execute bash script: %s", err)
				}
				log.Info("✓ Bash script execution completed")
				return nil
			}

			// Interactive shell mode
			log.Info("Starting interactive shell (press Ctrl+D or type 'exit' to quit)...")
			log.Info("═══════════════════════════════════════════════════════════════")

			// Try bash first, then fallback to sh
			shell := shellType
			termConfig := &utils.TerminalConfig{
				Term:   "xterm-256color",
				Height: 40,
				Weight: 120,
				Modes: ssh.TerminalModes{
					ssh.ECHO:          1,
					ssh.TTY_OP_ISPEED: 14400,
					ssh.TTY_OP_OSPEED: 14400,
				},
			}

			// Test if the shell exists
			testCmd := client.Cmd(fmt.Sprintf("command -v %s", shell))
			output, err := testCmd.Output()
			if err != nil || len(output) == 0 {
				// Fallback to sh
				if shell == "bash" {
					log.Warn("bash not available, falling back to sh")
					shell = "sh"
				}
			}

			terminal := client.Terminal(termConfig)
			terminal.SetStdio(os.Stdin, os.Stdout, os.Stderr)

			err = terminal.Start()
			if err != nil {
				return utils.Errorf("failed to start interactive shell: %s", err)
			}

			log.Info("═══════════════════════════════════════════════════════════════")
			log.Info("✓ Interactive shell session ended")
			return nil
		},
	},
}

// SSHConnect establishes an SSH connection and returns the client
func SSHConnect(host, user, password string, opts ...SSHOption) (*utils.SSHClient, error) {
	config := &SSHConfig{
		Port:       22,
		PrivateKey: "",
		Passphrase: "",
	}

	for _, opt := range opts {
		opt(config)
	}

	// Parse host and port
	parsedHost, parsedPort, err := utils.ParseStringToHostPort(host)
	if err != nil {
		parsedHost = host
		parsedPort = config.Port
	} else if parsedPort > 0 {
		config.Port = parsedPort
	}

	addr := fmt.Sprintf("%s:%d", parsedHost, config.Port)

	var client *utils.SSHClient
	if password != "" {
		client, err = utils.SSHDialWithPasswd(addr, user, password)
	} else if config.PrivateKey != "" {
		if config.Passphrase != "" {
			client, err = utils.SSHDialWithKeyWithPassphrase(addr, user, config.PrivateKey, config.Passphrase)
		} else {
			client, err = utils.SSHDialWithKey(addr, user, config.PrivateKey)
		}
	} else {
		// Try default keys
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, utils.Errorf("failed to get home directory: %s", err)
		}

		defaultKeys := []string{
			filepath.Join(homeDir, ".ssh", "id_rsa"),
			filepath.Join(homeDir, ".ssh", "id_ed25519"),
			filepath.Join(homeDir, ".ssh", "id_ecdsa"),
		}

		var lastErr error
		for _, keyPath := range defaultKeys {
			if _, err := os.Stat(keyPath); err == nil {
				client, lastErr = utils.SSHDialWithKey(addr, user, keyPath)
				if lastErr == nil {
					return client, nil
				}
			}
		}

		if lastErr != nil {
			return nil, utils.Errorf("failed to connect with default keys: %s", lastErr)
		}
		return nil, utils.Error("no authentication method available")
	}

	return client, err
}

// SSHExec executes a command on remote server
func SSHExec(client *utils.SSHClient, command string) (string, error) {
	script := client.Cmd(command)
	output, err := script.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// SSHExecScript executes a script file on remote server
func SSHExecScript(client *utils.SSHClient, scriptPath string) (string, error) {
	script := client.ScriptFile(scriptPath)
	output, err := script.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// SSHConfig holds SSH connection configuration
type SSHConfig struct {
	Port       int
	PrivateKey string
	Passphrase string
}

// SSHOption is a function that configures SSHConfig
type SSHOption func(*SSHConfig)

// WithPort sets the SSH port
func WithPort(port int) SSHOption {
	return func(c *SSHConfig) {
		c.Port = port
	}
}

// WithPrivateKey sets the private key path
func WithPrivateKey(keyPath string) SSHOption {
	return func(c *SSHConfig) {
		c.PrivateKey = keyPath
	}
}

// WithPassphrase sets the passphrase for encrypted private key
func WithPassphrase(passphrase string) SSHOption {
	return func(c *SSHConfig) {
		c.Passphrase = passphrase
	}
}
