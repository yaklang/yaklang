//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package stdio

import (
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

func prepareOutput(output io.WriteCloser) (io.WriteCloser, func(), error) {
	f, ok := output.(*os.File)
	if !ok {
		return output, func() { _ = output.Close() }, nil
	}
	raw, err := f.SyscallConn()
	if err != nil {
		return nil, nil, err
	}
	var flags, duplicate int
	var setupErr error
	err = raw.Control(func(fd uintptr) {
		flags, setupErr = unix.FcntlInt(fd, unix.F_GETFL, 0)
		if setupErr != nil {
			return
		}
		duplicate, setupErr = unix.FcntlInt(fd, unix.F_DUPFD_CLOEXEC, 0)
	})
	if err != nil {
		return nil, nil, err
	}
	if setupErr != nil {
		return nil, nil, setupErr
	}
	if err := unix.SetNonblock(duplicate, true); err != nil {
		_ = unix.Close(duplicate)
		return nil, nil, err
	}
	// NewFile registers a nonblocking pipe with Go's poller, so Close wakes Write.
	// The original descriptor remains available to the CLI's stdout restoration.
	writer := os.NewFile(uintptr(duplicate), "mcp-cancellable-output")
	cleanup := func() {
		_ = writer.Close()
		// dup shares file status flags. Restore the caller's blocking mode after
		// the private writer has finished; never call File.Fd(), which changes it.
		if flags&unix.O_NONBLOCK == 0 {
			_ = raw.Control(func(fd uintptr) { _ = unix.SetNonblock(int(fd), false) })
		}
	}
	info, err := writer.Stat()
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	if info.Mode()&(os.ModeNamedPipe|os.ModeSocket) != 0 {
		if err := writer.SetWriteDeadline(time.Time{}); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("MCP output does not support cancellable writes: %w", err)
		}
	}
	return writer, cleanup, nil
}
