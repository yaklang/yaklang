package yakcmds

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/tools"
)

// runSystemCommand executes a shell command and returns error if failed
func runSystemCommand(cmdStr string) error {
	cmd := exec.Command("bash", "-c", cmdStr)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return utils.Errorf("command failed: %s, output: %s", err, string(output))
	}
	return nil
}

var SystemdCommands = []*cli.Command{
	{
		Name:    "install-to-systemd",
		Aliases: []string{"systemd-install"},
		Usage:   "Install Yak script as systemd service (Linux only)",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:     "service-name",
				Usage:    "Service name (required)",
				Required: true,
			},
			cli.StringFlag{
				Name:     "script-path",
				Usage:    "Yak script path (required)",
				Required: true,
			},
			cli.StringFlag{
				Name:  "script-args",
				Usage: "Script arguments (optional)",
			},
			cli.StringFlag{
				Name:  "user",
				Usage: "User to run the service",
				Value: "root",
			},
			cli.StringFlag{
				Name:  "group",
				Usage: "Group to run the service",
				Value: "root",
			},
			cli.StringFlag{
				Name:  "restart",
				Usage: "Restart policy (always/on-failure/no)",
				Value: "always",
			},
			cli.StringFlag{
				Name:  "type",
				Usage: "Service type (simple/oneshot/forking)",
				Value: "simple",
			},
			cli.BoolFlag{
				Name:  "no-start",
				Usage: "Do not start the service",
			},
			cli.BoolFlag{
				Name:  "disable",
				Usage: "Do not enable auto-start",
			},
			cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show configuration only, do not install",
			},
		},
		Action: func(c *cli.Context) error {
			// Check if running on Linux
			if runtime.GOOS != "linux" {
				log.Error("Error: This tool only supports Linux systems")
				log.Info("systemd service management is only available on Linux systems")
				return utils.Error("systemd is only available on Linux")
			}

			serviceName := c.String("service-name")
			scriptPath := c.String("script-path")
			scriptArgs := c.String("script-args")
			serviceUser := c.String("user")
			serviceGroup := c.String("group")
			restartPolicy := c.String("restart")
			serviceType := c.String("type")
			noStart := c.Bool("no-start")
			disable := c.Bool("disable")
			dryRun := c.Bool("dry-run")

			// Set default behavior
			startNow := !noStart
			enableAutoStart := !disable

			log.Info("=== Configuration Summary ===")
			log.Infof("Service name: %s", serviceName)
			log.Infof("Script path: %s", scriptPath)
			log.Infof("Script arguments: %s", scriptArgs)
			log.Infof("Service type: %s", serviceType)
			log.Infof("Running user: %s", serviceUser)
			log.Infof("Running group: %s", serviceGroup)
			log.Infof("Restart policy: %s", restartPolicy)
			log.Infof("Enable auto-start: %v", enableAutoStart)
			log.Infof("Start immediately: %v", startNow)
			log.Infof("DRY RUN mode: %v", dryRun)
			log.Info("")

			// Validate script path
			absScriptPath, err := filepath.Abs(scriptPath)
			if err != nil {
				return utils.Errorf("failed to get absolute path: %s", err)
			}

			if _, err := os.Stat(absScriptPath); os.IsNotExist(err) {
				return utils.Errorf("script file does not exist: %s", absScriptPath)
			}

			log.Infof("✓ Script path: %s", absScriptPath)

			// Get yak executable path
			yakBinary, err := os.Executable()
			if err != nil {
				return utils.Errorf("failed to get yak executable path: %s", err)
			}

			log.Infof("✓ Yak binary: %s", yakBinary)

			// Build execution command
			execCommand := fmt.Sprintf("%s %s", yakBinary, absScriptPath)
			if scriptArgs != "" {
				execCommand = fmt.Sprintf("%s %s", execCommand, scriptArgs)
			}

			log.Infof("Execution command: %s", execCommand)

			// Create systemd service configuration
			serviceContent := tools.GenerateSystemdServiceConfig(
				serviceName,
				execCommand,
				serviceType,
				serviceUser,
				serviceGroup,
				restartPolicy,
			)

			fileName := serviceName + ".service"
			serviceFilePath := filepath.Join("/etc/systemd/system", fileName)

			if dryRun {
				log.Info("--- DRY RUN mode, service configuration content ---")
				fmt.Println(serviceContent)
				log.Info("--- DRY RUN mode, will not actually install ---")
				return nil
			}

			// Save service file
			log.Infof("Installing service to: %s", serviceFilePath)
			err = os.WriteFile(serviceFilePath, []byte(serviceContent), 0644)
			if err != nil {
				return utils.Errorf("failed to save service file: %s", err)
			}
			log.Info("✓ Service file saved successfully")

			// Reload systemd configuration
			log.Info("Reloading systemd configuration...")
			err = runSystemCommand("systemctl daemon-reload")
			if err != nil {
				return utils.Errorf("failed to reload systemd: %s", err)
			}
			log.Info("✓ systemd configuration reloaded")

			// Start service if requested
			if startNow {
				log.Infof("Starting service: %s", serviceName)
				err = runSystemCommand(fmt.Sprintf("systemctl start %s", serviceName))
				if err != nil {
					log.Errorf("Error: Failed to start service: %s", err)
				} else {
					log.Info("✓ Service started successfully")
				}
			}

			// Enable auto-start if requested
			if enableAutoStart {
				log.Infof("Enabling service auto-start: %s", serviceName)
				err = runSystemCommand(fmt.Sprintf("systemctl enable %s", serviceName))
				if err != nil {
					log.Errorf("Error: Failed to enable service auto-start: %s", err)
				} else {
					log.Info("✓ Service auto-start enabled successfully")
				}
			}

			// Show management commands
			log.Info("=== Service Management Commands ===")
			log.Info("View service status:")
			log.Infof("  systemctl status %s", serviceName)
			log.Info("")
			log.Info("Start/stop/restart service:")
			log.Infof("  systemctl start %s", serviceName)
			log.Infof("  systemctl stop %s", serviceName)
			log.Infof("  systemctl restart %s", serviceName)
			log.Info("")
			log.Info("Enable/disable auto-start:")
			log.Infof("  systemctl enable %s", serviceName)
			log.Infof("  systemctl disable %s", serviceName)
			log.Info("")
			log.Info("View service logs:")
			log.Infof("  journalctl -u %s -f", serviceName)
			log.Infof("  journalctl -u %s --since today", serviceName)
			log.Info("")
			log.Info("Uninstall service:")
			log.Infof("  yak uninstall-from-systemd --service-name %s", serviceName)

			log.Info("✓ Service installation completed")
			return nil
		},
	},
	{
		Name:    "uninstall-from-systemd",
		Aliases: []string{"systemd-uninstall"},
		Usage:   "Uninstall Yak script from systemd service (Linux only)",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:     "service-name",
				Usage:    "Service name (required)",
				Required: true,
			},
			cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show commands only, do not actually uninstall",
			},
		},
		Action: func(c *cli.Context) error {
			// Check if running on Linux
			if runtime.GOOS != "linux" {
				log.Error("Error: This tool only supports Linux systems")
				log.Info("systemd service management is only available on Linux systems")
				return utils.Error("systemd is only available on Linux")
			}

			serviceName := c.String("service-name")
			dryRun := c.Bool("dry-run")

			log.Info("=== Uninstall Configuration ===")
			log.Infof("Service name: %s", serviceName)
			log.Infof("DRY RUN mode: %v", dryRun)
			log.Info("")

			serviceFilePath := filepath.Join("/etc/systemd/system", serviceName+".service")

			if dryRun {
				log.Info("--- DRY RUN mode, uninstall commands ---")
				log.Infof("systemctl stop %s", serviceName)
				log.Infof("systemctl disable %s", serviceName)
				log.Infof("rm %s", serviceFilePath)
				log.Info("systemctl daemon-reload")
				log.Info("--- DRY RUN mode, will not actually execute ---")
				return nil
			}

			// Stop service
			log.Infof("Stopping service: %s", serviceName)
			err := runSystemCommand(fmt.Sprintf("systemctl stop %s", serviceName))
			if err != nil {
				log.Warnf("Warning: Failed to stop service (may already be stopped): %s", err)
			} else {
				log.Info("✓ Service stopped")
			}

			// Disable service
			log.Infof("Disabling service: %s", serviceName)
			err = runSystemCommand(fmt.Sprintf("systemctl disable %s", serviceName))
			if err != nil {
				log.Warnf("Warning: Failed to disable service: %s", err)
			} else {
				log.Info("✓ Service disabled")
			}

			// Remove service file
			log.Infof("Removing service file: %s", serviceFilePath)
			err = os.Remove(serviceFilePath)
			if err != nil {
				if os.IsNotExist(err) {
					log.Warnf("Warning: Service file does not exist: %s", serviceFilePath)
				} else {
					return utils.Errorf("failed to remove service file: %s", err)
				}
			} else {
				log.Info("✓ Service file removed")
			}

			// Reload systemd configuration
			log.Info("Reloading systemd configuration...")
			err = runSystemCommand("systemctl daemon-reload")
			if err != nil {
				return utils.Errorf("failed to reload systemd: %s", err)
			}
			log.Info("✓ systemd configuration reloaded")

			log.Info("✓ Service uninstalled successfully")
			return nil
		},
	},
}

