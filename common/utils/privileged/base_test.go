package privileged

import (
	"syscall"
	"testing"
)

func TestIsPrivileged(t *testing.T) {
	println(GetIsPrivileged())
}

func TestGetIsPrivileged(t *testing.T) {
	err := syscall.Setuid(0)
	if err != nil {
		panic(err)
	}
}
