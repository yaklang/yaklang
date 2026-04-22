//go:build hids && linux

package scannode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	gopsprocess "github.com/shirou/gopsutil/v4/process"
)

const (
	responseActionTerminateWait = 1500 * time.Millisecond
	responseActionKillWait      = 2 * time.Second
	responseActionPollInterval  = 100 * time.Millisecond
)

func executeHIDSResponseAction(
	ctx context.Context,
	actionType string,
	process hidsResponseActionProcess,
) (hidsResponseActionExecutionResult, error) {
	switch strings.TrimSpace(actionType) {
	case hidsResponseActionProcessTerminate:
		return executeProcessTerminate(ctx, process)
	default:
		return hidsResponseActionExecutionResult{
			ObservedAt: time.Now().UTC(),
			Process:    process,
		}, ErrHIDSResponseActionUnsupported
	}
}

func executeProcessTerminate(
	ctx context.Context,
	target hidsResponseActionProcess,
) (hidsResponseActionExecutionResult, error) {
	now := time.Now().UTC()
	if target.PID <= 1 || target.PID == os.Getpid() {
		return hidsResponseActionExecutionResult{
			ObservedAt: now,
			Process:    target,
			DetailJSON: mustMarshalHIDSResponseActionJSON(map[string]any{
				"pid":     target.PID,
				"blocked": true,
				"reason":  "protected_process",
			}),
		}, ErrHIDSResponseActionProtectedProcess
	}

	currentBootID := readCurrentBootID()
	if currentBootID == "" || currentBootID != target.BootID {
		return hidsResponseActionExecutionResult{
			ObservedAt: now,
			Process:    target,
			DetailJSON: mustMarshalHIDSResponseActionJSON(map[string]any{
				"requested_boot_id": target.BootID,
				"current_boot_id":   currentBootID,
			}),
		}, ErrHIDSResponseActionIdentityMismatch
	}

	proc, err := gopsprocess.NewProcessWithContext(ctx, int32(target.PID))
	if err != nil {
		return hidsResponseActionExecutionResult{
			ObservedAt: now,
			Process:    target,
		}, ErrHIDSResponseActionProcessNotFound
	}
	current, err := loadCurrentResponseActionProcess(ctx, proc, target.BootID)
	if err != nil {
		return hidsResponseActionExecutionResult{
			ObservedAt: now,
			Process:    target,
		}, err
	}
	if current.StartTimeUnixMillis != target.StartTimeUnixMillis {
		return hidsResponseActionExecutionResult{
			ObservedAt: now,
			Process:    current,
			DetailJSON: mustMarshalHIDSResponseActionJSON(map[string]any{
				"requested_start_time_unix_ms": target.StartTimeUnixMillis,
				"current_start_time_unix_ms":   current.StartTimeUnixMillis,
			}),
		}, ErrHIDSResponseActionIdentityMismatch
	}

	if err := proc.TerminateWithContext(ctx); err != nil {
		return hidsResponseActionExecutionResult{
			ObservedAt: time.Now().UTC(),
			Process:    current,
			DetailJSON: mustMarshalHIDSResponseActionJSON(map[string]any{
				"signal": "SIGTERM",
				"error":  err.Error(),
			}),
		}, fmt.Errorf("%w: %v", ErrHIDSResponseActionSignalFailed, err)
	}
	terminated, waitErr := waitForProcessExit(ctx, proc, responseActionTerminateWait)
	if waitErr != nil {
		return hidsResponseActionExecutionResult{
			ObservedAt: time.Now().UTC(),
			Process:    current,
		}, waitErr
	}
	if terminated {
		return hidsResponseActionExecutionResult{
			ObservedAt: time.Now().UTC(),
			Process:    current,
			DetailJSON: mustMarshalHIDSResponseActionJSON(map[string]any{
				"strategy":       []string{"SIGTERM"},
				"confirmed_exit": true,
			}),
		}, nil
	}

	if err := proc.SendSignalWithContext(ctx, syscall.SIGKILL); err != nil {
		return hidsResponseActionExecutionResult{
			ObservedAt: time.Now().UTC(),
			Process:    current,
			DetailJSON: mustMarshalHIDSResponseActionJSON(map[string]any{
				"strategy": []string{"SIGTERM", "SIGKILL"},
				"error":    err.Error(),
			}),
		}, fmt.Errorf("%w: %v", ErrHIDSResponseActionSignalFailed, err)
	}
	killed, waitErr := waitForProcessExit(ctx, proc, responseActionKillWait)
	if waitErr != nil {
		return hidsResponseActionExecutionResult{
			ObservedAt: time.Now().UTC(),
			Process:    current,
		}, waitErr
	}
	if !killed {
		return hidsResponseActionExecutionResult{
			ObservedAt: time.Now().UTC(),
			Process:    current,
			DetailJSON: mustMarshalHIDSResponseActionJSON(map[string]any{
				"strategy":       []string{"SIGTERM", "SIGKILL"},
				"confirmed_exit": false,
			}),
		}, ErrHIDSResponseActionStillRunning
	}
	return hidsResponseActionExecutionResult{
		ObservedAt: time.Now().UTC(),
		Process:    current,
		DetailJSON: mustMarshalHIDSResponseActionJSON(map[string]any{
			"strategy":       []string{"SIGTERM", "SIGKILL"},
			"confirmed_exit": true,
		}),
	}, nil
}

