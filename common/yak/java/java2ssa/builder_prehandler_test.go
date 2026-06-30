package java2ssa

import (
	"testing"

	"github.com/yaklang/antlr/v4"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestSSABuilder_FilterPreHandlerFile(t *testing.T) {
	builder := &SSABuilder{}

	for _, path := range []string{
		"src/main/java/App.java",
		"B.class",
		"src/main/resources/application.properties",
		"src/main/resources/application.yml",
		"src/main/resources/application.yaml",
		"src/main/resources/bootstrap.json",
		"src/main/resources/mapper/UserMapper.xml",
		"src/main/webapp/index.jsp",
		"src/main/webapp/index.jspx",
		"src/main/resources/templates/index.ftl",
		"pom.xml",
		"module/pom.xml",
	} {
		require.True(t, builder.FilterPreHandlerFile(path), path)
	}

	for _, path := range []string{
		"README.md",
		"docs/modules/ROOT/pages/index.adoc",
		"scripts/runAcceptanceTests.sh",
		"mvnw.cmd",
		"target/classes/com/example/App.class",
		"WEB-INF/classes/com/example/App.class",
		".gitignore",
		".mvn/jvm.config",
		"eclipse/org.eclipse.jdt.ui.prefs",
		".github/workflows/maven.yml",
	} {
		require.False(t, builder.FilterPreHandlerFile(path), path)
	}
}

func TestSSABuilder_PreHandlerProjectReleasesGeneratedTemplateASTRoot(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		source string
	}{
		{
			name:   "jsp",
			path:   "src/main/webapp/index.jsp",
			source: `<%= request.getParameter("name") %>`,
		},
		{
			name:   "freemarker",
			path:   "src/main/resources/templates/index.ftl",
			source: `<#if enabled>ok</#if>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := CreateBuilder().(*SSABuilder)
			prog := ssa.NewTmpProgram("template-release-test")
			fb := prog.GetAndCreateFunctionBuilder("", string(ssa.MainFunctionName))
			fs := filesys.NewVirtualFs()

			var directChildren []antlr.Tree
			prog.Build = func(ast ssa.FrontAST, _ *memedit.MemEditor, _ *ssa.FunctionBuilder) error {
				tree, ok := ast.(antlr.Tree)
				require.True(t, ok, "generated Java AST should be an ANTLR tree")
				directChildren = append([]antlr.Tree(nil), tree.GetChildren()...)
				require.NotEmpty(t, directChildren, "precondition: generated Java root should have children during build")
				for _, child := range directChildren {
					require.Equal(t, tree, child.GetParent(), "precondition: direct child should still point to the root during build")
				}
				return nil
			}

			editor := prog.CreateEditor([]byte(tt.source), tt.path, false)
			require.NoError(t, builder.PreHandlerProject(fs, nil, fb, editor))
			require.NotEmpty(t, directChildren)
			for _, child := range directChildren {
				require.Nil(t, child.GetParent(), "generated Java root direct child should be detached after build")
			}
		})
	}
}
