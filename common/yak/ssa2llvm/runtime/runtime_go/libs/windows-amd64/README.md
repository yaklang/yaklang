# Windows bdwgc archive

`libgc.a` is extracted from the MSYS2 package
`mingw-w64-x86_64-gc-8.2.8-1-any.pkg.tar.zst`
(`mingw64/lib/libgc.a`). It is committed so a Windows build links the garbage
collector without building bdwgc first.

- sha256: `7e7314af56cb27bd9b43cc5fb2290b93b3874a62f65c5b9065affaadf5f85a74`
- source: https://mirror.msys2.org/mingw/mingw64/

To refresh, extract `mingw64/lib/libgc.a` from a newer package and update the
hash above.
