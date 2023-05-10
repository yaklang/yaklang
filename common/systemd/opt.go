package systemd

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type ConfigOption func(*SystemdServiceConfig)

func WithUnitDescription(description string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.Description = description
	}
}

func WithUnitDocumentation(documentation string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.Documentation = documentation
	}
}

func WithUnitAfter(after string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.After = after
	}
}

func WithUnitBefore(before string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.Before = before
	}
}

func WithUnitRequires(requires string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.Requires = requires
	}
}

func WithUnitBindsTo(bindsTo string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.BindsTo = bindsTo
	}
}

func WithUnitWants(wants string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.Wants = wants
	}
}

func WithUnitExtraLine(extraLine string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.UnitExtraLine = utils.ParseStringToLines(extraLine)
	}
}

func WithServiceType(serviceType string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		switch serviceType {
		case "simple", "forking", "oneshot", "dbus", "notify", "idle":
			c.ServicesType = serviceType
		default:
			log.Warnf("service type %s is not supported, use default simple", serviceType)
		}
	}
}

func WithServiceUser(user string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.User = user
	}
}

func WithServiceGroup(group string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.Group = group
	}
}

func WithServiceExecStart(execStart string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.ExecStart = execStart
	}
}

func WithServiceExecStartPre(execStartPre string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.ExecStartPre = execStartPre
	}
}

func WithServiceExecStartPost(execStartPost string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.ExecStartPost = execStartPost
	}
}

func WithServiceExecStop(execStop string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.ExecStop = execStop
	}
}

func WithServiceExecStopPost(execStopPost string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.ExecStopPost = execStopPost
	}
}

func WithServiceRestart(restart string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		switch restart {
		case "no", "on-success", "on-failure", "on-abnormal", "on-watchdog", "on-abort", "always":
			c.Restart = restart
		default:
			log.Warnf("restart type %s is not supported, use default no", restart)
		}
	}
}

func WithServiceRestartSec(restartSec float64) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.RestartSec = restartSec
	}
}

func WithServiceTimeoutStartSec(timeoutStartSec float64) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.TimeoutStartSec = timeoutStartSec
	}
}

func WithServiceEnvironment(environment string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.Environment = environment
	}
}

func WithServiceEnvironmentFile(environmentFile string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.EnvironmentFile = environmentFile
	}
}

func WithServiceUmask(umask string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.Umask = umask
	}
}

func WithServiceStandardInput(standardInput string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		switch standardInput {
		case "null", "tty", "tty-force", "tty-fail", "socket", "fd", "fd-force", "fd-fail", "file", "file-force", "file-fail":
			c.StandardInput = standardInput
		default:
			log.Warnf("standard input type %s is not supported, use default null", standardInput)
		}
	}
}

func WithServiceStandardOutput(standardOutput string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		switch standardOutput {
		case "inherit", "null", "tty", "journal", "syslog", "kmsg", "journal+console", "syslog+console", "kmsg+console", "socket", "fd", "file":
			c.StandardOutput = standardOutput
		default:
			log.Warnf("standard output type %s is not supported, use default inherit", standardOutput)
		}
	}
}

func WithServiceStandardError(standardError string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		switch standardError {
		case "inherit", "null", "tty", "journal", "syslog", "kmsg", "journal+console", "syslog+console", "kmsg+console", "socket", "fd", "file":
			c.StandardError = standardError
		default:
			log.Warnf("standard error type %s is not supported, use default inherit", standardError)
		}
	}
}

func WithServiceKillMode(killMode string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		switch killMode {
		case "control-group", "process", "mixed", "none":
			c.KillMode = killMode
		default:
			log.Warnf("kill mode %s is not supported, use default control-group", killMode)
		}
	}
}

func WithServiceKillSignal(killSignal string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.KillSignal = killSignal
	}
}

func WithServiceExtraLine(extraLine string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.ServiceExtraLine = utils.ParseStringToLines(extraLine)
	}
}

func WithInstallWantedBy(wantedBy string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.InstallWantedBy = wantedBy
	}
}

func WithTimerOnCalendar(onCalendar string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.OnCalendar = onCalendar
	}
}

func WithTimerOnActiveSec(onActiveSec int64) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.OnActiveSec = onActiveSec
	}
}

func WithTimerOnBootSec(onBootSec int64) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.OnBootSec = onBootSec
	}
}

func WithTimerOnStartupSec(onStartupSec int64) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.OnStartupSec = onStartupSec
	}
}

func WithTimerOnUnitActiveSec(onUnitActiveSec int64) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.OnUnitActiveSec = onUnitActiveSec
	}
}

func WithTimerOnUnitInactiveSec(s int64) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.OnUnitInactiveSec = s
	}
}

func WithTimerUnit(unit string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.TimerUnit = unit
	}
}

func WithServiceKillSignal9() ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.KillSignal = "SIGKILL"
	}
}

func WithTimerExtraLine(extraLine string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.TimerExtraLine = utils.ParseStringToLines(extraLine)
	}
}

func WithRaw(i string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.Raw = i
	}
}
