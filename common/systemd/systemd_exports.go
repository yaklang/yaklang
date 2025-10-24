package systemd

func Create(name string, opt ...ConfigOption) (string, string) {
	return NewSystemServiceConfig(name, opt...).ToServiceFile()
}

var Exports = map[string]interface{}{
	"Create": Create,

	// params
	"unit_description":          WithUnitDescription,
	"unit_documentation":        WithUnitDocumentation,
	"unit_after":                WithUnitAfter,
	"unit_before":               WithUnitBefore,
	"unit_requires":             WithUnitRequires,
	"unit_binds_to":             WithUnitBindsTo,
	"unit_wants":                WithUnitWants,
	"unit_extra_line":           WithUnitExtraLine,
	"service_type":              WithServiceType,
	"service_user":              WithServiceUser,
	"service_group":             WithServiceGroup,
	"service_exec_start":        WithServiceExecStart,
	"service_exec_start_pre":    WithServiceExecStartPre,
	"service_exec_start_post":   WithServiceExecStartPost,
	"service_exec_stop":         WithServiceExecStop,
	"service_exec_stop_post":    WithServiceExecStopPost,
	"service_restart":           WithServiceRestart,
	"service_restart_sec":       WithServiceRestartSec,
	"service_timeout_start_sec": WithServiceTimeoutStartSec,
	"service_environment":       WithServiceEnvironment,
	"service_environment_file":  WithServiceEnvironmentFile,
	"service_umask":             WithServiceUmask,
	"service_raw":               WithServiceExtraLine,
	"service_stdin":             WithServiceStandardInput,
	"service_stdout":            WithServiceStandardOutput,
	"service_stderr":            WithServiceStandardError,
	"service_kill_signal":       WithServiceKillSignal,
	"service_kill9":             WithServiceKillSignal9,
	"install_wanted_by":         WithInstallWantedBy,
	"timer_calendar":            WithTimerOnCalendar,
	"timer_active_sec":          WithTimerOnActiveSec,
	"timer_unit":                WithTimerUnit,
	"timer_boot_sec":            WithTimerOnBootSec,
	"timer_startup_sec":         WithTimerOnStartupSec,
	"timer_unit_active_sec":     WithTimerOnUnitActiveSec,
	"timer_unit_inactive_sec":   WithTimerOnUnitInactiveSec,
	"timer_raw":                 WithTimerExtraLine,
	"extra_raw":                 WithRaw,
	"service_kill_mode":         WithServiceKillMode,
}
