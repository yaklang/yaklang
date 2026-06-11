//go:build !ssa2llvm_pruned_runtime || ssa2llvm_runtime_poc

package main

import "github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"

func init() {
	runtimeRegisterDispatchTarget(abi.IDPocTimeout, runtimePocTimeout)
	runtimeRegisterDispatchTarget(abi.IDPocGet, runtimePocGet)
	runtimeRegisterDispatchTarget(abi.IDPocGetHTTPPacketBody, runtimePocGetHTTPPacketBody)
}
