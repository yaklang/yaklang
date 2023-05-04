package hidsevent

type HostUserInfo struct {
	UserName     string `json:"user_name"`
	Uid          int32  `json:"uid"`
	Gid          int32  `json:"gid"`
	FullUserName string `json:"full_user_name"`
	HomeDir      string `json:"home_dir"`
	BashFile     string `json:"bash_file"`
}

type HostUsers struct {
	Users []HostUserInfo `json:"users"`
}
