package subprocess

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// StdioConfig configures the child process's three standard streams.
//
//   Stdin  – io.Reader used as the child's stdin source.
//             To write to the child, supply the read end of an io.Pipe
//             and keep the write end for yourself.
//
//   Stdout – io.Writer the child's stdout is drained to.
//             May be nil (output discarded).
//             If ManagedProcess.Stdout() is also called, a copy of
//             every byte is additionally delivered to that reader.
//
//   Stderr – same semantics as Stdout, for the child's stderr.
type StdioConfig struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// LaunchConfig configures process launch behavior.
type LaunchConfig struct {
	// Cmd is the exec.Cmd to launch. Caller sets Path/Args/Dir.
	// Env, SysProcAttr, Stdin/Stdout/Stderr are set by Launch
	// based on EnvExclude/EnvExtra and Stdio.
	Cmd *exec.Cmd

	// EnvExclude strips these env vars from the child (case-insensitive).
	EnvExclude []string

	// EnvExtra are appended to the child environment.
	EnvExtra []string

	// Stdio configures the child's standard streams.
	Stdio StdioConfig

	// StartupTimeout is the readiness deadline. Zero = 60s.
	StartupTimeout time.Duration

	// ShutdownTimeout is the force-kill grace period. Zero = 5s.
	ShutdownTimeout time.Duration

	// Ready is invoked after cmd.Start. Return nil when ready.
	// If nil, the process is ready immediately after Start.
	Ready func(ctx context.Context, p *ManagedProcess) error

	// GracefulShutdown is called before force-killing.
	// It receives the ManagedProcess so it can write to a pipe
	// it created for Stdin, or close application-level connections.
	// If nil, the process is killed directly.
	GracefulShutdown func(p *ManagedProcess) error
}

// ManagedProcess wraps an exec.Cmd with lifecycle management and
// exposes stdout/stderr tap readers to the caller.
type ManagedProcess struct {
	cmd      *exec.Cmd
	done     chan struct{}
	waitErr  error
	waitOnce sync.Once

	// whether stdout/stderr were piped (vs. direct *os.File or nil)
	stdoutPiped bool
	stderrPiped bool

	// drain goroutine completion
	drainWg sync.WaitGroup

	// tap pipes – created lazily by Stdout()/Stderr()
	stdoutTapMu     sync.Mutex
	stdoutTapWriter io.WriteCloser
	stdoutTapReader io.ReadCloser
	stderrTapMu     sync.Mutex
	stderrTapWriter io.WriteCloser
	stderrTapReader io.ReadCloser

	shutdownTimeout time.Duration
	onShutdown      func(*ManagedProcess) error
	closeOnce       sync.Once
}

