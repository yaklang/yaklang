//go:build ssa2llvm_pruned_runtime && ssa2llvm_runtime_yakit

package main

import (
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func runtimePrunedYakitExports() map[string]any {
	client := yaklib.NewVirtualYakitClient(func(result *ypb.ExecResult) error {
		if msg := yaklib.ConvertExecResultIntoAIToolCallStdoutLog(result); msg != "" {
			fmt.Fprintln(os.Stdout, msg)
		}
		return nil
	})
	yaklib.InitYakit(client)

	exports := make(map[string]any, len(yaklib.YakitExports))
	for name, value := range yaklib.YakitExports {
		exports[name] = value
	}
	for name, value := range yaklib.GetExtYakitLibByClient(client) {
		exports[name] = value
	}
	return exports
}