func loadCurrentResponseActionProcess(
	ctx context.Context,
	process *gopsprocess.Process,
	bootID string,
) (hidsResponseActionProcess, error) {
	if process == nil {
		return hidsResponseActionProcess{}, ErrHIDSResponseActionProcessNotFound
	}
	startTimeUnixMillis, err := process.CreateTimeWithContext(ctx)
	if err != nil || startTimeUnixMillis <= 0 {
		return hidsResponseActionProcess{}, ErrHIDSResponseActionProcessNotFound
	}
	name, _ := process.NameWithContext(ctx)
	image, _ := process.ExeWithContext(ctx)
	command, _ := process.CmdlineWithContext(ctx)
	username, _ := process.UsernameWithContext(ctx)
	return hidsResponseActionProcess{
		PID:                 int(process.Pid),
		BootID:              strings.TrimSpace(bootID),
		StartTimeUnixMillis: startTimeUnixMillis,
		ProcessName:         strings.TrimSpace(name),
		ProcessImage:        strings.TrimSpace(image),
		ProcessCommand:      strings.TrimSpace(command),
		Username:            strings.TrimSpace(username),
	}, nil
}

func waitForProcessExit(
	ctx context.Context,
	process *gopsprocess.Process,
	timeout time.Duration,
) (bool, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		running, err := isProcessEffectivelyRunning(ctx, process)
		if err != nil {
			return true, nil
		}
		if !running {
			return true, nil
		}
		time.Sleep(responseActionPollInterval)
	}
	running, err := isProcessEffectivelyRunning(ctx, process)
	if err != nil {
		return true, nil
	}
	return !running, nil
}

func isProcessEffectivelyRunning(
	ctx context.Context,
	process *gopsprocess.Process,
) (bool, error) {
	running, err := process.IsRunningWithContext(ctx)
	if err != nil {
		if errors.Is(err, gopsprocess.ErrorProcessNotRunning) {
			return false, nil
		}
		return false, err
	}
	if !running {
		return false, nil
	}
	statuses, err := process.StatusWithContext(ctx)
	if err != nil {
		if errors.Is(err, gopsprocess.ErrorProcessNotRunning) {
			return false, nil
		}
		return running, nil
	}
	for _, status := range statuses {
		if strings.TrimSpace(status) == gopsprocess.Zombie {
			return false, nil
		}
	}
	return true, nil
}

func readCurrentBootID() string {
	raw, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}