// InstallToSystemd installs a Yak script as a systemd service programmatically
func InstallToSystemd(serviceName, scriptPath, scriptArgs string, opts ...SystemdOption) error {
	if runtime.GOOS != "linux" {
		return utils.Error("systemd is only available on Linux")
	}

	config := &SystemdConfig{
		ServiceUser:     "root",
		ServiceGroup:    "root",
		RestartPolicy:   "always",
		ServiceType:     "simple",
		StartNow:        true,
		EnableAutoStart: true,
	}

	for _, opt := range opts {
		opt(config)
	}

	// Get absolute script path
	absScriptPath, err := filepath.Abs(scriptPath)
	if err != nil {
		return utils.Errorf("failed to get absolute path: %s", err)
	}

	// Get yak executable path
	yakBinary, err := os.Executable()
	if err != nil {
		return utils.Errorf("failed to get yak executable path: %s", err)
	}

	// Build execution command
	execCommand := fmt.Sprintf("%s %s", yakBinary, absScriptPath)
	if scriptArgs != "" {
		execCommand = fmt.Sprintf("%s %s", execCommand, scriptArgs)
	}

	// Create systemd service configuration
	serviceContent := tools.GenerateSystemdServiceConfig(
		serviceName,
		execCommand,
		config.ServiceType,
		config.ServiceUser,
		config.ServiceGroup,
		config.RestartPolicy,
	)

	// Save service file
	serviceFilePath := filepath.Join("/etc/systemd/system", serviceName+".service")
	err = os.WriteFile(serviceFilePath, []byte(serviceContent), 0644)
	if err != nil {
		return utils.Errorf("failed to save service file: %s", err)
	}

	// Reload systemd
	err = runSystemCommand("systemctl daemon-reload")
	if err != nil {
		return utils.Errorf("failed to reload systemd: %s", err)
	}

	// Start service if requested
	if config.StartNow {
		err = runSystemCommand(fmt.Sprintf("systemctl start %s", serviceName))
		if err != nil {
			return utils.Errorf("failed to start service: %s", err)
		}
	}

	// Enable auto-start if requested
	if config.EnableAutoStart {
		err = runSystemCommand(fmt.Sprintf("systemctl enable %s", serviceName))
		if err != nil {
			return utils.Errorf("failed to enable service: %s", err)
		}
	}

	return nil
}

