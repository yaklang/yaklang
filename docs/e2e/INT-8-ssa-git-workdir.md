# INT-8 SSA Git Workspace Verification

This document travels in the same Yaklang pull request as the implementation.
The task changes engine and Scan Node filesystem behavior, so its end-to-end
entry is a real Git code-source compilation and child-process lifecycle test;
there is no browser surface.

## Acceptance coverage

| Requirement | Evidence |
| --- | --- |
| Explicit workdir and stable default | `ssagitworkdir` unit tests plus the real clone scenario below |
| Writable/free-space preflight | Injected low-space test asserts directory, available bytes, required minimum, and remediation |
| Large repository/history/LFS strategy | `common/yak/ssaapi/docs/ssa-git-workdir.md` |
| Cleanup after success and clone failure | `ssa_compile_info_git_test.go` |
| Cleanup after timeout/cancellation | Scan Node child-process cancellation test |
| Concurrent isolation | Eight simultaneous PID-fallback workspaces plus separate owner cleanup |
| Custom workdir | Configured-root unit test and real clone scenario |

## Commands

```bash
YAKIT_HOME=/tmp/int8-e2e-yakit-home GOCACHE=/tmp/int8-e2e-gocache \
  go test ./common/yak/ssaapi/ssagitworkdir -count=1
YAKIT_HOME=/tmp/int8-e2e-yakit-home GOCACHE=/tmp/int8-e2e-gocache \
  go test ./common/yak/ssaapi -run 'TestGit(FS|SourceLocalRepository)' -count=1
YAKIT_HOME=/tmp/int8-e2e-yakit-home GOCACHE=/tmp/int8-e2e-gocache \
  go test ./scannode -run 'Test(ExecuteScriptCleansChildSSAGitWorkspace|ReplaceEnvironmentValue)' -count=1
YAKIT_HOME=/tmp/int8-e2e-yakit-home GOCACHE=/tmp/int8-e2e-gocache \
  go vet ./common/yak/ssaapi/ssagitworkdir ./common/yak/ssaapi ./scannode
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go test -c -o /tmp/int8-e2e-windows.test.exe ./common/yak/ssaapi/ssagitworkdir
CGO_ENABLED=0 GOOS=freebsd GOARCH=amd64 \
  go test -c -o /tmp/int8-e2e-freebsd.test ./common/yak/ssaapi/ssagitworkdir
git diff --check
```

## Result (2026-08-09)

All commands above passed. `TestGitSourceLocalRepositoryEndToEnd` created and
committed a real local Git repository, invoked `ParseProject` through the Git
code-source path with a custom managed root, compiled Yak source, and verified
that the clone workspace was removed. The Scan Node tests ran real child
processes and verified cleanup after normal exit and `context` cancellation.

The full unfiltered `ssaapi` and `scannode` suites were also attempted in the
restricted runner. They reached unrelated existing tests but could not be used
as a gate because the default Yakit database was read-only and `httptest`
listeners were denied. Targeted reruns used a task-owned `YAKIT_HOME` and passed.

The workspace component cross-compiled for Windows and FreeBSD. A native
Windows run was not available in this Linux environment. Broader cross-platform
builds of all SSA/Scan Node dependencies remain blocked by existing platform
constraints in third-party PCRE, pcap, RPM database, and privileged packages;
this patch does not add those dependencies.

Docker was checked read-only after the user started it. The daemon was healthy;
no task container or product stack was started. This engine-only task has no UI
entry, so browser screenshots are not applicable. The independent HTML report
was retained as a session artifact outside the repository, while all
`/tmp/int8-e2e-*` caches, binaries, logs, processes, and listeners were removed.

## Verification tier

T3 achieved for the original INT-8 requirements: T1 targeted code/static checks
passed, T2 exercised the real local-repository Git clone and SSA compilation
entry plus Scan Node cancellation, and every acceptance item above has code,
test, or operational-documentation evidence. Native Windows runtime execution
and the repository's pre-existing broad cross-platform dependency gaps remain
explicit residual evidence limits rather than claimed verification.
