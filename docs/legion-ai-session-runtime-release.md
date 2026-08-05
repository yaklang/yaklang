# Legion AI Session Runtime Release

The AI Session Runtime is the containerized `legion-smoke-node` execution form
used for `kind=ai_session`. It is not `legion-sessionmgr`: Legion owns the
manager process, while Yaklang owns the Runtime image and its producer
provenance.

## Workflow

`Build Trusted Legion AI Session Runtime` has three entry modes:

| Event | Behavior | Output trust level |
|---|---|---|
| Pull request | Build, test, inspect, and upload preview provenance | Preview; rejected by customer packaging |
| `workflow_dispatch` on `main` | Build and attest trusted provenance without publishing a distribution object | Trusted producer verification |
| SemVer `legion-runtime-v*` tag reachable from `main` | Publish the immutable image archive, provenance, and release index to OSS | Trusted producer release |

The isolated tag prefix intentionally does not match the repository's generic
`v*` release workflows. For example, an alpha producer release can use
`legion-runtime-v0.1.0-alpha.1`.

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
└── release-index.json.sha256
```

The `legion-ai-session-runtime_linux_amd64` Actions artifact retains the
provenance files and release index for short-term workflow inspection. It is
not the cross-repository distribution channel.

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
