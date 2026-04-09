//go:build unix

package node

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const nodeLockHelperEnv = "LEGION_NODE_LOCK_HELPER"

func TestAcquireNodeInstanceLockRejectsAnotherProcess(t *testing.T) {
	if os.Getenv(nodeLockHelperEnv) == "1" {
		helperHoldNodeLock()
		return
	}

	t.Parallel()

	nodeID := fmt.Sprintf("node-lock-%d", time.Now().UnixNano())
	cmd := exec.Command(os.Args[0], "-test.run=TestAcquireNodeInstanceLockRejectsAnotherProcess")
	cmd.Env = append(os.Environ(), nodeLockHelperEnv+"=1", "LEGION_NODE_LOCK_NODE_ID="+nodeID)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("open helper stdout: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			ready <- scanner.Text()
			return
		}
		ready <- ""
	}()

	select {
	case line := <-ready:
		if line != "locked" {
			t.Fatalf("unexpected helper readiness output: %q", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for helper to acquire node lock")
	}

	lock, err := acquireNodeInstanceLock(nodeID)
	if err == nil {
		_ = lock.Release()
		t.Fatal("expected duplicate node lock acquisition to fail")
	}
	if !strings.Contains(err.Error(), "already running in another process") {
		t.Fatalf("unexpected duplicate lock error: %v", err)
	}
}

func TestSanitizeNodeIDForFilename(t *testing.T) {
	t.Parallel()

	got := sanitizeNodeIDForFilename(" Node A/1 ")
	if got != "Node-A-1" {
		t.Fatalf("unexpected sanitized node id: %s", got)
	}
}

func helperHoldNodeLock() {
	nodeID := os.Getenv("LEGION_NODE_LOCK_NODE_ID")
	lock, err := acquireNodeInstanceLock(nodeID)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stdout, "lock-error:"+err.Error())
		os.Exit(2)
	}
	defer func() {
		_ = lock.Release()
	}()

	_, _ = fmt.Fprintln(os.Stdout, "locked")
	time.Sleep(30 * time.Second)
}
