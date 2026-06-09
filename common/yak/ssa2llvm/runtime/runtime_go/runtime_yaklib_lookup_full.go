//go:build !ssa2llvm_pruned_runtime

package main

import (
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/yak/yaklang"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Trigger yaklang stdlib registration via yak package init for the full runtime.
import _ "github.com/yaklang/yaklang/common/yak"

func init() {
	client := yaklib.NewVirtualYakitClient(func(result *ypb.ExecResult) error {
		if msg := yaklib.ConvertExecResultIntoAIToolCallStdoutLog(result); msg != "" {
			fmt.Fprintln(os.Stdout, msg)
		}
		return nil
	})
	yaklib.InitYakit(client)

	exports := make(map[string]interface{}, len(yaklib.YakitExports))
	for name, value := range yaklib.YakitExports {
		exports[name] = value
	}
	for name, value := range yaklib.GetExtYakitLibByClient(client) {
		exports[name] = value
	}
	yaklang.Import("yakit", exports)
}

func runtimeLookupYaklibCallable(pkg, method string) (any, bool) {
	if pkg == "" {
		return yaklang.LookupGlobalCallable(method)
	}
	return yaklang.LookupExport(pkg, method)
}
