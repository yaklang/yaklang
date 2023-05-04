package umask

import "syscall"

func Umask(i int) int {
	return syscall.Umask(i)
}
