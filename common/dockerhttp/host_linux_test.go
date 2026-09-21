//go:build linux

package dockerhttp_test

import (
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/dockerhttp"
)

func TestLinuxDefaultDockerHost(t *testing.T) {
	if dockerhttp.DefaultDockerHost != "unix:///var/run/docker.sock" {
		t.Fatalf("DefaultDockerHost=%q", dockerhttp.DefaultDockerHost)
	}
}

func TestLinuxHostFromEnvDefault(t *testing.T) {
	t.Setenv(dockerhttp.EnvOverrideHost, "")
	os.Unsetenv(dockerhttp.EnvOverrideHost)
	got := dockerhttp.HostFromEnv()
	if got != "unix:///var/run/docker.sock" {
		t.Fatalf("HostFromEnv=%q", got)
	}
}
