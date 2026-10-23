package java2ssa

import (
	"os"
	"path/filepath"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestFrontendRawRejectsMalformedDecompilerOutput(t *testing.T) {
	cache := CreateBuilder().(*SSABuilder).GetAntlrCache()
	defer cache.Clear()
	for _, src := range []string{
		`class Sample { void run((Class)(Class var1)) {} }`,
		`class Sample { void run() { try {} catch (Exception e) {} else {} } }`,
		`class Sample { void run() { call("unterminated); } }`,
	} {
		if _, err := FrontendRaw(src, cache); err == nil {
			t.Fatalf("raw parser accepted malformed producer output: %s", src)
		}
	}
	if _, err := FrontendRaw(`class Sample { void run(Class<?> type) { try { call(type); } catch (Exception e) { if (type != null) throw e; else call(null); } } }`, cache); err != nil {
		t.Fatal(err)
	}
}

// These shapes are illegal for javac and legal for the Java grammar. They are
// the retained forms of the coremail classes that used to be failure comments:
// a handwritten enum valueOf, a non-javac enum rendered as a class, a blank
// final with no constructor, and a qualified static call whose package root is
// also a type name.
func TestFrontendRawAcceptsRetainedDecompilerShapes(t *testing.T) {
	cache := CreateBuilder().(*SSABuilder).GetAntlrCache()
	defer cache.Clear()
	for _, src := range []string{
		"enum Messages { ILLEGAL_PARAMETER, MISSING_ID; public static Messages valueOf(String name) { if (name == null) throw new IllegalArgumentException(name); return ILLEGAL_PARAMETER; } }",
		"public final class NestingType extends Enum<NestingType> { public static final NestingType NA; public NestingType next() { return NA; } }",
		"final class Comparators$1 implements Comparator { final ToIntFunction val$t; public int compare(Object o1, Object o2) { return val$t.applyAsInt(o1); } }",
		"package a; class cn {} class ct { int b() { return cn.mailtech.utils.g.a(); } }",
	} {
		ast, err := FrontendRaw(src, cache)
		if err != nil || ast == nil || len(ast.AllTypeDeclaration()) == 0 {
			t.Fatalf("retained source was not parsed: %v\n%s", err, src)
		}
	}
}

func TestJavaPrepareRetainsUnprovedSource(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Sample.class"), []byte{0xca, 0xfe, 0xba, 0xbe}, 0600); err != nil {
		t.Fatal(err)
	}
	local := filesys.NewRelLocalFs(dir)
	wrapped := CreateBuilder().(*SSABuilder).WrapWithPreprocessedFS(local, true)
	unified, ok := wrapped.(*filesys.UnifiedFS)
	if !ok {
		t.Fatalf("wrapper = %T", wrapped)
	}
	expanded, ok := unified.GetFileSystem().(*javaclassparser.ExpandedZipFS)
	if !ok || !expanded.RetainsUnprovedSource() {
		t.Fatalf("analysis output did not keep unproved members: %T", unified.GetFileSystem())
	}
}
