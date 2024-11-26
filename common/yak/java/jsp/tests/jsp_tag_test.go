package tests

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/java/jsp"
	"testing"
)

func TestJSPAST(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{name: "pure html", code: "<html><body><h1>Hello World</h1></body></html>"},
		{name: "core out", code: "<c:out value='${name}'/>"},
		{name: "pure code", code: "<% out.println(\"Hello World\"); %>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			visitor := jsp.NewJSPVisitor()
			ast, err := jsp.GetAST(tt.code)
			require.NoError(t, err)
			visitor.VisitJspDocuments(ast.JspDocuments())
		})
	}

}
