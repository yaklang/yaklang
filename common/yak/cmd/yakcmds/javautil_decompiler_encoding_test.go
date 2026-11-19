package yakcmds

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestJavaDecompilerPreservesNonASCIIStringConstants guards the class-file
// modified-UTF-8 decode path and the source writer used by `yak java-decompiler`.
// The fixture is EncodingProbe.java in testdata/java_encoding, compiled with
// `javac --release 8`; see the source comment for the rebuild command.
func TestJavaDecompilerPreservesNonASCIIStringConstants(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "java_encoding", "EncodingProbe.class"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	target := filepath.Join(t.TempDir(), "EncodingProbe.java")
	if err := classDecompiler(raw, target); err != nil {
		t.Fatalf("classDecompiler: %v", err)
	}
	source, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read decompiled source: %v", err)
	}
	if !utf8.Valid(source) {
		t.Fatal("decompiled source is not valid UTF-8")
	}

	text := string(source)
	for _, want := range []string{
		`CHINESE = "中文字符串常量"`,
		`JAPANESE = "日本語の文字列"`,
		`KOREAN = "한국어 문자열"`,
		`ACCENT = "café naïve © ®"`,
		`EMOJI = "emoji\uD83D\uDE00"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("decompiled source lost %q", want)
		}
	}
	if strings.ContainsRune(text, utf8.RuneError) {
		t.Error("decompiled source contains U+FFFD replacement characters")
	}
}
