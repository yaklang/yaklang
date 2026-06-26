package ssa

import (
	"testing"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
)

func Test_Get_RangeByText(t *testing.T) {
	t.Run("test get first range by text", func(t *testing.T) {
		content := `
		server.port=8080
		server.servlet.context-path=/api
		
		spring.datasource.url=jdbc:mysql://localhost:3306/mydb?useSSL=false&serverTimezone=UTC
		spring.datasource.username=root
		spring.datasource.password=secret
		spring.datasource.driver-class-name=com.mysql.cj.jdbc.Driver
		spring.datasource.hikari.connection-timeout=60000
		spring.datasource.hikari.maximum-pool-size=10
		spring.datasource.hikari.idle-timeout=300000
		spring.datasource.hikari.max-lifetime=2000000
		management.endpoints.web.exposure.include=health,info,metrics
		management.endpoint.health.show-details=always
		management.server.port=8081
`
		editor := memedit.NewMemEditor(content)
		rng := GetFirstRangeByText(editor, "spring.datasource.url")
		require.Equal(t, `spring.datasource.url`, rng.GetText())

		rng2 := GetFirstRangeByText(editor, "management.endpoint.health.show-details")
		require.Equal(t, `management.endpoint.health.show-details`, rng2.GetText())
	})

	t.Run("test get ranges by text", func(t *testing.T) {
		content := `
		server.port=8080
		server.servlet.context-path=/api
		
		spring.datasource.url=jdbc:mysql://localhost:3306/mydb?useSSL=false&serverTimezone=UTC
		spring.datasource.username=root
				server.port=8080

		spring.datasource.driver-class-name=com.mysql.cj.jdbc.Driver
		spring.datasource.hikari.connection-timeout=60000
		spring.datasource.hikari.maximum-pool-size=10
		spring.datasource.hikari.idle-timeout=300000
				server.port=8080

`
		editor := memedit.NewMemEditor(content)
		rngs := GetRangesByText(editor, "server.port")
		require.Equal(t, 3, len(rngs))
		for _, rng := range rngs {
			require.Equal(t, `server.port`, rng.GetText())
		}
	})
}

func TestGetRangeAfterSlimParserTree(t *testing.T) {
	source := "class A { void run(){ int x = 1 + 2; } }"
	lexer := javaparser.NewJavaLexer(antlr.NewInputStream(source))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := javaparser.NewJavaParser(tokenStream)
	cu := parser.CompilationUnit().(*javaparser.CompilationUnitContext)
	method := cu.TypeDeclaration(0).(*javaparser.TypeDeclarationContext).ClassDeclaration().(*javaparser.ClassDeclarationContext).ClassBody().(*javaparser.ClassBodyContext).ClassBodyDeclaration(0).(*javaparser.ClassBodyDeclarationContext).MemberDeclaration().(*javaparser.MemberDeclarationContext).MethodDeclaration().(*javaparser.MethodDeclarationContext)
	body := method.MethodBody().(*javaparser.MethodBodyContext)
	editor := memedit.NewMemEditor(source)

	before := GetRange(editor, body)
	require.NotNil(t, before)
	require.Contains(t, before.GetText(), "int x = 1 + 2")

	DetachAST(body)

	start := body.GetStart()
	require.NotNil(t, start)
	require.Nil(t, start.GetInputStream())
	require.Nil(t, start.GetTokenSource())

	after := GetRange(editor, body)
	require.NotNil(t, after)
	require.Equal(t, before.GetStart().String(), after.GetStart().String())
	require.Equal(t, before.GetEnd().String(), after.GetEnd().String())
	require.Equal(t, before.GetText(), after.GetText())
}

func TestNewTextRangeTokenKeepsRangeWithoutParserTree(t *testing.T) {
	source := "class A extends Base { }"
	lexer := javaparser.NewJavaLexer(antlr.NewInputStream(source))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := javaparser.NewJavaParser(tokenStream)
	cu := parser.CompilationUnit().(*javaparser.CompilationUnitContext)
	classDecl := cu.TypeDeclaration(0).(*javaparser.TypeDeclarationContext).ClassDeclaration().(*javaparser.ClassDeclarationContext)
	baseType := classDecl.TypeType().(*javaparser.TypeTypeContext)
	editor := memedit.NewMemEditor(source)

	token := NewTextRangeToken(baseType)
	require.NotNil(t, token)
	require.Equal(t, "Base", token.GetText())
	_, isTree := any(token).(antlr.Tree)
	require.False(t, isTree, "lightweight range token must not retain the parser tree")
	require.Nil(t, token.GetStart().GetInputStream())
	require.Nil(t, token.GetStart().GetTokenSource())
	require.Nil(t, token.GetStop().GetInputStream())
	require.Nil(t, token.GetStop().GetTokenSource())

	ReleaseASTRoot(cu)

	rng := GetRange(editor, token)
	require.NotNil(t, rng)
	require.Equal(t, "Base", rng.GetText())
}
