package privileged

import "os"

func isPrivileged() bool {
	return os.Geteuid() == 0
}
