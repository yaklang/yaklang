package hidsevent

import "encoding/json"

type HIDSEvent string
type HIDSTimestampType string

type HIDSSoftwareType string

var (
	// 进程监控
	HIDSEvent_Proccessings     HIDSEvent = "proccessings"
	HIDSEvent_ProccessingEvent HIDSEvent = "processing-event"
	HIDSEvent_ProccessingTouch HIDSEvent = "processing-touch"

	// 网络连接情况
	// 谨慎处理:
	//    1. 注意服务器某些情况下会有很多从外对内的连接, 如果针对这种情况过分处理会导致资源消耗过大
	//    2. 可以参考 telegraf 对 gopsutil 的使用
	//    3. 如果有必要, 抄代码出来, 一定要避免 (1) 中造成的问题: 可以找找有没有分页/或者预筛选的办法
	// 需要处理以下连接:
	// 1. 本地监听端口 (Status 为 LISTEN 的状态的端口, 上报) (重点)
	// 2. 对外连接的端口 (估计不会太多, 这个必须处理) (重点)
	// 3. 尽量避免 HA/NGINX 这种过多的连接对监控的影响
	//
	HIDSEvent_Connections     HIDSEvent = "connections"
	HIDSEvent_ConnectionTouch HIDSEvent = "connections-touch"
	HIDSEvent_ConnectionEvent HIDSEvent = "connection-event"

	// nginx / apache 监控
	HIDSEvent_NginxFound   HIDSEvent = "nginx-found"
	HIDSEvent_NginxMissed  HIDSEvent = "nginx-missed"
	HIDSEvent_ApacheFound  HIDSEvent = "apache-found"
	HIDSEvent_ApacheMissed HIDSEvent = "apache-missed"

	// ssh 审计分析
	// 1. 获取 SSH 精确版本信息
	// 2. 配置文件, 公钥私钥监控
	// 3. 配置文件关键选项:
	//    1. 是否允许密码登录
	//    2. 是否允许空密码
	//    3. 密钥登录
	HIDSEvent_SSHAudit HIDSEvent = "ssh-audit"

	// 文件改变
	// 暂时默认监控 /etc /bin /usr/bin ~/.ssh 下的文件内容
	HIDSEvent_FileChanged HIDSEvent = `file-changed`

	// 监测到 webshell
	HIDSEvent_WebShell HIDSEvent = "webshell"

	// 节点被扫描 (NIDS 的功能, 可以选择性)
	HIDSEvent_Scanned HIDSEvent = "scanned"

	// 关键配置文件
	HIDSEvent_ConfigFile HIDSEvent = "config-file"

	// 漏洞信息
	HIDSEvent_VulnInfo HIDSEvent = "vuln-info"

	// 危险文件样本
	HIDSEvent_DangerousFileSample HIDSEvent = "dangerous-file-sample"

	// 攻击行为
	HIDSEvent_Attack HIDSEvent = "attack"

	HIDSEvent_ReverseShell HIDSEvent = "reverse-shell"

	//请求配置
	HIDSEvent_RequestConfig HIDSEvent = "request_config"

	//上报主机用户信息
	HIDSEvent_ReportHostUser HIDSEvent = "report_host_user"
	//上报所有登陆成功用户信息
	HIDSEvent_ReportAllUsrLoginOK HIDSEvent = "all_user_login_ok"
	//上报所有登陆失败用户信息
	HIDSEvent_ReportAllUsrLoginFail HIDSEvent = "all_user_login_fail"
	//上报所有登陆失败用户信息文件过大
	HIDSEvent_ReportAllUsrLoginFailFileTooLarge HIDSEvent = "all_user_login_fail_file_too_large"
	//用户账号暴力破击
	HIDSEvent_UserLoginAttempt HIDSEvent = "user_login_attempt"
	//软件信息上报
	HIDSEvent_ReportSoftwareVersion HIDSEvent = "report_software_version"
	//开启启动软件信息
	HIDSEvent_BootSoftware HIDSEvent = "boot_software"
	//定时任务
	HIDSEvent_Crontab HIDSEvent = "crontab"
)

var (
	HIDSEvent_Notify_Config HIDSEvent = "notify_config"
)

var (
	HIDSTimestampType_Last_Check_Login_Fail HIDSTimestampType = "last_check_login_fail"
)

var (
	HIDSSoftwareType_APT HIDSSoftwareType = "apt"
	HIDSSoftwareType_YUM HIDSSoftwareType = "yum"
)

type HIDSMessage struct {
	Type    HIDSEvent       `json:"event"`
	Content json.RawMessage `json:"content"`
}
