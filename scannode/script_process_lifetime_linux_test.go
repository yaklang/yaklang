//go:build linux

package scannode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/urfavecli"
)

const scriptLifetimeTestMode = "YAK_SCRIPT_LIFETIME_TEST_MODE"
const scriptLifetimeTestDir = "YAK_SCRIPT_LIFETIME_TEST_DIR"

// Use the test executable as the node, script, and nested process. Helpers run
// before test flag parsing because executeScript invokes "distyak file.yak".
func init() {
	mode := os.Getenv(scriptLifetimeTestMode)
	if mode == "" {
		return
	}
	dir := os.Getenv(scriptLifetimeTestDir)
	switch mode {
	case "node", "node-cancel":
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if mode == "node-cancel" {
			go func() {
				for {
					if _, err := os.Stat(filepath.Join(dir, "cancel")); err == nil {
						cancel()
						return
					}
					time.Sleep(10 * time.Millisecond)
				}
			}()
		}
		err := (&ScanNode{}).executeScript(&Task{Ctx: ctx, Cancel: cancel}, os.Args[0], "fixture.yak", nil,
			"lifetime-test", []string{scriptLifetimeTestMode + "=script"}, nil)
		if mode == "node-cancel" && err != nil && ctx.Err() != nil {
			os.Exit(0)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		os.Exit(0)
	case "script":
		stop, err := watchScriptParentLifetime()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		defer stop()
		if err := os.WriteFile(filepath.Join(dir, "script.pid"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			os.Exit(2)
		}
		leaf := exec.Command(os.Args[0])
		leaf.Env = replaceEnvironmentValue(os.Environ(), scriptLifetimeTestMode, "leaf")
		if err := leaf.Start(); err != nil {
			os.Exit(2)
		}
		for {
			if _, err := os.Stat(filepath.Join(dir, "finish")); err == nil {
				_ = leaf.Process.Kill()
				_ = leaf.Wait()
				stop()
				os.Exit(0)
			}
			time.Sleep(10 * time.Millisecond)
		}
	case "leaf":
		if err := os.WriteFile(filepath.Join(dir, "leaf.pid"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			os.Exit(2)
		}
	default:
		return
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestScriptLifetimeNodeSIGKILLStopsScriptAndDescendant(t *testing.T) {
	node, script, leaf := startScriptLifetimeTest(t, "node")
	require.NoError(t, node.Process.Kill())
	require.Error(t, node.Wait())
	waitScriptLifetimeExited(t, script, "script survived SIGKILL of its node")
	waitScriptLifetimeExited(t, leaf, "nested execution survived SIGKILL of its node")
}

func TestScriptLifetimeCancellationStopsProcessGroup(t *testing.T) {
	node, script, leaf := startScriptLifetimeTest(t, "node-cancel")
	require.NoError(t, os.WriteFile(filepath.Join(node.Dir, "cancel"), nil, 0600))
	done := make(chan error, 1)
	go func() { done <- node.Wait() }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("node did not finish a canceled execution")
	}
	waitScriptLifetimeExited(t, script, "cancellation left the script running")
	waitScriptLifetimeExited(t, leaf, "cancellation left a nested process running")
}

func TestScriptLifetimeStandaloneDoesNotRequireNode(t *testing.T) {
	t.Setenv(scriptParentLifetimeFDEnv, "")
	stop, err := watchScriptParentLifetime()
	require.NoError(t, err)
	stop()
}

func TestScriptLifetimeHealthyParentAllowsNormalExit(t *testing.T) {
	node, script, leaf := startScriptLifetimeTest(t, "node")
	require.NoError(t, os.WriteFile(filepath.Join(node.Dir, "finish"), nil, 0600))
	done := make(chan error, 1)
	go func() { done <- node.Wait() }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("normal script completion blocked on its lifetime monitor")
	}
	waitScriptLifetimeExited(t, script, "completed script remained alive")
	waitScriptLifetimeExited(t, leaf, "helper left nested work after normal completion")
}

func TestDistYakCommandRejectsInvalidParentDescriptor(t *testing.T) {
	t.Setenv(scriptParentLifetimeFDEnv, "not-a-number")
	app := cli.NewApp()
	app.Commands = []cli.Command{DistYakCommand}
	err := app.Run([]string{"node", "distyak"})
	require.ErrorContains(t, err, "invalid script parent lifetime descriptor")
}

func TestScriptLifetimeRejectsInvalidDescriptor(t *testing.T) {
	for _, value := range []string{"not-a-number", "0", "2", "999999"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(scriptParentLifetimeFDEnv, value)
			_, err := watchScriptParentLifetime()
			require.Error(t, err)
		})
	}
}

func startScriptLifetimeTest(t *testing.T, mode string) (*exec.Cmd, *os.Process, *os.Process) {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0])
	cmd.Dir = dir
	cmd.Env = replaceEnvironmentValue(os.Environ(), scriptLifetimeTestMode, mode)
	cmd.Env = replaceEnvironmentValue(cmd.Env, scriptLifetimeTestDir, dir)
	cmd.Env = replaceEnvironmentValue(cmd.Env, "YAKIT_HOME", filepath.Join(dir, "yakit"))
	cmd.Env = replaceEnvironmentValue(cmd.Env, "LEGION_SCRIPT_MEMORY_GUARD", "0")
	logFile, err := os.Create(filepath.Join(dir, "helper.log"))
	require.NoError(t, err)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = logFile.Close()
		if t.Failed() {
			log, _ := os.ReadFile(filepath.Join(dir, "helper.log"))
			t.Logf("process helper: %s", log)
		}
	})
	children := make([]*os.Process, 0, 2)
	for _, name := range []string{"script", "leaf"} {
		var child *os.Process
		require.Eventually(t, func() bool {
			raw, err := os.ReadFile(filepath.Join(dir, name+".pid"))
			if err != nil {
				return false
			}
			pid, err := strconv.Atoi(string(raw))
			if err != nil || pid <= 1 {
				return false
			}
			child, err = os.FindProcess(pid)
			return err == nil
		}, 20*time.Second, 10*time.Millisecond, "script hierarchy never became ready: %s", name)
		t.Cleanup(func() { _ = child.Kill(); _ = child.Release() })
		children = append(children, child)
	}
	require.False(t, scriptLifetimeProcessExited(children[0].Pid), "a healthy parent must keep its script alive")
	require.False(t, scriptLifetimeProcessExited(children[1].Pid), "a healthy parent must keep nested work alive")
	return cmd, children[0], children[1]
}

func waitScriptLifetimeExited(t *testing.T, process *os.Process, message string) {
	t.Helper()
	require.Eventually(t, func() bool { return scriptLifetimeProcessExited(process.Pid) },
		3*time.Second, 10*time.Millisecond, message)
}

func scriptLifetimeProcessExited(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		return false
	}
	_, fields, ok := strings.Cut(string(data), ") ")
	return ok && (strings.HasPrefix(fields, "Z ") || strings.HasPrefix(fields, "X "))
}
