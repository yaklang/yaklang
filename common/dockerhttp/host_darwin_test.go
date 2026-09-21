//go:build darwin

package dockerhttp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/dockerhttp"
)

func TestDarwinDefaultDockerHostConstant(t *testing.T) {
	if dockerhttp.DefaultDockerHost != "unix:///var/run/docker.sock" {
		t.Fatalf("DefaultDockerHost=%q", dockerhttp.DefaultDockerHost)
	}
}

func TestDarwinHostFromEnvProbesDesktopSock(t *testing.T) {
	t.Setenv(dockerhttp.EnvOverrideHost, "")
	os.Unsetenv(dockerhttp.EnvOverrideHost)

	got := dockerhttp.HostFromEnv()
	if !strings.HasPrefix(got, "unix://") {
		t.Fatalf("HostFromEnv=%q, want unix://...", got)
	}
	// Must be either the official default or the Desktop user socket.
	home, _ := os.UserHomeDir()
	desktop := "unix://" + filepath.Join(home, ".docker", "run", "docker.sock")
	if got != dockerhttp.DefaultDockerHost && got != desktop {
		t.Fatalf("HostFromEnv=%q not in {default, desktop}", got)
	}
}

func TestDarwinHostFromEnvOverride(t *testing.T) {
	t.Setenv(dockerhttp.EnvOverrideHost, "unix:///tmp/custom-docker.sock")
	if got := dockerhttp.HostFromEnv(); got != "unix:///tmp/custom-docker.sock" {
		t.Fatalf("got %q", got)
	}
}