// Launch starts the process, waits for readiness, and returns
// a ManagedProcess. On failure the process is killed and reaped.
func Launch(ctx context.Context, cfg *LaunchConfig) (_ *ManagedProcess, retErr error) {
	if cfg.Cmd == nil {
		return nil, fmt.Errorf("subprocess: Cmd is nil")
	}
	cmd := cfg.Cmd

	// --- Environment ---
	parent := cmd.Env
	if parent == nil {
		parent = os.Environ()
	}
	cmd.Env = BuildChildEnvironment(parent, cfg.EnvExclude, cfg.EnvExtra)

	// --- Platform process group ---
	ConfigureProcessGroup(cmd)

	// --- Stdio: stdin ---
	cmd.Stdin = cfg.Stdio.Stdin

	// --- Stdio: stdout ---
	stdoutChildFD, stdoutDrainFD, stdoutNeedsPipe, err := prepareStdPipe(cfg.Stdio.Stdout)
	if err != nil {
		return nil, fmt.Errorf("subprocess: prepare stdout pipe: %w", err)
	}
	if stdoutNeedsPipe {
		cmd.Stdout = stdoutChildFD
	} else {
		cmd.Stdout = cfg.Stdio.Stdout
	}

	// --- Stdio: stderr ---
	stderrChildFD, stderrDrainFD, stderrNeedsPipe, err := prepareStdPipe(cfg.Stdio.Stderr)
	if err != nil {
		return nil, fmt.Errorf("subprocess: prepare stderr pipe: %w", err)
	}
	if stderrNeedsPipe {
		cmd.Stderr = stderrChildFD
	} else {
		cmd.Stderr = cfg.Stdio.Stderr
	}

	p := &ManagedProcess{
		cmd:             cmd,
		done:            make(chan struct{}),
		stdoutPiped:     stdoutNeedsPipe,
		stderrPiped:     stderrNeedsPipe,
		shutdownTimeout: orDefault(cfg.ShutdownTimeout, 5*time.Second),
		onShutdown:      cfg.GracefulShutdown,
	}

	// --- Start ---
	if err := cmd.Start(); err != nil {
		if stdoutChildFD != nil {
			_ = stdoutChildFD.Close()
		}
		if stderrChildFD != nil {
			_ = stderrChildFD.Close()
		}
		if stdoutDrainFD != nil {
			_ = stdoutDrainFD.Close()
		}
		if stderrDrainFD != nil {
			_ = stderrDrainFD.Close()
		}
		return nil, fmt.Errorf("subprocess: start: %w", err)
	}

	// Close child-side write ends in the parent process.
	if stdoutChildFD != nil {
		_ = stdoutChildFD.Close()
	}
	if stderrChildFD != nil {
		_ = stderrChildFD.Close()
	}

	go func() {
		p.waitErr = cmd.Wait()
		close(p.done)
	}()

	// Start drain goroutines (they copy pipe → configured writer + tap).
	if stdoutNeedsPipe {
		p.drainWg.Add(1)
		go func() {
			defer p.drainWg.Done()
			p.drain(stdoutDrainFD, cfg.Stdio.Stdout, &p.stdoutTapMu, &p.stdoutTapWriter)
		}()
	}
	if stderrNeedsPipe {
		p.drainWg.Add(1)
		go func() {
			defer p.drainWg.Done()
			p.drain(stderrDrainFD, cfg.Stdio.Stderr, &p.stderrTapMu, &p.stderrTapWriter)
		}()
	}

	defer func() {
		if retErr != nil {
			p.Kill()
		}
	}()

	// --- Readiness ---
	if cfg.Ready != nil {
		startupTimeout := orDefault(cfg.StartupTimeout, 60*time.Second)
		readyCtx, cancel := context.WithTimeout(ctx, startupTimeout)
		defer cancel()

		readyDone := make(chan error, 1)
		go func() {
			readyDone <- cfg.Ready(readyCtx, p)
		}()

		select {
		case <-p.done:
			cancel()
			if p.waitErr != nil {
				return nil, fmt.Errorf("subprocess: process exited during startup: %w", p.waitErr)
			}
			return nil, fmt.Errorf("subprocess: process exited during startup")
		case err := <-readyDone:
			if err != nil {
				return nil, fmt.Errorf("subprocess: ready: %w", err)
			}
		case <-readyCtx.Done():
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("subprocess: startup timeout: %w", readyCtx.Err())
		}
	}

	return p, nil
}

// prepareStdPipe decides whether a pipe is needed for a stdout/stderr stream.
//
// Returns:
//   childFD   – the *os.File to assign to cmd.Stdout/Stderr (write end)
//   drainFD   – the *os.File to read from in the drain goroutine (read end)
//   needsPipe – true if a pipe was created
//
// A pipe is NOT created when the configured writer is an *os.File,
// because exec.Cmd connects the fd directly to the child (no goroutine).
// In that case Stdout()/Stderr() will return nil.
//
// Otherwise a pipe is always created, so Stdout()/Stderr() can be
// called later to get a tap reader.
func prepareStdPipe(w io.Writer) (childFD, drainFD *os.File, needsPipe bool, err error) {
	if _, ok := w.(*os.File); ok {
		return nil, nil, false, nil
	}
	r, w2, err := os.Pipe()
	if err != nil {
		return nil, nil, false, err
	}
	return w2, r, true, nil
}

