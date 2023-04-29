package hidsevent

type UserLoginInfo struct {
	UserName         string `json:"user_name"`
	EndpointType     string `json:"endpoint_type"`
	SourceEndpointIP string `json:"source_endpoint_ip"`
	LoginTimestamp   int32  `json:"login_timestamp"`
}

type UserLoginOK struct {
	LoginActions []UserLoginInfo `json:"login_actions"`
}

type UserLoginFail struct {
	LoginActions []UserLoginInfo `json:"login_actions"`
}

type UserLoginFailFileTooLarge struct {
	FilePath string `json:"file_path"`
	SizeM    int64  `json:"size_m"`
}

type UserLoginAttempt struct {
	TotalTicket int64            `json:"total_ticket"`
	Info        map[string]int64 `json:"info"`
}
