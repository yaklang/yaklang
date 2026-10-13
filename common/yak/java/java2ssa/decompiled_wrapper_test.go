package java2ssa

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
	javaclassparser "github.com/yaklang/javajive/classparser"
)

// Gradle wrapper fixture from spring-petclinic:
// org/gradle/wrapper/GradleWrapperMain.class, taken from the Gradle wrapper JAR
// shipped in the spring-petclinic checkout used for code-scan validation.
// SHA-256: f02e3739c319aa17d83581478d4904d05a67b4cdef76a30f12c80cd12fe55d83.
// Its Apache-2.0 license is kept beside it as
// testdata/GradleWrapper-LICENSE.txt, which Apache-2.0 section 4 requires when
// redistributing the class; the rest of testdata stays Java sources.
//
// The optimized class reuses reference parameter slots for unrelated locals, so
// before javajive PR #10 lambda capture substitution produced invalid assignment
// targets such as `new WrapperExecutor(...) = new PathAssembler(...)`. This test
// guards strict Yaklang AST parsing; it does not assert full Java recompilation
// or behavioral equivalence of the decompiled wrapper.
//
//go:embed testdata/GradleWrapperMain.class
var gradleWrapperMainClass []byte

func TestDecompiledGradleWrapperParses(t *testing.T) {
	class, err := javaclassparser.Parse(gradleWrapperMainClass)
	require.NoError(t, err)
	source, err := class.Dump()
	require.NoError(t, err)
	_, err = Frontend(source)
	require.NoError(t, err, "decompiler output must remain valid Java")
	require.Contains(t, source, "WrapperExecutor")
	require.Contains(t, source, "PathAssembler")
}
