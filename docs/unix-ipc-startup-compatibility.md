# Unix IPC startup and recovery

TCP remains the default. Existing Yakit clients continue to use their original
authenticated loopback TCP startup protocol. Unix IPC is explicitly selected:

```sh
yak grpc --transport unix --socket-path /tmp/yak-engine.sock --local-password "$ENGINE_PASSWORD"
```

Use a real session password. IPC does not also start the default TCP listener.
The caller's absolute socket address must fit in 103 UTF-8 bytes, including
directory names. This is a byte limit, not a character limit.

## Directories and aliases

Existing directory permissions are preserved, including 0755 and 0777. Only a
missing immediate parent is created (0700); missing ancestors must be created
by the caller. The socket itself is 0600.

Directory symlinks are supported, including macOS `/tmp` → `/private/tmp` and
`/var` → `/private/var`. The parent is resolved for file identity checks and
cleanup; bind, dial and ready events retain the original address so expanding
an alias cannot unexpectedly exceed the Unix address length limit. The socket
leaf itself must not be a symlink. Changing the parent alias after startup does
not redirect cleanup to the new target.

## Recovery and concurrent startup

- SIGINT, SIGTERM and SIGHUP stop an IPC server and remove its own socket.
- SIGKILL cannot run cleanup. The next `grpc` start or server-mode
  `check-secret-local-grpc` automatically reclaims an abandoned socket, including
  one left by an older engine.
- Reclamation requires a socket owned by the current effective user, an explicit
  connection-refused result, and unchanged file and parent identities. Access
  errors, timeouts, ordinary files, leaf symlinks and live listeners are not
  grounds for removal. An occupied endpoint reports `ipc_bind_in_use`; the
  existing service remains running.
- A private `<socket-path>.lock` file serializes competing engine listeners,
  including starts through different aliases of the same directory. Its advisory
  lock is released by the OS on process death. The empty 0600 file is deliberately
  retained and reused; deleting it during operation can split the lock between
  two inodes. It contains no password and is not a listening endpoint.
- CLI preflight checks before database initialization. It can recognize a stale
  socket without deleting it; the listener repeats the checks and reclaims it
  while holding the endpoint lock. Preflight success is not a reservation.

The lock coordinates engine processes. Callers should not concurrently replace
the endpoint directory, socket or lock file from another program. Choose another
path when it belongs to a different user or a running service.

## Fast three-platform IPC smoke test

Essential Tests runs three parallel `ipc-smoke` jobs after `prepare-yak` succeeds:
Windows named pipes, macOS Unix sockets, and Linux Unix sockets. All check out the
same prepared revision. They build only the small `engineendpoint` test package
with CGO disabled, not another full engine. No engine download, database loading,
TCP listener or external service is involved.

```powershell
pwsh -File scripts/ci/test-ipc-smoke.ps1
```

The smoke suite verifies real bidirectional IPC payloads, cancellation, occupied
endpoint protection, normal close/reuse, and restart after killing its own server
child while retaining the client handle. Platform cases cover private Windows
ACLs, ordinary-token startup and Unicode names; Unix aliases, stale socket
reclamation, locks, shared directories and preservation of unrelated paths.

The target is 1–2 minutes per job including a small OS-specific Go cache restore
and compilation. Test execution has a 45-second deadline, its CI step a 2-minute
limit, and the whole job a 3-minute guard for runner/setup overhead. Cold downloads
and GitHub runner queues are not guaranteed to fit the target. No matrix failure
is ignored: the Essential Tests Gate and successful-result cache depend on all
three jobs. These are transport smoke tests, not full CLI/auth/database tests.

## Prepared binary CLI acceptance test

After building the native engine, run from the repository:

```sh
YAK_BINARY_PATH=/absolute/path/to/built/yak bash scripts/ci/test-yak-startup.sh
```

CI runs this suite in `prepared-startup`, parallel to the lightweight IPC matrix
and other test jobs after `prepare-yak`. It reuses the prepared Linux executable
and Go build cache; it does not build another engine. The final gate and artifact
cleanup depend on its result. It executes the exact supplied artifact and fails
if that executable is missing; acceptance tests cannot silently skip. The same
command supports explicit native-engine acceptance runs on macOS.

The smoke test covers actual TCP authentication and legacy check output, Unix
authentication, both live endpoint collision paths, no database initialization
on collision, self-check recovery of older abandoned sockets, SIGKILL restart
without manual unlink, SIGINT/SIGTERM/SIGHUP cleanup, shared directories, macOS
`/tmp`, custom aliases and client-mode checks. Endpoint tests additionally cover
concurrent recovery, lock ownership during startup, permission-denied sockets,
ordinary files, leaf/lock symlinks, live stream/datagram sockets, replacement
preservation and short aliases for long real paths.

The script isolates database environment variables before Go package
initialization. Each real engine child uses a separate test home, a deadline and
cleanup of only its own PID. Tests use ephemeral TCP ports and random Unix paths.
The parallel IPC jobs retain their smoke logs on failure and block the final gate.
