package systemd

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type ConfigOption func(*SystemdServiceConfig)

// unit_description 设置 [Unit] 段的 Description，用于描述该服务用途
//
// 参数:
//   - description: 服务描述文本
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.unit_description("My Application"))
// assert str.Contains(serviceFile, "Description=My Application")
// ```
func WithUnitDescription(description string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.Description = description
	}
}

// unit_documentation 设置 [Unit] 段的 Documentation，指向服务文档链接
//
// 参数:
//   - documentation: 文档地址（如 man 手册或 URL）
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.unit_documentation("https://example.com/docs"))
// assert str.Contains(serviceFile, "Documentation=https://example.com/docs")
// ```
func WithUnitDocumentation(documentation string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.Documentation = documentation
	}
}

// unit_after 设置 [Unit] 段的 After，声明本服务应在指定单元之后启动
//
// 参数:
//   - after: 依赖的单元名（如 network.target）
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.unit_after("network.target"))
// assert str.Contains(serviceFile, "After=network.target")
// ```
func WithUnitAfter(after string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.After = after
	}
}

// unit_before 设置 [Unit] 段的 Before，声明本服务应在指定单元之前启动
//
// 参数:
//   - before: 在其之前启动的单元名
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.unit_before("nginx.service"))
// assert str.Contains(serviceFile, "Before=nginx.service")
// ```
func WithUnitBefore(before string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.Before = before
	}
}

// unit_requires 设置 [Unit] 段的 Requires，声明强依赖单元（依赖失败则本服务也失败）
//
// 参数:
//   - requires: 强依赖的单元名
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.unit_requires("postgresql.service"))
// assert str.Contains(serviceFile, "Requires=postgresql.service")
// ```
func WithUnitRequires(requires string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.Requires = requires
	}
}

// unit_binds_to 设置 [Unit] 段的 BindsTo，声明强绑定单元（绑定单元停止则本服务也停止）
//
// 参数:
//   - bindsTo: 绑定的单元名
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.unit_binds_to("docker.service"))
// assert str.Contains(serviceFile, "BindsTo=docker.service")
// ```
func WithUnitBindsTo(bindsTo string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.BindsTo = bindsTo
	}
}

// unit_wants 设置 [Unit] 段的 Wants，声明弱依赖单元（依赖失败不影响本服务启动）
//
// 参数:
//   - wants: 弱依赖的单元名
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.unit_wants("redis.service"))
// assert str.Contains(serviceFile, "Wants=redis.service")
// ```
func WithUnitWants(wants string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.Wants = wants
	}
}

// unit_extra_line 向 [Unit] 段追加自定义原始行（多行可用换行分隔）
//
// 参数:
//   - extraLine: 追加到 [Unit] 段的原始内容
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.unit_extra_line("ConditionPathExists=/etc/myapp.conf"))
// assert str.Contains(serviceFile, "ConditionPathExists=/etc/myapp.conf")
// ```
func WithUnitExtraLine(extraLine string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.UnitExtraLine = utils.ParseStringToLines(extraLine)
	}
}

// service_type 设置 [Service] 段的 Type，仅接受 simple/forking/oneshot/dbus/notify/idle
//
// 参数:
//   - serviceType: 服务类型，非法值会回退为默认 simple
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_type("forking"))
// assert str.Contains(serviceFile, "Type=forking")
// ```
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

// service_user 设置 [Service] 段的 User，指定运行服务的用户
//
// 参数:
//   - user: 运行用户名
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_user("www-data"))
// assert str.Contains(serviceFile, "User=www-data")
// ```
func WithServiceUser(user string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.User = user
	}
}

// service_group 设置 [Service] 段的 Group，指定运行服务的用户组
//
// 参数:
//   - group: 运行用户组名
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_group("www-data"))
// assert str.Contains(serviceFile, "Group=www-data")
// ```
func WithServiceGroup(group string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.Group = group
	}
}

