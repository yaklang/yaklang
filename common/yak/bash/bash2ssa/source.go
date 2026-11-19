package bash2ssa

import (
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"mvdan.cc/sh/v3/syntax"
	"path"
	"strings"
)

// Index only project sources. A literal `source` in analyzed text must never
// read an arbitrary host path or consult the analyzer process's PATH.
func (*SSABuilder) PreHandlerProject(_ fi.FileSystem, _ ssa.FrontAST, b *ssa.FunctionBuilder, e *memedit.MemEditor) error {
	b.GetProgram().SetEditor(e.GetUrl(), e)
	return nil
}

func (ab *astBuilder) source(w *syntax.Word, fields []ssa.Value, spread bool) (ssa.Value, bool) {
	name, ok := staticWord(w)
	if !ok || name == "" || path.IsAbs(name) || ab.GetEditor() == nil {
		return nil, false
	}
	p := ab.GetProgram()
	current := ab.GetEditor()
	url := path.Join(path.Dir(current.GetUrl()), name)
	e, ok := p.GetEditor(url)
	if !ok {
		e, ok = p.GetEditor(strings.TrimPrefix(url, "/"))
	}
	if !ok {
		e, ok = p.GetEditor("/" + url)
	}
	if !ok || e == nil || ab.sourcing[e.GetUrl()] || len(ab.sourcing) >= 32 {
		return nil, false
	}
	f, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(e.GetSourceCode()), url)
	if err != nil {
		return nil, false
	}
	ab.sourcing[e.GetUrl()] = true
	defer delete(ab.sourcing, e.GetUrl())
	ab.SetEditor(e)
	defer ab.SetEditor(current)
	if len(fields) > 1 {
		saved := ab.positional("@")
		ab.put("$@", ab.packFields(fields[1:], spread))
		defer ab.put("$@", saved)
	}
	ab.put(statusName, ab.EmitConstInst(0))
	return ab.buildStmts(f.Stmts), true
}
