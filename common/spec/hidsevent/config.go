package hidsevent

import "encoding/json"

type HIDSConfigType string

var (
	HIDSConfig_All             HIDSConfigType = "all_config"
	HIDSConfig_WatchFileChange HIDSConfigType = "watch_file_change"
	HIDSConfig_Base            HIDSConfigType = "base"
)

type HIDSConfig struct {
	Type    HIDSConfigType  `json:"config_type"`
	Content json.RawMessage `json:"content"`
}

type HIDSConfigBase struct {
	UpdatedTicket                 int32  `json:"updated_ticket" `
	ProcessMonitorIntervalSeconds int32  `json:"process_monitor_interval_seconds" `
	NetportMonitorIntervalSeconds int32  `json:"netport_monitor_interval_seconds" `
	UserLoginOkFilePath           string `json:"usr_login_ok_file_path" `
	UserLoginFailFilePath         string `json:"usr_login_fail_file_path" `
	UserLoginFailFileMaxSize      int32  `json:"user_login_fail_file_max_size" `
	UserLoginFailCheckInterval    int64  `json:"user_login_fail_check_nterval" `
	UserLoginFailMaxTicket        int64  `json:"user_login_fail_max_ticket" `
	AptSoftwareLogFilePath        string `json:"apt_software_log_file_path" `
	YumSoftwareLogFilePath        string `json:"yum_software_log_file_path" `
	CrontabFilePath               string `json:"crontab_file_path" `
	SSHFilePath                   string `json:"ssh_file_path" `
}

type HIDSConfigSSHFile struct {
	SSHFile []string `json:"ssh_file"`
}

type HIDSConfigWatchFile struct {
	WatchFileList []string `json:"watch_file_list"`
}
