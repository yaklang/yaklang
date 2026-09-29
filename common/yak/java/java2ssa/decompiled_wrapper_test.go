package java2ssa

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
	javaclassparser "github.com/yaklang/javajive/classparser"
)

// Gradle wrapper fixture from spring-petclinic: local-variable slot reuse
// previously produced `new WrapperExecutor(...) = new PathAssembler(...)`.
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
