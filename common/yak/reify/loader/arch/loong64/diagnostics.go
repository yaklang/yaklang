package loong64

import "github.com/yaklang/yaklang/common/yak/reify/core"

const maxDiagnostics = 64

// diagnostics collects parse diagnostics up to a fixed limit, so a corrupt
// region cannot grow the report without bound.
type diagnostics struct {
	items   []core.Diagnostic
	dropped int
}

func (d *diagnostics) add(code, message string) {
	if len(d.items) < maxDiagnostics {
		d.items = append(d.items, core.Diagnostic{Code: code, Message: message})
		return
	}
	d.dropped++
}

func (d *diagnostics) finish() []core.Diagnostic {
	if d.dropped == 0 {
		return d.items
	}
	return append(d.items, core.Diagnostic{
		Code:    "diagnostics-truncated",
		Message: "further " + architectureName + " diagnostics were omitted",
	})
}
