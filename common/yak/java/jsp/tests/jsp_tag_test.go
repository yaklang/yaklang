package tests

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/java/jsp"
	"testing"
)

func TestAAA(t *testing.T) {
	code :=
		`
<html>
<body>
</body>
</html>
`

	visitor := jsp.NewJSPVisitor()
	ast, err := jsp.GetAST(code)
	require.NoError(t, err)
	visitor.VisitJspDocument(ast.JspDocument())

}
