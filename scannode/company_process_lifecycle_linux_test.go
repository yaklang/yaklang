//go:build linux

package scannode

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestResilienceCompanyTaskCancellationStopsChildTools(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 30 & echo $!; wait")
	configureScriptProcessCancellation(cmd)
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(output)
	if !scanner.Scan() {
		cancel()
		_ = cmd.Wait()
		t.Fatal("child PID unavailable")
	}
	pid, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
	if err != nil {
		cancel()
		_ = cmd.Wait()
		t.Fatal(err)
	}
	cancel()
	if err := cmd.Wait(); err == nil {
		t.Fatal("cancelled process reported success")
	}
	deadline := time.Now().Add(time.Second)
	for {
		stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if os.IsNotExist(err) {
			return
		}
		if err == nil {
			suffix := string(stat)[strings.LastIndex(string(stat), ")")+2:]
			if strings.HasPrefix(suffix, "Z ") {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("spawned tool continued after task cancellation")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
