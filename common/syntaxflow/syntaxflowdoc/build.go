package syntaxflowdoc

import (
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

// BuildOptions controls live document assembly.
type BuildOptions struct {
	// BuiltinRulesDir overrides DefaultBuiltinRulesDir(); empty uses default.
	BuiltinRulesDir string
	// NativeCalls supplies NativeCall docs (typically from ssaapi.NativeCallDocuments).
	NativeCalls []NativeCallInfo
	// SkipBuiltinLibs skips scanning .sf include libraries.
	SkipBuiltinLibs bool
}

// BuildDocumentHelper assembles a full SyntaxFlowDoc index from:
//   - static operator / syntax / opcode / desc catalogs
//   - optional NativeCall rows
//   - builtin include libs under sfbuildin/buildin (unless skipped)
func BuildDocumentHelper(opts ...BuildOptions) *DocumentHelper {
	var opt BuildOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	h := NewEmptyDocumentHelper()
	seedOperatorAndSyntaxCatalog(h)
	seedDescKeyCatalog(h)
	seedOpcodeCatalog(h)

	if len(opt.NativeCalls) > 0 {
		CollectNativeCalls(h, opt.NativeCalls)
	}

	if !opt.SkipBuiltinLibs {
		dir := strings.TrimSpace(opt.BuiltinRulesDir)
		if dir == "" {
			dir = DefaultBuiltinRulesDir()
		}
		if dir != "" {
			if err := CollectBuiltinLibsFromDir(h, dir); err != nil {
				log.Warnf("syntaxflowdoc: collect builtin libs from %s: %v", dir, err)
			}
		}
	}
	return h
}
