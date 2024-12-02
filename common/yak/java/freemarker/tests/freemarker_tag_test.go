package tests

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/java/freemarker"
	"testing"
)

func TestFreeMarkerAST(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{name: "pure html", code: "<html><body><h1>Hello World</h1></body></html>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			visitor := freemarker.NewFreeMarkerVisitor()
			ast, err := freemarker.GetAST(tt.code)
			require.NoError(t, err)
			visitor.VisitTemplate(ast.Template())
		})
	}
}
