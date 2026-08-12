# SSA Git Workspace Operations

SSA compilation clones Git code sources into a managed directory instead of
the operating system's generic temporary directory. The default root is:

```text
<YAKIT_HOME>/temp/ssa-git
```

If `YAKIT_HOME` is unset, the existing Yakit home resolution applies.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `YAK_SSA_GIT_WORKDIR` | `<YAKIT_HOME>/temp/ssa-git` | Put clone traffic on a filesystem selected by the operator. Relative values are resolved to absolute paths before use. |
| `YAK_SSA_GIT_MIN_FREE_BYTES` | `1073741824` (1 GiB) | Refuse a new clone when the selected filesystem has less free space. Use a decimal byte count; `0` disables only the minimum threshold, not the writability/free-space check. |

Before cloning, Yaklang creates the root if needed, verifies it is writable,
and checks available bytes and inodes. A capacity failure reports the resolved
directory, available bytes, required minimum, and the configuration variable
that can move clone traffic to a larger filesystem. Clone failures repeat the
workspace and current free-space evidence so an `ENOSPC` reached during transfer
is diagnosable rather than appearing as a generic Git error.

## Repository sizing and compatibility

SSA Git sources retain the existing clone semantics: branch selection is
honored, history is not silently made shallow, and submodules are not recursively
materialized by default. Large or long-lived repositories should set
`YAK_SSA_GIT_MIN_FREE_BYTES` above the expected checkout plus pack-file size and
point `YAK_SSA_GIT_WORKDIR` at a dedicated volume.

The current go-git path does not run the Git LFS smudge filter. Repositories that
need LFS object contents for analysis must materialize them before using a local
code source; Git URL scans see LFS pointer files. This avoids an unbounded hidden
download while making the capacity limitation explicit.

## Ownership and cleanup

Each clone uses `yakgit-<owner>-<random>` under the managed root. The random
suffix isolates concurrent clones. Scan Node derives a stable, hashed owner
scope from its installation identity and gives every Yak child a unique task
owner through the internal `YAK_SSA_GIT_WORKSPACE_OWNER` variable. The raw
installation identity is not written to the filesystem. Standalone Yak
execution falls back to a process-ID owner.

SSA removes its workspace after successful compilation and on clone or
compilation failure. Scan Node also removes the child owner after normal exit,
timeout, or cancellation, covering forced child termination where child defers
cannot run. Scan Node keeps an exclusive node-scope lock in the managed root for
its lifetime. Each child workspace also keeps a shared activity lock while the
child is alive. Startup recovery must acquire the activity lock exclusively
before removing workspaces left by the same installation scope. Therefore a
replacement node cannot delete a clone still used by an orphan child after the
old parent is killed; it can retry after the operating system releases the
child's activity lock. Placing both locks beside the protected workspaces makes
their domain independent of the node's `BaseDir` and applies the same safety on
Unix and Windows.

Operators should not place unrelated data under names beginning with
`yakgit-<owner>-` inside the managed root.