// service_exec_start 设置 [Service] 段的 ExecStart，即服务启动命令
//
// 参数:
//   - execStart: 启动命令（建议使用绝对路径）
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_exec_start("/usr/bin/myapp --serve"))
// assert str.Contains(serviceFile, "ExecStart=/usr/bin/myapp --serve")
// ```
func WithServiceExecStart(execStart string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.ExecStart = execStart
	}
}

// service_exec_start_pre 设置 [Service] 段的 ExecStartPre，启动前执行的命令
//
// 参数:
//   - execStartPre: 启动前命令
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_exec_start_pre("/usr/bin/myapp --check"))
// assert str.Contains(serviceFile, "ExecStartPre=/usr/bin/myapp --check")
// ```
func WithServiceExecStartPre(execStartPre string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.ExecStartPre = execStartPre
	}
}

// service_exec_start_post 设置 [Service] 段的 ExecStartPost，启动后执行的命令
//
// 参数:
//   - execStartPost: 启动后命令
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_exec_start_post("/usr/bin/notify-ready"))
// assert str.Contains(serviceFile, "ExecStartPost=/usr/bin/notify-ready")
// ```
func WithServiceExecStartPost(execStartPost string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.ExecStartPost = execStartPost
	}
}

// service_exec_stop 设置 [Service] 段的 ExecStop，即服务停止命令
//
// 参数:
//   - execStop: 停止命令
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_exec_stop("/usr/bin/myapp --shutdown"))
// assert str.Contains(serviceFile, "ExecStop=/usr/bin/myapp --shutdown")
// ```
func WithServiceExecStop(execStop string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.ExecStop = execStop
	}
}

// service_exec_stop_post 设置 [Service] 段的 ExecStopPost，停止后执行的命令
//
// 参数:
//   - execStopPost: 停止后命令
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_exec_stop_post("/usr/bin/cleanup"))
// assert str.Contains(serviceFile, "ExecStopPost=/usr/bin/cleanup")
// ```
func WithServiceExecStopPost(execStopPost string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.ExecStopPost = execStopPost
	}
}

// service_restart 设置 [Service] 段的 Restart 策略
// 仅接受 no/on-success/on-failure/on-abnormal/on-watchdog/on-abort/always
//
// 参数:
//   - restart: 重启策略，非法值会回退为默认 no
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_restart("always"))
// assert str.Contains(serviceFile, "Restart=always")
// ```
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

// service_restart_sec 设置 [Service] 段的 RestartSec，重启前等待的秒数
//
// 参数:
//   - restartSec: 重启等待时间，单位为秒
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_restart_sec(5))
// assert str.Contains(serviceFile, "RestartSec=")
// ```
func WithServiceRestartSec(restartSec float64) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.RestartSec = restartSec
	}
}

// service_timeout_start_sec 设置 [Service] 段的 TimeoutStartSec，启动超时秒数
//
// 参数:
//   - timeoutStartSec: 启动超时时间，单位为秒
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_timeout_start_sec(30))
// assert str.Contains(serviceFile, "TimeoutStartSec=")
// ```
func WithServiceTimeoutStartSec(timeoutStartSec float64) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.TimeoutStartSec = timeoutStartSec
	}
}

// service_environment 设置 [Service] 段的 Environment，注入环境变量
//
// 参数:
//   - environment: 环境变量声明（如 "KEY=VALUE"）
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_environment("ENV=production"))
// assert str.Contains(serviceFile, "Environment=ENV=production")
// ```
func WithServiceEnvironment(environment string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.Environment = environment
	}
}

// service_environment_file 设置 [Service] 段的 EnvironmentFile，从文件加载环境变量
//
// 参数:
//   - environmentFile: 环境变量文件路径
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_environment_file("/etc/myapp.env"))
// assert str.Contains(serviceFile, "EnvironmentFile=/etc/myapp.env")
// ```
func WithServiceEnvironmentFile(environmentFile string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.EnvironmentFile = environmentFile
	}
}

