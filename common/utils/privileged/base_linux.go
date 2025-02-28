package privileged

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

func isPrivileged() bool {
	header := unix.CapUserHeader{
		Version: unix.LINUX_CAPABILITY_VERSION_3,
		Pid:     int32(os.Getpid()),
	}
	data := unix.CapUserData{}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := unix.Capget(&header, &data); err == nil {
		data.Inheritable = (1 << unix.CAP_NET_RAW)

		if err := unix.Capset(&header, &data); err == nil {
			return true
		}
	}
	return os.Geteuid() == 0
}

type Executor struct {
	AppName       string
	AppIcon       string
	DefaultPrompt string
}

func NewExecutor(appName string) *Executor {
	return &Executor{
		AppName:       appName,
		DefaultPrompt: "this operation requires administrator privileges",
	}
}

func (p *Executor) Execute(ctx context.Context, cmd string, opts ...ExecuteOption) ([]byte, error) {
	return nil, utils.Error("not implemented")
}
