//go:build windows

package dockerhttp

// DefaultDockerHost is the platform default when DOCKER_HOST is unset.
// Adapted from moby/moby client.DefaultDockerHost @ v25.0.6 (Apache-2.0).
const DefaultDockerHost = "npipe:////./pipe/docker_engine"

func resolveDefaultHost() string {
	return DefaultDockerHost
}