// service_umask 设置 [Service] 段的 UMask，设置进程文件创建掩码
//
// 参数:
//   - umask: 文件掩码（如 "0022"）
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_umask("0027"))
// assert str.Contains(serviceFile, "Umask=0027")
// ```
func WithServiceUmask(umask string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.Umask = umask
	}
}

// service_stdin 设置 [Service] 段的 StandardInput
// 仅接受 null/tty/tty-force/tty-fail/socket/fd/fd-force/fd-fail/file/file-force/file-fail
//
// 参数:
//   - standardInput: 标准输入类型，非法值会回退为默认 null
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_stdin("tty"))
// assert str.Contains(serviceFile, "StandardInput=tty")
// ```
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

// service_stdout 设置 [Service] 段的 StandardOutput
// 仅接受 inherit/null/tty/journal/syslog/kmsg/journal+console/syslog+console/kmsg+console/socket/fd/file
//
// 参数:
//   - standardOutput: 标准输出类型，非法值会回退为默认 inherit
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_stdout("journal"))
// assert str.Contains(serviceFile, "StandardOutput=journal")
// ```
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

// service_stderr 设置 [Service] 段的 StandardError
// 仅接受 inherit/null/tty/journal/syslog/kmsg/journal+console/syslog+console/kmsg+console/socket/fd/file
//
// 参数:
//   - standardError: 标准错误类型，非法值会回退为默认 inherit
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_stderr("journal"))
// assert str.Contains(serviceFile, "StandardError=journal")
// ```
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

// service_kill_mode 设置 [Service] 段的 KillMode
// 仅接受 control-group/process/mixed/none
//
// 参数:
//   - killMode: 进程终止模式，非法值会回退为默认 control-group
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_kill_mode("mixed"))
// assert str.Contains(serviceFile, "KillMode=mixed")
// ```
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

// service_kill_signal 设置 [Service] 段的 KillSignal，指定停止服务时发送的信号
//
// 参数:
//   - killSignal: 信号名（如 SIGTERM、SIGINT）
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_kill_signal("SIGTERM"))
// assert str.Contains(serviceFile, "KillSignal=SIGTERM")
// ```
func WithServiceKillSignal(killSignal string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.KillSignal = killSignal
	}
}

// service_raw 向 [Service] 段追加自定义原始行（多行可用换行分隔）
//
// 参数:
//   - extraLine: 追加到 [Service] 段的原始内容
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_raw("LimitNOFILE=65536"))
// assert str.Contains(serviceFile, "LimitNOFILE=65536")
// ```
func WithServiceExtraLine(extraLine string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.ServiceExtraLine = utils.ParseStringToLines(extraLine)
	}
}

// install_wanted_by 设置 [Install] 段的 WantedBy，决定 enable 时挂载到哪个 target
//
// 参数:
//   - wantedBy: 目标 target（如 multi-user.target）
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.install_wanted_by("multi-user.target"))
// assert str.Contains(serviceFile, "WantedBy=multi-user.target")
// ```
func WithInstallWantedBy(wantedBy string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.InstallWantedBy = wantedBy
	}
}

// timer_calendar 设置 [Timer] 段的 OnCalendar，使用日历表达式定时触发
//
// 参数:
//   - onCalendar: 日历表达式（如 "*-*-* 02:00:00"）
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项；配置该项后 systemd.Create 会返回非空 timer 内容
//
// Example:
// ```
// _, timerFile = systemd.Create("myapp", systemd.timer_unit("myapp.service"), systemd.timer_calendar("*-*-* 02:00:00"))
// assert str.Contains(timerFile, "OnCalendar=*-*-* 02:00:00")
// ```
func WithTimerOnCalendar(onCalendar string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.OnCalendar = onCalendar
	}
}

// timer_active_sec 设置 [Timer] 段的 OnActiveSec，相对定时器激活时刻触发
//
// 参数:
//   - onActiveSec: 相对激活时刻的秒数
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, timerFile = systemd.Create("myapp", systemd.timer_unit("myapp.service"), systemd.timer_active_sec(60))
// assert str.Contains(timerFile, "OnActiveSec=")
// ```
func WithTimerOnActiveSec(onActiveSec int64) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.OnActiveSec = onActiveSec
	}
}

