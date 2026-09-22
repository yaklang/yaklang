package ci_antiviral_check

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPackageRunnerDiscoveryFailureIsVisible(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Essential Tests package runner runs on Unix")
	}
	for _, tool := range []string{"bash", "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []string{"pass", "discovery-failure", "test-failure"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			write := func(name, body string) string {
				t.Helper()
				path := filepath.Join(dir, name)
				if err := os.WriteFile(path, []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
				return path
			}
			write("go", `#!/usr/bin/env bash
if [[ "$1" == list ]]; then
  if [[ "$RUNNER_CASE" == discovery-failure && "$4" == ./missing ]]; then
    echo 'fixture package discovery failed' >&2
    exit 1
  fi
  echo github.com/yaklang/yaklang/fixture
  exit 0
fi
if [[ "$RUNNER_CASE" == test-failure ]]; then
  echo '--- FAIL: TestFixture (0.10s)'
  echo 'FAIL github.com/yaklang/yaklang/fixture 0.10s'
  exit 1
fi
echo '--- PASS: TestFixture (0.10s)'
echo 'ok github.com/yaklang/yaklang/fixture 0.10s'
`)
			config := write("config.json", `[{"package":"./missing"},{"package":"./fixture"}]`)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "bash", filepath.Join(repoRoot(t), "scripts", "ci", "test-run-pkg.sh"))
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"RUNNER_CASE="+mode, "TEST_CONFIG="+config, "TEST_LOG_DIR="+filepath.Join(dir, "logs"),
				"PACKAGE_WORKERS=1", "PACKAGE_PARALLEL=", "TEST_VERBOSE=1")
			output, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("runner timed out: %s", output)
			}
			if mode == "pass" {
				if err != nil || !strings.Contains(string(output), "Result: ALL PASSED") {
					t.Fatalf("passing runner: %v\n%s", err, output)
				}
				return
			}
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 1 || !strings.Contains(string(output), "Result: SOME FAILED") {
				t.Fatalf("failure must propagate: %v\n%s", err, output)
			}
			if mode == "discovery-failure" {
				for _, want := range []string{"::error::go list failed for ./missing", "fixture package discovery failed", "PASS: ./fixture"} {
					if !strings.Contains(string(output), want) {
						t.Fatalf("missing %q in runner output:\n%s", want, output)
					}
				}
			}
		})
	}
}
