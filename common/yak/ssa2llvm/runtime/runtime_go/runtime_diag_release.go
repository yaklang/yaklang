//go:build !ssa2llvm_runtime_debug

package main

func runtimeGCLogEnabled() bool { return false }

func runtimeDiagPrintf(format string, args ...any) {}

func runtimeLogPanicRecovery(kind string, recovered any) {}
