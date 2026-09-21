//go:build !windows && !darwin

package dockerhttp

// DefaultDockerHost is the platform default when DOCKER_HOST is unset.
// Adapted from moby/moby client.DefaultDockerHost @ v25.0.6 (Apache-2.0).
const DefaultDockerHost = "unix:///var/run/docker.sock"

// resolveDefaultHost returns the Linux default daemon URL.
// Precedence when DOCKER_HOST is unset: DefaultDockerHost only
// (unix:///var/run/docker.sock). Rootless users should set DOCKER_HOST
// to unix:///run/user/$UID/docker.sock explicitly.
func resolveDefaultHost() string {
	return DefaultDockerHost
}
