//go:build hids && linux

package scannode

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	gopsprocess "github.com/shirou/gopsutil/v4/process"
)

func TestExecuteProcessTerminate(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep process: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	process, err := gopsprocess.NewProcessWithContext(ctx, int32(cmd.Process.Pid))
	if err != nil {
		t.Fatalf("load process: %v", err)
	}
	startTimeUnixMillis, err := process.CreateTimeWithContext(ctx)
	if err != nil {
		t.Fatalf("load process start time: %v", err)
	}

	result, err := executeProcessTerminate(ctx, hidsResponseActionProcess{
		PID:                 cmd.Process.Pid,
		BootID:              readCurrentBootID(),
		StartTimeUnixMillis: startTimeUnixMillis,
		ProcessName:         "sleep",
		ProcessCommand:      "sleep 30",
	})
	if err != nil {
		t.Fatalf("terminate process: %v (detail=%s)", err, string(result.DetailJSON))
	}

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()

	select {
	case waitErr := <-waitDone:
		var exitErr *exec.ExitError
		if waitErr != nil && !errors.As(waitErr, &exitErr) {
			t.Fatalf("wait terminated process: %v", waitErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for terminated process to exit")
	}
}