// timer_boot_sec 设置 [Timer] 段的 OnBootSec，相对系统启动时刻触发
//
// 参数:
//   - onBootSec: 相对系统启动的秒数
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, timerFile = systemd.Create("myapp", systemd.timer_unit("myapp.service"), systemd.timer_boot_sec(120))
// assert str.Contains(timerFile, "OnBootSec=")
// ```
func WithTimerOnBootSec(onBootSec int64) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.OnBootSec = onBootSec
	}
}

// timer_startup_sec 设置 [Timer] 段的 OnStartupSec，相对 systemd 启动时刻触发
//
// 参数:
//   - onStartupSec: 相对 systemd 启动的秒数
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, timerFile = systemd.Create("myapp", systemd.timer_unit("myapp.service"), systemd.timer_startup_sec(90))
// assert str.Contains(timerFile, "OnStartupSec=")
// ```
func WithTimerOnStartupSec(onStartupSec int64) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.OnStartupSec = onStartupSec
	}
}

// timer_unit_active_sec 设置 [Timer] 段的 OnUnitActiveSec，相对单元上次激活时刻触发
//
// 参数:
//   - onUnitActiveSec: 相对单元上次激活的秒数
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, timerFile = systemd.Create("myapp", systemd.timer_unit("myapp.service"), systemd.timer_unit_active_sec(3600))
// assert str.Contains(timerFile, "OnUnitActiveSec=")
// ```
func WithTimerOnUnitActiveSec(onUnitActiveSec int64) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.OnUnitActiveSec = onUnitActiveSec
	}
}

// timer_unit_inactive_sec 设置 [Timer] 段的 OnUnitInactiveSec，相对单元上次停用时刻触发
//
// 参数:
//   - s: 相对单元上次停用的秒数
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, timerFile = systemd.Create("myapp", systemd.timer_unit("myapp.service"), systemd.timer_unit_inactive_sec(1800))
// assert str.Contains(timerFile, "OnUnitInactiveSec=")
// ```
func WithTimerOnUnitInactiveSec(s int64) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.OnUnitInactiveSec = s
	}
}

// timer_unit 设置 [Timer] 段的 Unit，指定定时器触发时激活的单元
//
// 参数:
//   - unit: 被触发的单元名
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, timerFile = systemd.Create("myapp", systemd.timer_unit("backup.service"), systemd.timer_calendar("daily"))
// assert str.Contains(timerFile, "Unit=backup.service")
// ```
func WithTimerUnit(unit string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.TimerUnit = unit
	}
}

// service_kill9 设置 [Service] 段的 KillSignal 为 SIGKILL（强制杀死）
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.service_kill9())
// assert str.Contains(serviceFile, "KillSignal=SIGKILL")
// ```
func WithServiceKillSignal9() ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.KillSignal = "SIGKILL"
	}
}

// timer_raw 向 [Timer] 段追加自定义原始行（多行可用换行分隔）
//
// 参数:
//   - extraLine: 追加到 [Timer] 段的原始内容
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, timerFile = systemd.Create("myapp", systemd.timer_unit("myapp.service"), systemd.timer_raw("Persistent=true"))
// assert str.Contains(timerFile, "Persistent=true")
// ```
func WithTimerExtraLine(extraLine string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.TimerExtraLine = utils.ParseStringToLines(extraLine)
	}
}

// extra_raw 设置整体追加到单元文件末尾的原始内容
//
// 参数:
//   - i: 追加的原始内容
//
// 返回值:
//   - 可传入 systemd.Create 的配置选项
//
// Example:
// ```
// _, serviceFile = systemd.Create("myapp", systemd.extra_raw("# generated by yaklang"))
// assert str.Contains(serviceFile, "# generated by yaklang")
// ```
func WithRaw(i string) ConfigOption {
	return func(c *SystemdServiceConfig) {
		c.Raw = i
	}
}
