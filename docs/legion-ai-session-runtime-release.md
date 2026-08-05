# Legion AI Session Runtime Release

The AI Session Runtime is the containerized `legion-smoke-node` execution form
used for `kind=ai_session`. It is not `legion-sessionmgr`: Legion owns the
manager process, while Yaklang owns the Runtime image and its producer
provenance.

## Workflow

`Build Trusted Legion AI Session Runtime` has three entry modes:

| Event | Behavior | Output trust level |
|---|---|---|
| Pull request | Build, test, inspect, and upload preview provenance without pushing an image | Preview; rejected by customer packaging |
| `workflow_dispatch` on `main` | Publish the immutable image and provenance artifact | Trusted producer |
| SemVer `legion-runtime-v*` tag reachable from `main` | Publish the immutable image and provenance artifact | Trusted producer |

The isolated tag prefix intentionally does not match the repository's generic
`v*` release workflows. For example, an alpha producer release can use
`legion-runtime-v0.1.0-alpha.1`.

## Published outputs

The workflow publishes the Runtime image as:

```text
ghcr.io/yaklang/legion-ai-session-runtime@sha256:<digest>
```

The `legion-ai-session-runtime_linux_amd64` Actions artifact contains:

```text
SESSION_RUNTIME_MANIFEST.json
SHA256SUMS
legion-ai-session-runtime_linux_amd64.tar.gz
legion-ai-session-runtime_linux_amd64.tar.gz.sha256
```

The artifact is provenance for the image; the deployable Runtime binary stays
inside the OCI image. Customer assembly must consume the exact image digest,
not its human-readable tag.

`SESSION_RUNTIME_MANIFEST.json` binds the Yaklang source commit, workflow run,
Dockerfile and its context-ignore policy, immutable builder and runtime base
images, embedded binary digest, OCI image digest, and the
`ai.session.runtime` plus `yak.execute` capabilities.

## Release boundary

This workflow does not build Legion, assemble a customer package, deploy a
host, or prove a real model conversation. Final `memfit,ssa` assembly consumes
this Runtime provenance together with the Product Node artifact and a trusted
Legion bundle. Runtime acceptance still requires deployment, License and
Provider configuration, and the real two-turn AI conversation verifier.
