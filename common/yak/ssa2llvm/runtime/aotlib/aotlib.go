// Package aotlib provides lightweight yaklib export tables for the ssa2llvm
// AOT runtime.
//
// The monolithic github.com/yaklang/yaklang/common/yak/yaklib package pulls
// the entire yaklang frontend stack (typescript/java/php/python parsers,
// goja, ssaapi, ...) into every AOT binary through its import graph. The AOT
// runtime therefore registers module exports from this package instead, which
// only imports the implementing subpackages and the standard library. The
// normal yaklang interpreter keeps using the full yaklib package unchanged.
package aotlib
