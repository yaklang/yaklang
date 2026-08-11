# Legion AI Session Runtime Release

The AI Session Runtime is the containerized `legion-smoke-node` execution form
used for `kind=ai_session`. It is not `legion-sessionmgr`: Legion owns the
manager process, while Yaklang owns the Runtime image and its producer
provenance.

## Workflow

`Build Trusted Legion AI Session Runtime` is a release-only workflow:

| Event | Behavior | Output trust level |
|---|---|---|
| Pull request or ordinary branch push | Does not run or produce a Runtime package | No release artifact |
| `legion-runtime-alpha-*` tag on any selected commit | Build Linux amd64 and arm64 image archives and provenance packages; retain Actions artifacts for 7 days without OSS credentials or publication | Short-lived integration candidate |
| SemVer `legion-runtime-v*` tag reachable from `main` | Build amd64 and arm64 variants, then publish immutable image archives, provenance, and release indexes to OSS | Trusted producer release |

The isolated tag prefix intentionally does not match the repository's generic
`v*` release workflows. For example, an alpha producer release can use
`legion-runtime-v0.1.0-alpha.1`.

For short-lived testing of a pull request commit, use the separate candidate
namespace:

```bash
git tag legion-runtime-alpha-0212 <commit-sha>
git push origin refs/tags/legion-runtime-alpha-0212
```

The tagged commit is the complete source identity; the tag does not carry or
resolve a pull request number. Each candidate tag must be new and immutable.
The tagged commit must already contain the alpha entry workflow. Candidate
artifacts include `linux/amd64` and `linux/arm64` Docker image archives plus
their provenance packages. They are Actions-only and never receive OSS release
secrets.

## Published outputs

The tag workflow publishes a versioned public OSS directory:

```text
https://yaklang.oss-accelerate.aliyuncs.com/legion/components/session-runtime/<tag>/<source-sha>/
├── legion-ai-session-runtime_linux_amd64.docker.tar.gz
├── legion-ai-session-runtime_linux_amd64.tar.gz
├── legion-ai-session-runtime_linux_amd64.tar.gz.sha256
├── SESSION_RUNTIME_MANIFEST.json
├── SHA256SUMS
├── release-index.json
├── release-index.json.sha256
├── legion-ai-session-runtime_linux_arm64.docker.tar.gz
├── legion-ai-session-runtime_linux_arm64.tar.gz
├── legion-ai-session-runtime_linux_arm64.tar.gz.sha256
├── SESSION_RUNTIME_MANIFEST_linux_arm64.json
├── SHA256SUMS_linux_arm64
├── release-index-linux-arm64.json
└── release-index-linux-arm64.json.sha256
```

The separate `legion-ai-session-runtime_linux_amd64` and
`legion-ai-session-runtime_linux_arm64` Actions artifacts retain provenance
files and release indexes for short-term workflow inspection. They are not the
cross-repository distribution channel. The legacy `release-index.json` selects
amd64; `release-index-linux-arm64.json` selects arm64.

The deployable Runtime binary stays inside the Docker image archive. The
release index binds every OSS object to its SHA-256 digest and records the
loaded image ID. Customer assembly supplies the independently approved index
digest, downloads the archive without repository credentials, and verifies the
image ID and source revision label before packaging.

`SESSION_RUNTIME_MANIFEST.json` binds the Yaklang source commit, workflow run,
Dockerfile and its context-ignore policy, immutable builder and runtime base
images, embedded binary digest, immutable image ID, and the
`ai.session.runtime` plus `yak.execute` capabilities.

## Release boundary

This workflow does not build Legion, assemble a customer package, deploy a
host, or prove a real model conversation. Final assembly consumes this Runtime
release together with the Product Node OSS release and a trusted Legion bundle.
Because no container registry is part of this release contract, packages
containing `memfit` use offline delivery and carry the verified image archive.
Runtime acceptance still requires deployment, License and Provider
configuration, and the real two-turn AI conversation verifier.
