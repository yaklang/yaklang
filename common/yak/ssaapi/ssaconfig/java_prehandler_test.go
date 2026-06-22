package ssaconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchJavaPreHandlerFile(t *testing.T) {
	for _, path := range []string{
		"src/main/java/App.java",
		"B.class",
		"src/main/resources/application.properties",
		"pom.xml",
		"module/pom.xml",
	} {
		require.True(t, MatchJavaPreHandlerFile(path), path)
	}

	for _, path := range []string{
		"README.md",
		"scripts/runAcceptanceTests.sh",
		"target/classes/com/example/App.class",
		".gitignore",
	} {
		require.False(t, MatchJavaPreHandlerFile(path), path)
	}
}

func TestDefaultCompileExcludeJavaDirs(t *testing.T) {
	exclude := BuildCompileExcludeFunc(nil, "")
	for _, path := range []string{
		".github/workflows/maven.yml",
		".mvn/jvm.config",
		"docs/modules/ROOT/pages/index.adoc",
		"eclipse/org.eclipse.jdt.ui.prefs",
	} {
		require.True(t, exclude(path), path)
	}
}
