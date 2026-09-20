# Oracle password login probe

The built-in `oracle` handler performs TNS/TTC password authentication and closes
the connection without executing SQL. Its production implementation depends only
on the Go standard library. Successful authentication requires a complete login,
a session identity and valid server proof.

## Yak usage

```yak
// Explicit service avoids guessing the default service list.
bruter = brute.New("oracle",
    brute.oracleService("SALES_PDB"),
    brute.userList("audit_user"),
    brute.passList("candidate"),
    brute.okToStop(true))~
for result = range bruter.Start("127.0.0.1:1521")~ {
    println(result.Ok, string(result.ExtraInfo))
}
```

Connection options:

- `brute.oracleService(names...)`: explicit SERVICE_NAME candidates; default is
  `orcl, xe, oracle, orclpdb1, xepdb1, freepdb1, free`. Maximum 32 candidates.
- `brute.oracleSID(name)`: use a SID instead of SERVICE_NAME. The last service/SID
  option wins. An empty SID is rejected before connecting.
- `brute.oracleSysDBA(bool)`: override automatic SYSDBA selection for username SYS.
- `brute.oracleTLS(serverName, caPEM...)~`: require TCPS, with certificate and host
  verification. An empty serverName uses the target host; omitted CA PEM uses
  system trust. There is no plaintext fallback. Supply the listener's TCPS port.
- `brute.oracleEncryption("accepted"|"rejected"|"requested"|"required")~`:
  native network encryption, independent of TCPS. `required` rejects downgrade.
- `brute.oracleTimeout(seconds)~`: total credential budget including all services,
  DNS/dial, TLS, redirects, authentication and retry; default 10s, capped at 20s.

```yak
tlsOpt = brute.oracleTLS("db.example.com", string(file.ReadFile("test-ca.pem")~))~
encOpt = brute.oracleEncryption("required")~
bruter = brute.New("oracle", brute.oracleSID("ORCL"), tlsOpt, encOpt)~
```

Go callers use `WithOracleConfig(&OracleConfig{...})` on a `BruteUtil`, or supply
`BruteItem.OracleConfig` for a direct handler invocation. The configuration is
snapshotted by `WithOracleConfig`; callers must not concurrently mutate a config
attached directly to an item. An explicit Go `tls.Config` can provide additional
certificate policy or client certificates. No full database driver is needed.

## Results and scanning behavior

`ExtraInfo` is JSON with protocol, requested transport, SID/SERVICE_NAME mode, SYSDBA, overall status,
and bounded per-service attempts. Each attempt identifies the service, ORA code,
failure stage, established TCP/TCPS transport (unknown until dial/TLS succeeds), negotiated encryption/integrity and
verifier when available. `cached: true` means a recent unknown-service result was
reused, not a new network exchange. Raw server messages, passwords and session
keys are never added to this metadata. `ProbeResult` preserves the corresponding
core outcome, error category and retry delay through the stream adapter.

Only ORA-01017 is classified as credential rejection. Lock/expiry results are
not successes; if every available service reports a locked/expired account,
remaining passwords for that username are skipped while other accounts continue.
Unsupported credentials (including legacy non-ASCII verifier inputs) and an
incomplete authentication do not eliminate the target. The same applies to
unsupported verifiers and [ORA-28040](https://docs.oracle.com/en/error-help/db/ora-28040/),
which can concern an individual account's verifier, and
[ORA-28041](https://docs.oracle.com/en/error-help/db/ora-28041/), an internal
authentication error. Unsupported transport protocol,
TLS verification failure and unknown service are explicit results, not wrong
passwords. A terminal target result requires all candidate services to have a
terminal failure; a failure of another service cannot invalidate an observed
credential rejection.

Transient I/O, listener capacity and connection-loss errors permit one probe
retry per credential across the entire service list, within its original budget.
A rejected password is never retried. Exhausted transient errors remain
nonterminal, allowing later candidates; the stream applies a short bounded
backoff. The underlying dialer also has its existing connection retry policy,
which now shares the caller's deadline and cancellation.

Each scan has a separate, bounded service cache (4096 entries, 30-second TTL).
Known services are prioritized; only an explicit ORA-12505/12514 is negatively
cached. Other services remain candidates. No account/password data is cached.
Multiple workers already in flight can finish before a user-elimination result
is observed; subsequent scheduled work checks the elimination flag.

## Maintenance and verification

The protocol/crypto implementation was cropped from go-ora v3.0.1
(`025c51529284177330cde75c2e60c137c5f30fe1`) and selected later fixes. The upstream
MIT license is retained in `internal/oracleprobe/LICENSE`. Connection-loss code
classification was adapted from `v3/network/oracle_error.go:OracleError.Bad` at
`360b4b7ac9e96cee3e443180f2d6412bcacee62a`: SQL-only errors were omitted, and login
capacity/timeouts added. Upstream code can be copied selectively with provenance;
do not restore database/sql registration, SQL/LOB codecs or character-map tables.

```sh
go test -race ./common/utils/bruteutils/internal/oracleprobe -count=1 -timeout=30s
go test -race ./common/utils/bruteutils -run 'TestOracle|TestStreamPreserves' -count=1 -timeout=60s
go test ./common/yak/yaklib/tools -run TestOracle -count=1 -timeout=60s
```

Live tests require explicit `YAK_ORACLE_TEST_*` environment variables and an
isolated database; see `internal/oracleprobe/live_test.go`. Existing recorded
version coverage does not imply all Oracle patchsets or RAC/SCAN topologies.
Legacy verifier 2361 still requires ASCII credentials. OS/Kerberos/wallet auth,
SQL and DES/3DES transport remain outside this login-only implementation.
