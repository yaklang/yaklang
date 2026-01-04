//go:build irify_exclude

package yak

import (
	"github.com/yaklang/yaklang/common/log"
)

func initIrifyLibs() {
	log.Info("irify_exclude mode: SSA and SyntaxFlow libraries are replaced with stubs for frontend hints")
}
