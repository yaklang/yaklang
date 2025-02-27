package privileged

var IsPrivileged = false

func init() {
	IsPrivileged = isPrivileged()
}

func GetIsPrivileged() bool {
	return isPrivileged()
}

type ExecuteOptions struct {
	Command     string
	Title       string
	Prompt      string
	Description string
}
