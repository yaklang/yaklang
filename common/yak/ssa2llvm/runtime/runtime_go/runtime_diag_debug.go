//go:build ssa2llvm_runtime_debug

package main

import (
	"fmt"
	"os"
)

func runtimeGCLogEnabled() bool {
	v := os.Getenv("GCLOG")
	return v != "" && v != "0"
}

func runtimeDiagPrintf(format string, args ...any) {
	fmt.Printf(format, args...)
}

func runtimeLogPanicRecovery(kind string, recovered any) {
	if recovered == nil {
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "[yak-runtime] %s: %v\n", kind, recovered)
}
