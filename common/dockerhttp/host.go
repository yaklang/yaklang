package dockerhttp

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Environment variable names matching the official Docker client.
const (
	// EnvOverrideHost overrides the default daemon host (DOCKER_HOST).
	EnvOverrideHost = "DOCKER_HOST"
	// EnvOverrideAPIVersion forces an API version (DOCKER_API_VERSION), e.g. "1.44".
	EnvOverrideAPIVersion = "DOCKER_API_VERSION"
	// EnvTLSVerify enables TLS verification when set to a non-empty value (DOCKER_TLS_VERIFY).
	EnvTLSVerify = "DOCKER_TLS_VERIFY"
	// EnvCertPath is the directory containing ca.pem, cert.pem, key.pem (DOCKER_CERT_PATH).
	EnvCertPath = "DOCKER_CERT_PATH"
)

// DummyHost is used as the HTTP Host for unix/npipe connections.
// Adapted from moby/moby client.DummyHost @ v25.0.6.
const DummyHost = "api.moby.localhost"

// FallbackAPIVersion is used when negotiation fails / old daemons.
const FallbackAPIVersion = "1.24"

// MaxAPIVersion is the highest Engine API version this client speaks.
const MaxAPIVersion = "1.44"

// Host holds a parsed Docker host URL.
type Host struct {
	// Raw is the original host string, e.g. "unix:///var/run/docker.sock".
	Raw string
	// Proto is the scheme: unix, tcp, npipe, fd, etc.
	Proto string
	// Addr is the dial address (socket path or host:port).
	Addr string
	// BasePath is an optional path prefix (tcp hosts only).
	BasePath string
}

// ParseHostURL parses a Docker host string the same way the official client does.
// Adapted from moby/moby client.ParseHostURL @ v25.0.6 (Apache-2.0).
func ParseHostURL(host string) (*Host, error) {
	proto, addr, ok := strings.Cut(host, "://")
	if !ok || addr == "" {
		return nil, fmt.Errorf("unable to parse docker host `%s`", host)
	}

	var basePath string
	if proto == "tcp" || proto == "http" || proto == "https" {
		parsed, err := url.Parse(proto + "://" + addr)
		if err != nil {
			return nil, err
		}
		addr = parsed.Host
		basePath = parsed.EscapedPath()
		if addr == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("invalid Docker host URL %q", host)
		}
	}
	return &Host{
		Raw:      host,
		Proto:    proto,
		Addr:     addr,
		BasePath: basePath,
	}, nil
}

// HostFromEnv returns DOCKER_HOST if set and non-empty, otherwise the
// platform-specific default from resolveDefaultHost (see host_linux.go,
// host_darwin.go, host_windows.go).
func HostFromEnv() string {
	if h := os.Getenv(EnvOverrideHost); h != "" {
		return h
	}
	return resolveDefaultHost()
}

// APIVersionFromEnv returns DOCKER_API_VERSION if set, else "".
func APIVersionFromEnv() string {
	return os.Getenv(EnvOverrideAPIVersion)
}

// TLSVerifyFromEnv reports whether DOCKER_TLS_VERIFY is set to a non-empty value.
func TLSVerifyFromEnv() bool {
	return os.Getenv(EnvTLSVerify) != ""
}

// CertPathFromEnv returns DOCKER_CERT_PATH, or "".
func CertPathFromEnv() string {
	return os.Getenv(EnvCertPath)
}
