package ssa

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

func lazyDisplayName(full string) string {
	if i := strings.Index(full, "||"); i >= 0 {
		full = full[:i]
	}
	switch {
	case strings.HasPrefix(full, "Function:"):
		return strings.TrimPrefix(full, "Function:")
	case strings.HasPrefix(full, "Blueprint:"):
		return strings.TrimPrefix(full, "Blueprint:")
	default:
		return full
	}
}

func traceDescFromRange(r *memedit.Range) string {
	if r == nil {
		return ""
	}
	file, line := "", ""
	if ed := r.GetEditor(); ed != nil {
		file = ed.GetUrl()
	}
	if start := r.GetStart(); start != nil && start.GetLine() > 0 {
		line = fmt.Sprintf("%d", start.GetLine())
	}
	file = strings.TrimPrefix(strings.TrimSpace(file), "/")
	if file == "" {
		return ""
	}
	if i := strings.Index(file, "/"); i >= 0 && i < len(file)-1 {
		file = file[i+1:]
	}
	line = strings.TrimSpace(line)
	if line != "" {
		return file + ":" + line
	}
	return file
}

func lazyBuildLab(lb *LazyBuilder, prog *Program, rng *memedit.Range) diagnostics.Lab {
	name := lazyDisplayName(lb._lazybuild_name)
	desc := traceDescFromRange(rng)
	if desc == "" && prog != nil {
		if fun := prog.GetFunction(name, prog.PkgName); fun != nil {
			desc = traceDescFromRange(fun.GetRange())
		}
	}
	opts := []diagnostics.LabOption{
		diagnostics.LabKind("lazybuild"),
		diagnostics.LabName(name),
		diagnostics.LabText(name),
	}
	if desc != "" {
		opts = append(opts, diagnostics.LabDesc(desc))
	}
	return diagnostics.NewLab(opts...)
}

// runLazyBuilder executes lb.Build(), wrapping with TRACE only when diagnostics is active.
func (p *Program) runLazyBuilder(lb *LazyBuilder, rng *memedit.Range) {
	if lb == nil {
		return
	}
	rec := p.diagnosticsRecorderForChild()
	if rec == nil {
		lb.Build()
		return
	}
	_ = rec.TraceLab(lazyBuildLab(lb, p, rng), func() error {
		lb.Build()
		return nil
	})
}