// drain copies from src to the configured writer and, if a tap has been
// created, to the tap pipe writer. When src reaches EOF or error, the
// tap writer is closed so the tap reader gets EOF.
func (p *ManagedProcess) drain(
	src *os.File,
	configuredWriter io.Writer,
	tapMu *sync.Mutex,
	tapWriter *io.WriteCloser,
) {
	dest := configuredWriter
	if dest == nil {
		dest = io.Discard
	}

	buf := make([]byte, 4096)
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			data := buf[:n]
			_, _ = dest.Write(data)
			tapMu.Lock()
			if *tapWriter != nil {
				_, _ = (*tapWriter).Write(data)
			}
			tapMu.Unlock()
		}
		if readErr != nil {
			break
		}
	}
	_ = src.Close()
	tapMu.Lock()
	if *tapWriter != nil {
		_ = (*tapWriter).Close()
		*tapWriter = nil
	}
	tapMu.Unlock()
}

// Stdout returns a reader that yields a copy of the child's stdout.
// The data is also drained to the StdioConfig.Stdout writer.
// Returns nil if stdout was configured as *os.File (fd inheritance)
// or not piped.
//
// Must be called after Launch returns. Data produced before this call
// is not replayed; call it promptly after Launch.
func (p *ManagedProcess) Stdout() io.Reader {
	if !p.stdoutPiped {
		return nil
	}
	p.stdoutTapMu.Lock()
	defer p.stdoutTapMu.Unlock()
	if p.stdoutTapReader != nil {
		return p.stdoutTapReader
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil
	}
	p.stdoutTapWriter = w
	p.stdoutTapReader = r
	return r
}

// Stderr returns a reader that yields a copy of the child's stderr.
// Same semantics as Stdout().
func (p *ManagedProcess) Stderr() io.Reader {
	if !p.stderrPiped {
		return nil
	}
	p.stderrTapMu.Lock()
	defer p.stderrTapMu.Unlock()
	if p.stderrTapReader != nil {
		return p.stderrTapReader
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil
	}
	p.stderrTapWriter = w
	p.stderrTapReader = r
	return r
}

// Done returns a channel closed when the process exits.
func (p *ManagedProcess) Done() <-chan struct{} { return p.done }

// WaitError returns the error from cmd.Wait, or nil if still running.
func (p *ManagedProcess) WaitError() error {
	select {
	case <-p.done:
		return p.waitErr
	default:
		return nil
	}
}

// PID returns the process ID, or 0 if not started.
func (p *ManagedProcess) PID() int {
	if p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

// Cmd returns the underlying exec.Cmd for advanced use cases.
func (p *ManagedProcess) Cmd() *exec.Cmd { return p.cmd }

// Close gracefully shuts down: calls GracefulShutdown, waits for
// ShutdownTimeout, then force-kills the process group.
// After the process exits, drain goroutines are joined to ensure all
// buffered output has been flushed to the configured writers.
// Idempotent.
func (p *ManagedProcess) Close() {
	p.closeOnce.Do(func() {
		if p.onShutdown != nil {
			_ = p.onShutdown(p)
		}
		timer := time.NewTimer(p.shutdownTimeout)
		defer timer.Stop()
		select {
		case <-p.done:
			p.drainWg.Wait()
			return
		case <-timer.C:
		}
		p.Kill()
	})
}

// Kill immediately kills the process group and waits for exit.
// Drain goroutines are joined to ensure pipe cleanup.
func (p *ManagedProcess) Kill() {
	KillProcessGroup(p.cmd)
	p.waitOnce.Do(func() { <-p.done })
	p.drainWg.Wait()
}

func orDefault(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}
