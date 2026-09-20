package ssa

// CompileDiagnostic records a bounded example, not an AST or source buffer.
type CompileDiagnostic struct {
	Kind    string `json:"kind"`
	File    string `json:"file"`
	Message string `json:"message"`
}

type CompileDiagnostics struct {
	ASTErrors        int                 `json:"ast_errors"`
	FileErrors       int                 `json:"file_errors"`
	PreHandlerErrors int                 `json:"prehandler_errors"`
	ConfigErrors     int                 `json:"config_errors"`
	TemplatesSkipped int                 `json:"templates_skipped"`
	Examples         []CompileDiagnostic `json:"examples,omitempty"`
}

func (d CompileDiagnostics) Incomplete() bool {
	return d.ASTErrors+d.FileErrors+d.PreHandlerErrors+d.ConfigErrors+d.TemplatesSkipped > 0
}

func (p *Program) RecordCompileDiagnostic(kind, file, message string) {
	if p == nil {
		return
	}
	if app := p.GetApplication(); app != nil {
		p = app
	}
	p.compileDiagnosticsMu.Lock()
	defer p.compileDiagnosticsMu.Unlock()
	d := &p.compileDiagnostics
	switch kind {
	case "ast":
		d.ASTErrors++
	case "file":
		d.FileErrors++
	case "config":
		d.ConfigErrors++
	case "template":
		d.TemplatesSkipped++
	default:
		d.PreHandlerErrors++
	}
	if len(d.Examples) < 20 {
		if len(message) > 512 {
			message = message[:512]
		}
		d.Examples = append(d.Examples, CompileDiagnostic{Kind: kind, File: file, Message: message})
	}
}

func (p *Program) CompileDiagnostics() CompileDiagnostics {
	if p == nil {
		return CompileDiagnostics{}
	}
	if app := p.GetApplication(); app != nil {
		p = app
	}
	p.compileDiagnosticsMu.Lock()
	defer p.compileDiagnosticsMu.Unlock()
	d := p.compileDiagnostics
	d.Examples = append([]CompileDiagnostic(nil), d.Examples...)
	return d
}