// UninstallFromSystemd uninstalls a Yak script from systemd service programmatically
func UninstallFromSystemd(serviceName string) error {
	if runtime.GOOS != "linux" {
		return utils.Error("systemd is only available on Linux")
	}

	// Stop service
	_ = runSystemCommand(fmt.Sprintf("systemctl stop %s", serviceName))

	// Disable service
	_ = runSystemCommand(fmt.Sprintf("systemctl disable %s", serviceName))

	// Remove service file
	serviceFilePath := filepath.Join("/etc/systemd/system", serviceName+".service")
	err := os.Remove(serviceFilePath)
	if err != nil && !os.IsNotExist(err) {
		return utils.Errorf("failed to remove service file: %s", err)
	}

	// Reload systemd
	err = runSystemCommand("systemctl daemon-reload")
	if err != nil {
		return utils.Errorf("failed to reload systemd: %s", err)
	}

	return nil
}

// SystemdConfig holds configuration for systemd service
type SystemdConfig struct {
	ServiceUser     string
	ServiceGroup    string
	RestartPolicy   string
	ServiceType     string
	StartNow        bool
	EnableAutoStart bool
}

// SystemdOption is a functional option for configuring systemd service
type SystemdOption func(*SystemdConfig)

// WithServiceUser sets the user for the service
func WithServiceUser(user string) SystemdOption {
	return func(c *SystemdConfig) {
		c.ServiceUser = user
	}
}

// WithServiceGroup sets the group for the service
func WithServiceGroup(group string) SystemdOption {
	return func(c *SystemdConfig) {
		c.ServiceGroup = group
	}
}

// WithRestartPolicy sets the restart policy for the service
func WithRestartPolicy(policy string) SystemdOption {
	return func(c *SystemdConfig) {
		c.RestartPolicy = policy
	}
}

// WithServiceType sets the type for the service
func WithServiceType(serviceType string) SystemdOption {
	return func(c *SystemdConfig) {
		c.ServiceType = serviceType
	}
}

// WithNoStart disables starting the service immediately
func WithNoStart() SystemdOption {
	return func(c *SystemdConfig) {
		c.StartNow = false
	}
}

// WithDisable disables auto-start for the service
func WithDisable() SystemdOption {
	return func(c *SystemdConfig) {
		c.EnableAutoStart = false
	}
}
