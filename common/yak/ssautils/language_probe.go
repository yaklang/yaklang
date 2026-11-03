package ssautils

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// ProbeLanguageOrder is the deterministic probe order: when a tree holds
// evidence for more than one language, the earlier one wins. Java is first
// because its wrapper expands archives, and a Java webapp routinely carries a
// lot of JavaScript that must not outvote it.
var ProbeLanguageOrder = []ssaconfig.Language{
	ssaconfig.JAVA,
	ssaconfig.GO,
	ssaconfig.PHP,
	ssaconfig.PYTHON,
	ssaconfig.CSHARP,
	ssaconfig.C,
	ssaconfig.TS,
	ssaconfig.JS,
	ssaconfig.Yak,
}

// probeLanguageByExtension maps the extensions that identify a project's
// language. Deliberately narrow: it holds markers a human would accept as
// "this tree is written in X", not every extension a builder accepts. Notably
// absent are .txt/.xml/.json, which several pre-handlers claim.
var probeLanguageByExtension = map[string]ssaconfig.Language{
	".java":  ssaconfig.JAVA,
	".jsp":   ssaconfig.JAVA,
	".jspx":  ssaconfig.JAVA,
	".class": ssaconfig.JAVA,
	".war":   ssaconfig.JAVA,
	".jar":   ssaconfig.JAVA,
	".go":    ssaconfig.GO,
	".php":   ssaconfig.PHP,
	".py":    ssaconfig.PYTHON,
	".cs":    ssaconfig.CSHARP,
	".c":     ssaconfig.C,
	".h":     ssaconfig.C,
	".cpp":   ssaconfig.C,
	".cc":    ssaconfig.C,
	".ts":    ssaconfig.TS,
	".tsx":   ssaconfig.TS,
	".js":    ssaconfig.JS,
	".jsx":   ssaconfig.JS,
	".yak":   ssaconfig.Yak,
}

// languageProbeFileLimit caps the probe walk. A project declares its language
// long before the two-hundredth file; the cap keeps a tree with no recognizable
// source at all from costing a full traversal.
const languageProbeFileLimit = 200

// ProbeLanguage collects language evidence from the first files of the tree and
// returns the highest-priority language seen. Looking at a sample instead of
// the first matching file keeps a webapp's bundled JavaScript from deciding
// that a Java project is a JavaScript project. An empty result means no
// evidence.
func ProbeLanguage(filesystem fi.FileSystem, root string) ssaconfig.Language {
	if filesystem == nil {
		return ""
	}
	seen := make(map[ssaconfig.Language]bool)
	files := 0
	_ = filesys.Recursive(root,
		filesys.WithFileSystem(filesystem),
		filesys.WithFileStat(func(path string, info fs.FileInfo) error {
			if info == nil || info.IsDir() {
				return nil
			}
			files++
			if lang, ok := probeLanguageByExtension[strings.ToLower(filepath.Ext(path))]; ok {
				seen[lang] = true
			}
			if files >= languageProbeFileLimit {
				return filesys.SkipAll
			}
			return nil
		}),
	)
	for _, lang := range ProbeLanguageOrder {
		if seen[lang] {
			return lang
		}
	}
	return ""
}

// ResolveLanguageBuilder returns the frontend for a project before its archive
// layer is wrapped. An already resolved builder is never replaced: the caller
// may have supplied a probe or a custom builder. Otherwise configured wins and
// ProbeLanguage decides when it is empty. create maps the language onto the
// caller's frontend registry, so this package does not import it.
func ResolveLanguageBuilder(existing ssa.Builder, filesystem fi.FileSystem, root string, configured ssaconfig.Language, create func(ssaconfig.Language) ssa.Builder) ssa.Builder {
	if existing != nil {
		return existing
	}
	if filesystem == nil || create == nil {
		return nil
	}
	lang := configured
	if lang == "" {
		lang = ProbeLanguage(filesystem, root)
	}
	return create(lang)
}
