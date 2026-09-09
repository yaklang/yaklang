//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package stdio

import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// prepareInput must run after prepareOutput: terminal stdin and stdout may share
// file status flags, so preparing output can also make stdin nonblocking. An
// existing os.Stdin created in blocking mode cannot wait for EAGAIN via Go's
// poller. A new wrapper observes the updated flags and enables that waiting.
func prepareInput(input io.ReadCloser) (io.Reader, func(), error) {
	f, ok := input.(*os.File)
	if !ok {
		return input, func() {}, nil
	}
	raw, err := f.SyscallConn()
	if err != nil {
		return nil, nil, err
	}
	duplicate := -1
	var setupErr error
	err = raw.Control(func(fd uintptr) {
		var flags int
		flags, setupErr = unix.FcntlInt(fd, unix.F_GETFL, 0)
		if setupErr == nil && flags&unix.O_NONBLOCK != 0 {
			duplicate, setupErr = unix.FcntlInt(fd, unix.F_DUPFD_CLOEXEC, 0)
		}
	})
	if err != nil {
		return nil, nil, err
	}
	if setupErr != nil {
		return nil, nil, setupErr
	}
	if duplicate == -1 {
		return input, func() {}, nil
	}
	reader := os.NewFile(uintptr(duplicate), "mcp-cancellable-input")
	cleanup := func() { _ = reader.Close() }
	return reader, cleanup, nil
}
