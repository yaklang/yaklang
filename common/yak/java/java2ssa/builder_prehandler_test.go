package java2ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
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
