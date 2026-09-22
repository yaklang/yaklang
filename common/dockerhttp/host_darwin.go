//go:build darwin

package dockerhttp

import (
	"os"
	"path/filepath"
)

// DefaultDockerHost is the official CLI constant for macOS.
// Adapted from moby/moby client.DefaultDockerHost @ v25.0.6 (Apache-2.0).
const DefaultDockerHost = "unix:///var/run/docker.sock"

// resolveDefaultHost picks a reachable local Docker socket on macOS.
//
// Precedence (when DOCKER_HOST is unset):
//  1. unix:///var/run/docker.sock — official default; Docker Desktop may
//     create a symlink here to the Desktop socket when admin install is enabled
//  2. unix://${HOME}/.docker/run/docker.sock — Docker Desktop user socket
//     (desktop-linux context) when the /var/run symlink is absent
//  3. DefaultDockerHost (even if missing) so dial errors are explicit
//
// DOCKER_HOST always wins via HostFromEnv before this is called.
// Full docker context JSON parsing is intentionally not implemented.
func resolveDefaultHost() string {
	candidates := []string{DefaultDockerHost}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates,
			"unix://"+filepath.Join(home, ".docker", "run", "docker.sock"),
		)
	}
	for _, c := range candidates {
		h, err := ParseHostURL(c)
		if err != nil {
			continue
		}
		if st, err := os.Stat(h.Addr); err == nil && !st.IsDir() {
			return c
		}
	}
	return DefaultDockerHost
}
