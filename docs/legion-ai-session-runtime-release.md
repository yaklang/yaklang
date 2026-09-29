# AI Session Runtime in the Unified Yaklang Release

The AI Session Runtime is the containerized `legion-smoke-node` execution form
used for `kind=ai_session`. It is not `legion-sessionmgr`: Legion owns the
manager process, while Yaklang owns the executable and container image.

## Release ownership

AI Session Runtime has no independent customer release channel. The trusted
`Build Trusted Legion Product Node` workflow triggered by a `legion-node-v*`
tag builds the host Node and matching Runtime from the same source commit. It
then produces one architecture-specific installation package containing:

- `yaklang-node`;
- `ai-session-runtime.docker.tar.gz`;
- `manifest.json` and `runtime-manifest.json`;
- `release-index.json` and its checksum.

The unified package is published below the immutable Product Node OSS prefix
and is also retained as a workflow artifact. A customer administrator imports
that one file into Legion, confirms the detected Yaklang version, and promotes
one release for new nodes and new AI sessions. There is no `legion-runtime-v*`
formal tag, Runtime catalog, Runtime promotion action, or customer selector.

## Internal candidate workflow

`legion-runtime-alpha-*` remains an Actions-only engineering candidate. It is
useful for validating the container build on Linux amd64 and arm64 before a
unified Yaklang release. It receives no OSS credentials, publishes no stable
index, and cannot become a customer-visible Runtime release.

```bash
git tag legion-runtime-alpha-0212 <commit-sha>
git push origin refs/tags/legion-runtime-alpha-0212
```

Candidate artifacts expire after seven days. A formal release must rebuild the
Runtime through `legion-node-v*`; candidate bytes are never promoted in place.

## Provenance contract

`SESSION_RUNTIME_MANIFEST.json` binds the Yaklang source commit, producing
workflow, target platform, Dockerfile, immutable base images, embedded binary
digest, image archive digest and size, immutable image ID, and the
`ai.session.runtime`, `yak.execute`, bind/turn lifecycle and
`ai.input.managed_attachment.v1` capabilities. Managed input admission and
Linux confinement are specified in [Managed input workspaces](legion-managed-input-workspaces.md).

The Docker archive is exported through the verified build tag rather than a
bare image ID. Its `manifest.json` must retain that non-pullable tag, and the
referenced config blob must hash to the immutable image ID recorded by the
Runtime manifest. This makes `docker load` register an addressable image on
both the classic Docker image store and containerd-backed Docker engines;
archives with `RepoTags=null` are rejected before publication.

The Runtime Host does not require the image ID reported by the target Docker
engine after `docker load` to equal the producer's config digest. Some
containerd-backed Docker versions assign a different target-local descriptor
while importing the same verified archive. Before loading, the host still
requires exactly one fixed non-pullable tag and proves that the referenced
config blob hashes to the producer image ID. It then persists the exact release,
archive, tag, and target-local image ID mapping and re-resolves that mapping
before starting a session container. A missing, retagged, or mismatched local
image is never adopted without re-validating and re-loading the pinned archive.

The unified bundle builder rejects a Node and Runtime pair when their version,
source commit, operating system, architecture, workflow identity, capability
set, manifest digest, or image-archive digest differs. The Runtime archive is
therefore part of the same immutable Yaklang release identity even though it
uses a container image internally.

## Product boundary

Yaklang CI builds and publishes the unified producer package. Legion imports
the package into private object storage, controls the default release, pins a
new AI session before provisioning, loads the exact admitted image when it is
missing, and keeps existing sessions on their persisted release. A real model
conversation still requires deployed Legion, License and Provider
configuration; producing the package alone is not end-to-end acceptance.

## Forge tool capability contract

New Node and Session Runtime manifests declare `ai.forge_release.v1` for immutable
Forge execution, `ai.forge.custom_tools.v1` for custom script/schema snapshots,
and `ai.tools.builtin.v1` for the existing bounded built-in tool adapters.
The aggregate tool key covers DNS/TCP discovery, native HTTP/crawler/fingerprint,
and evidence adapters. Evidence file access additionally requires
`ai.input.managed_attachment.v1` on a supported platform. None of these keys grants
a task additional tool or target permissions; the server-owned immutable profile,
input manifest and execution scope still control each invocation.

Do not advertise `ai.forge.discovery.v1/v2`, `ai.forge.evidence.v1`, or
`ai.forge.http_assessment.v2` in new manifests. Node normalization drops those
retired keys even when an older enrollment record supplies them. Stateful rollback
mode advertises neither Forge protocols nor the aggregate built-in tool key.
Published profile values and their adapter semantics remain unchanged.

Deploy the compatible Legion server before these binaries. Legion accepts the
new aggregate protocol or the exact legacy profile capability from older
releases; an old subset does not imply aggregate support. An older server may
reject the new capability surface. Do not forge legacy declarations to bypass
that admission check. Packaging tests bind the declared capability set to the
compiled Node surface and Runtime manifest fixture.

## Platform-owned Forge final-output recovery

Legion pins `ContextForgeRelease.retry_empty_output` in each invocation. The
runtime honors that value: true permits one extra final-report request on empty
output, while absent/false disables that retry. It never reruns tools or the task
plan. Client model token settings are preserved; there is no Forge token override.
The invocation digest includes the policy; the published definition digest does
not. Imported definitions do not choose this runtime policy.

Upgrade the Yaklang runtime before enabling the new Legion producer. Older
runtimes retain field 19 as an unknown field during definition hashing and can
reject policy-bearing invocations. Deploy these paired changes together; an old
runtime cannot enforce a platform-disabled policy. No extra capability key or
user-facing setting is introduced.
