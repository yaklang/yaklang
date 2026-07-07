//go:build irify_exclude

package yak

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

const irifyExcludeLibMsg = "not available in yak-slim (irify_exclude) build"

func irifyExcludeLibError(name string) error {
	return utils.Errorf("%s: %s", name, irifyExcludeLibMsg)
}

func naslStubOpt() func(any) {
	return func(any) {}
}

func initIrifyLibs() {
	log.Info("irify_exclude mode: SSA, SyntaxFlow and NASL libraries are replaced with stubs")
	yaklang.Import("nasl", map[string]any{
		"UpdateDatabase": func(string) {
			log.Warnf("nasl.UpdateDatabase: %s", irifyExcludeLibMsg)
		},
		"RemoveDatabase": func() error {
			return irifyExcludeLibError("nasl.RemoveDatabase")
		},
		"QueryAllScripts": func(...any) any {
			return nil
		},
		"ScanTarget": func(string, ...any) (any, error) {
			return nil, irifyExcludeLibError("nasl.ScanTarget")
		},
		"Scan": func(string, string, ...any) any {
			ch := make(chan any)
			close(ch)
			return ch
		},
		"plugin":      func(...string) func(any) { return naslStubOpt() },
		"family":      func(string) func(any) { return naslStubOpt() },
		"riskHandle":  func(func(any)) func(any) { return naslStubOpt() },
		"proxy":       func(...string) func(any) { return naslStubOpt() },
		"conditions":  func(...any) func(any) { return naslStubOpt() },
		"sourcePaths": func(...string) func(any) { return naslStubOpt() },
	})
}
