package privileged

var IsPrivileged = false

func init() {
	IsPrivileged = isPrivileged()
}

func GetIsPrivileged() bool {
	return isPrivileged()
}
