package dockerhttp_test

import (
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/dockerhttp"
)

func TestParseHostURL(t *testing.T) {
	cases := []struct {
		in, proto, addr, base string
		wantErr               bool
	}{
		{"unix:///var/run/docker.sock", "unix", "/var/run/docker.sock", "", false},
		{"unix:///run/user/1000/docker.sock", "unix", "/run/user/1000/docker.sock", "", false},
		{"tcp://127.0.0.1:2375", "tcp", "127.0.0.1:2375", "", false},
		{"tcp://localhost:2375/prefix", "tcp", "localhost:2375", "/prefix", false},
		{"http://127.0.0.1:2375", "http", "127.0.0.1:2375", "", false},
		{"npipe:////./pipe/docker_engine", "npipe", "//./pipe/docker_engine", "", false},
		{"", "", "", "", true},
		{"noscheme", "", "", "", true},
		{"unix://", "", "", "", true},
	}
	for _, tc := range cases {
		h, err := dockerhttp.ParseHostURL(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("ParseHostURL(%q) expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseHostURL(%q): %v", tc.in, err)
		}
		if h.Proto != tc.proto || h.Addr != tc.addr || h.BasePath != tc.base {
			t.Fatalf("ParseHostURL(%q)=%+v want proto=%s addr=%s base=%s", tc.in, h, tc.proto, tc.addr, tc.base)
		}
	}
}

func TestHostFromEnv(t *testing.T) {
	t.Setenv(dockerhttp.EnvOverrideHost, "")
	os.Unsetenv(dockerhttp.EnvOverrideHost)
	if got := dockerhttp.HostFromEnv(); got != dockerhttp.DefaultDockerHost {
		t.Fatalf("default host = %q", got)
	}
	t.Setenv(dockerhttp.EnvOverrideHost, "tcp://1.2.3.4:2375")
	if got := dockerhttp.HostFromEnv(); got != "tcp://1.2.3.4:2375" {
		t.Fatalf("env host = %q", got)
	}
}

func TestFromEnvAPIVersion(t *testing.T) {
	t.Setenv(dockerhttp.EnvOverrideHost, "unix:///tmp/x.sock")
	t.Setenv(dockerhttp.EnvOverrideAPIVersion, "1.40")
	c, err := dockerhttp.New(dockerhttp.FromEnv)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.ClientVersion() != "1.40" {
		t.Fatalf("version=%s", c.ClientVersion())
	}
	if c.DaemonHost() != "unix:///tmp/x.sock" {
		t.Fatalf("host=%s", c.DaemonHost())
	}
}

func TestTLSEnvHelpers(t *testing.T) {
	t.Setenv(dockerhttp.EnvTLSVerify, "")
	os.Unsetenv(dockerhttp.EnvTLSVerify)
	if dockerhttp.TLSVerifyFromEnv() {
		t.Fatal("expected TLSVerify false")
	}
	t.Setenv(dockerhttp.EnvTLSVerify, "1")
	if !dockerhttp.TLSVerifyFromEnv() {
		t.Fatal("expected TLSVerify true")
	}
	t.Setenv(dockerhttp.EnvCertPath, "/tmp/certs")
	if dockerhttp.CertPathFromEnv() != "/tmp/certs" {
		t.Fatalf("cert path=%q", dockerhttp.CertPathFromEnv())
	}
}

func TestParseHostURLNpipeAndUnix(t *testing.T) {
	h, err := dockerhttp.ParseHostURL("npipe:////./pipe/docker_engine")
	if err != nil {
		t.Fatal(err)
	}
	if h.Proto != "npipe" || h.Addr != "//./pipe/docker_engine" {
		t.Fatalf("%+v", h)
	}
}
