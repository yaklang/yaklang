package hidsevent

type CrontabNode struct {
	Cron     string `json:"cron"`
	Cmd      string `json:"cmd"`
	Software string `json:"software"`
}

type CrontabInfo struct {
	Info []CrontabNode `json:"info"`
}
