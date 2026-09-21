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

## Verification

```sh
scripts/ci/dockerhttp-fast.sh
GOWORK=off go test -race ./common/dockerhttp -count=1 -timeout=10s
GOWORK=off go test ./scannode -run '^(TestDockerRuntime|TestRuntimeHost|TestResilienceRuntimeHost)' -count=1
GOWORK=off go test ./common/thirdpartyservices -count=1
```

The fast suite includes every client mock and the actual scannode adapter's
contract tests. Compilation is separate; the test phase has a total 10-second
budget. CI runs its binaries in a network namespace with only loopback enabled,
without access to an external network or a Docker daemon. Separate jobs run
call-site/SCA regressions and Windows named-pipe mocks.

Optional real-daemon tests require preinstalled `alpine:3.20` and `nginx:alpine`:

```sh
go test -tags=docker_integration ./common/dockerhttp -run '^TestLive' -count=1 -timeout=6m -v
```

They cover a local registry pull, CPU/memory limits, exec, label lookup, NAT HTTP,
save/load identity, export/import, and idempotent removal. Containers and image
tags created by tests are uniquely named and cleaned up; existing tags are not
removed. Pull uses a temporary registry bound to the daemon's loopback. Set
`DOCKERHTTP_TEST_IMAGE` to override Alpine for the archive test.
