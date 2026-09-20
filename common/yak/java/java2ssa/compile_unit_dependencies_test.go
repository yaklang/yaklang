package java2ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJavaQualifiedReferencesIgnoreLiteralsAndComments(t *testing.T) {
	refs := javaQualifiedReferences(`package app;
import dep.api.*;
class C extends base.Parent {
  nested.pkg.Type x;
  String text = "not.a.Dependency";
  // missing.one.Type x;
  /* missing.two.Type x; */
}`)
	require.ElementsMatch(t, []string{"dep.api", "base.Parent", "nested.pkg.Type"}, refs)
}

func TestResolveJavaImportUsesLongestPackage(t *testing.T) {
	packages := map[string]string{"a": "java:a", "a.b": "java:a.b", "a.bc": "java:a.bc"}
	require.Equal(t, "java:a.b", resolveJavaImport(packages, "a.b.Outer.Inner.method"))
	require.Equal(t, "java:a.bc", resolveJavaImport(packages, "a.bc"))
	require.Empty(t, resolveJavaImport(packages, "abc.Type"))
}
