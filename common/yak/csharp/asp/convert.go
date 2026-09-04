package asp

import (
	"path/filepath"
	"strings"
	"unicode"

	"github.com/yaklang/antlr/v4"
	aspparser "github.com/yaklang/yaklang/common/yak/csharp/asp/parser"
)

// ConvertToCSharp lowers ASP/ASPX markup to a C# compilation unit, JSP→Java style:
// declarations and runat=server script islands become class members; scriptlets
// become statements in Render(); <%= %> / <%# %> become println(...) so the
// shipped csharp frontend can compile the island.
func ConvertToCSharp(src, filePath string) (string, error) {
	ast, err := Front(src)
	if err != nil {
		return "", err
	}
	var decls []string
	var stmts []string
	var walk func(antlr.Tree)
	walk = func(n antlr.Tree) {
		if n == nil {
			return
		}
		switch x := n.(type) {
		case aspparser.IAspDeclarationContext:
			if t := blobText(x.BlobContent()); t != "" {
				decls = append(decls, t)
			}
			return
		case aspparser.IAspScriptletContext:
			if t := blobText(x.BlobContent()); t != "" {
				stmts = append(stmts, t)
			}
			return
		case aspparser.IAspExpressionContext:
			if t := blobText(x.BlobContent()); t != "" {
				stmts = append(stmts, "println("+trimSemi(t)+");")
			}
			return
		case aspparser.IAspDatabindContext:
			if t := blobText(x.BlobContent()); t != "" {
				stmts = append(stmts, "println("+trimSemi(t)+");")
			}
			return
		case aspparser.IScriptContext:
			open := terminalText(x.SCRIPT_OPEN())
			body := terminalText(x.SCRIPT_BODY())
			if isServerScript(open) {
				if t := stripScriptClose(body); t != "" {
					decls = append(decls, t)
				}
			}
			return
		}
		for i := 0; i < n.GetChildCount(); i++ {
			walk(n.GetChild(i))
		}
	}
	walk(ast)
	className := GeneratedClassName(filePath)
	var b strings.Builder
	b.WriteString("using System;\n")
	b.WriteString("public class ")
	b.WriteString(className)
	b.WriteString(" {\n")
	for _, d := range decls {
		b.WriteString("    ")
		b.WriteString(d)
		if !strings.HasSuffix(strings.TrimSpace(d), "}") && !strings.HasSuffix(strings.TrimSpace(d), ";") {
			b.WriteString(";")
		}
		b.WriteByte('\n')
	}
	b.WriteString("    public void Render() {\n")
	for _, s := range stmts {
		b.WriteString("        ")
		b.WriteString(s)
		if !strings.HasSuffix(strings.TrimSpace(s), "}") && !strings.HasSuffix(strings.TrimSpace(s), ";") {
			b.WriteString(";")
		}
		b.WriteByte('\n')
	}
	b.WriteString("    }\n}\n")
	return b.String(), nil
}

// GeneratedClassName maps a template path to a valid C# class identifier.
func GeneratedClassName(filePath string) string {
	base := filepath.Base(filePath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	if name == "" {
		name = "Page"
	}
	var b strings.Builder
	b.WriteString("Generated_")
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "Generated_" {
		return "Generated_Page"
	}
	return out
}

func blobText(b aspparser.IBlobContentContext) string {
	if b == nil {
		return ""
	}
	return strings.TrimSpace(b.GetText())
}

func terminalText(n antlr.TerminalNode) string {
	if n == nil {
		return ""
	}
	return n.GetText()
}

func isServerScript(open string) bool {
	low := strings.ToLower(open)
	return strings.Contains(low, "runat") && strings.Contains(low, "server")
}

func stripScriptClose(body string) string {
	t := strings.TrimSpace(body)
	low := strings.ToLower(t)
	if i := strings.LastIndex(low, "</script>"); i >= 0 {
		t = strings.TrimSpace(t[:i])
	}
	return t
}

func trimSemi(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), ";")
}
