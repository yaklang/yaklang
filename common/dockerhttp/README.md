# Docker Engine HTTP client

`dockerhttp` replaces the Docker SDK behind scannode's runtime host and the
Postgres/RabbitMQ service helpers. SCA's separate snapshot-only refactor removes
its Docker entry points; this change is stacked on that branch.

The client uses `net/http` for Unix sockets and TCP/TLS. Windows reuses the
repository's existing `go-winio` dependency for named-pipe I/O with cancellation
and deadlines. No Docker SDK, Swarm, BuildKit, or gogo types are imported.
Host parsing attribution is in `third_party/NOTICE`.

```go
c, err := dockerhttp.New(dockerhttp.FromEnv)
// Check err; defer c.Close(); use a bounded context for daemon operations.
```

`DOCKER_HOST` overrides the platform default. macOS also checks Docker Desktop's
`~/.docker/run/docker.sock`. `DOCKER_API_VERSION` forces a version; otherwise the
client negotiates down from API 1.44 (legacy fallback 1.24). Full Docker context
configuration is not parsed. `DOCKER_CERT_PATH` enables TLS and
`DOCKER_TLS_VERIFY` controls certificate verification, matching the old SDK.
With verification enabled and no cert path, certificates are read from `~/.docker`.

Image load/pull/import decode progress streams and return embedded errors or
malformed JSON. A failed start or inspect rolls back only the newly created
container, using a bounded cleanup context even after caller cancellation.
Scannode retains exact cleanup-label matching and rejects multiple matches.

## Tests

```sh
go test ./common/dockerhttp -count=1 -timeout=10s
go test scannode/runtime_host_docker.go scannode/runtime_host_docker_test.go -count=1 -timeout=10s
go test common/thirdpartyservices/docker_images.go common/thirdpartyservices/docker_test.go -count=1 -timeout=10s
```

These tests use temporary local HTTP/Unix/named-pipe mocks, require no daemon,
images, certificates, or external services, and finish within 10 seconds once
compiled. Essential Tests adds only the client and service-pull entries; its
existing scannode entry covers the adapter. No additional jobs or runner setup.

Real-daemon tests are opt-in local checks, requiring preinstalled `alpine:3.20`
and `nginx:alpine`:

```sh
go test -tags=docker_integration ./common/dockerhttp -run '^TestLive' -count=1 -timeout=6m -v
```

They cover a local registry pull, resources, exec, labels, NAT HTTP, save/load
identity, export/import, and idempotent removal. Only uniquely named test
containers and image tags are cleaned up. CI never enables this build tag.
