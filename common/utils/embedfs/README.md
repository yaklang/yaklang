# Compressed embedded resources

`embedfs.FS` presents single-file gzip sidecars under the original resource names.
`Open`, `ReadFile`, `ReadDir`, `Stat`, `fs.Sub`, directory iteration and HTTP range
requests retain their normal `io/fs` behavior. Each open has its own reader;
resources are decompressed on demand without a permanent global cache. Existing
`.gz`, `.gzip`, `.zip` and `.zst` resources remain opaque to this layer.

Original source files stay editable. The generator uses gzip level 9 with no
filename or timestamp and emits explicit embed directives, selecting either the
original or its `.embed.gz` sidecar, never both. Files are compressed only when
at least 256 bytes and 20% are saved. The suffix is reserved for generated files;
sidecars are hidden from the public filesystem. Gzip members must be smaller than
4 GiB; `Stat` uses the original size in their trailer without decompressing them.

After editing or adding resources, regenerate and commit the generated Go files
and gzip sidecars:

```sh
go generate ./embed ./common/crep ./common/thirdparty_bin ./common/syntaxflow/sfbuildin/standards ./common/syntaxflow/sfdb
bash scripts/ci/check-embedded-resources.sh
```

The check fails for outdated data, added/deleted sources or stale sidecars. It is
also run by Essential Tests. Resource parity and filesystem tests cover original
bytes, legacy `Asset` decompression, missing files, HTTP output, seeking and
concurrent reads. Compression applies to normal builds as well as `gzip_embed`
release builds. Rule versions retain their `!irify_exclude` build constraint.

This is an on-disk binary size optimization, not a guarantee of lower peak heap
usage: accessing a compressed file necessarily allocates its uncompressed data.
Small embedded prompts and resources already in compressed archives are left in
their current representation to avoid adding global decompressed copies or
recompression overhead.
