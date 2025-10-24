package tools

import (
	"github.com/yaklang/yaklang/common/systemd"
)

// GenerateSystemdServiceConfig generates a systemd service configuration file content
// using the yaklang systemd library
func GenerateSystemdServiceConfig(serviceName, execCommand, serviceType, user, group, restart string) string {
	_, content := systemd.Create(
		serviceName,
		systemd.WithServiceExecStart(execCommand),
		systemd.WithServiceType(serviceType),
		systemd.WithServiceUser(user),
		systemd.WithServiceGroup(group),
		systemd.WithServiceRestart(restart),
		systemd.WithServiceRestartSec(5),
		systemd.WithServiceKillMode("mixed"),
		systemd.WithServiceKillSignal("SIGTERM"),
	)
	return content
}
