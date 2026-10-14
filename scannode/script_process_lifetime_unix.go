//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package scannode

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"

	"github.com/yaklang/yaklang/common/utils/subprocess"
	"golang.org/x/sys/unix"
)

const scriptParentLifetimeFDEnv = "LEGION_SCRIPT_PARENT_LIFETIME_FD"

func configureScriptProcessLifetime(cmd *exec.Cmd) (func(), error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	// Only the read end crosses exec. The close-on-exec writer stays in the
	// node, so EOF reflects process death rather than an individual Go thread.
	fd := 3 + len(cmd.ExtraFiles)
	cmd.ExtraFiles = append(cmd.ExtraFiles, reader)
	cmd.Env = replaceEnvironmentValue(cmd.Env, scriptParentLifetimeFDEnv, strconv.Itoa(fd))
	subprocess.ConfigureProcessGroup(cmd)
	if cmd.Cancel != nil {
		cmd.Cancel = func() error {
			subprocess.KillProcessGroup(cmd)
			return nil
		}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = reader.Close()
			_ = writer.Close()
		})
	}, nil
}

func watchScriptParentLifetime() (func(), error) {
	value := os.Getenv(scriptParentLifetimeFDEnv)
	if value == "" {
		// Standalone distyak invocations have no node lifetime contract.
		return func() {}, nil
	}
	fd, err := strconv.Atoi(value)
	if err != nil || fd < 3 {
		return nil, fmt.Errorf("invalid script parent lifetime descriptor")
	}
	reader := os.NewFile(uintptr(fd), "script-parent-lifetime")
	if reader == nil {
		return nil, fmt.Errorf("script parent lifetime descriptor is unavailable")
	}
	info, err := reader.Stat()
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		_ = reader.Close()
		return nil, fmt.Errorf("script parent lifetime descriptor is not a pipe")
	}
	if !scriptOwnsProcessGroup() {
		_ = reader.Close()
		return nil, fmt.Errorf("supervised script must own its process group")
	}
	// Do not leak the inherited read descriptor to commands launched by Yak.
	syscall.CloseOnExec(fd)
	done := make(chan struct{})
	var once sync.Once
	go func() {
		_, _ = io.Copy(io.Discard, reader)
		select {
		case <-done:
			return
		default:
		}
		// The parent cannot run cancellation handlers after SIGKILL. Stop the
		// execution even if a Yak/native function ignores context cancellation.
		// Never signal an inherited caller group if code changed our group.
		if scriptOwnsProcessGroup() {
			_ = syscall.Kill(-os.Getpid(), syscall.SIGKILL)
		}
		os.Exit(125)
	}()
	return func() {
		once.Do(func() {
			close(done)
			_ = reader.Close()
		})
	}, nil
}

func scriptOwnsProcessGroup() bool {
	group, err := unix.Getpgid(0)
	return err == nil && group == os.Getpid()
}
