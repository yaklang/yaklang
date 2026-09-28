# Linux build portability

Linux CI artifacts use `osusergo` so Go's `os/user` reads local `/etc/passwd`
and `/etc/group` instead of entering libc NSS. CGO remains enabled for native
PCRE2/pcap and other dependencies. Non-Linux builds retain their existing tags.
This policy applies to the full/slim/experimental Yaklang engine, Product Node,
portable Linux Node, Session Runtime, SyntaxFlow web server, and Vulinbox.
Linux CI helper engines use the same user lookup policy.

## Build owners

| Artifact | Build owner | Tags |
|---|---|---|
| Full/slim/experimental Linux engine | `.github/scripts/build-linux-manylinux.sh`, called by reuse-build and exp-cross-build | existing gzip_embed/irify_exclude plus osusergo |
| Product Node | `.github/scripts/build-legion-product-node.sh` | hids,osusergo |
| Portable Linux Node | `.github/workflows/build-legion-node-alpha.yml` | osusergo |
| Session Runtime | `docker/session-runtime/Dockerfile` | osusergo |
| SyntaxFlow web server | `.github/workflows/reuse-build-sfweb.yml` | osusergo on Linux |
| Vulinbox | `.github/workflows/release-vulinbox.yml` | osusergo on Linux |

Keep producer manifests and compile-only verification jobs aligned with the
actual flags. Do not replace immutable published bytes or change historical
release locks to pretend an old artifact used new tags.

## Semantics and limits

NSS-only accounts (for example LDAP/NIS) may no longer resolve to names through
Go's os/user. HIDS must retain numeric UID/GID when a display name is unavailable.
This is not a full NSS replacement. Processes requiring NSS-only identity
resolution need a separate explicitly designed integration.

`osusergo` does not change Go DNS resolution or calls made by native libraries.
Static ELF and absence of NEEDED entries do not exclude runtime dlopen. Audit
DNS, SQLite extensions and other native loading separately; no universal glibc
compatibility claim follows from this user lookup fix.

## Artifact regression

Run on an amd64 Docker host with a Linux amd64 binary:

```bash
bash .github/scripts/verify-linux-nss-startup.sh /path/to/legion-smoke-node node /tmp/node-nss-evidence
bash .github/scripts/verify-linux-nss-startup.sh /path/to/yak engine /tmp/engine-nss-evidence
```

The script pins Debian Buster/glibc 2.28 and checks files/no-HOME,
compat/with-HOME, and compat/no-HOME in disposable no-network containers.
It unsets YAKIT_HOME as well, preventing a false pass caused by a directory
workaround. The node must reach main and the explicit missing release identity
check; the engine must execute a Yak script successfully. Timeout, a generic
nonzero exit, and --help output are not success evidence. ARM64 retains native
build checks; this pinned amd64 runtime matrix does not establish ARM64 runtime
compatibility. Registration, heartbeat, real workloads, and target HIDS kernel
collectors remain separate acceptance gates.
