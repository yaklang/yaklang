package tests

import (
	"embed"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"path/filepath"
	"strings"
	"testing"
)

//go:embed code
var codeFs embed.FS

func validateSource(t *testing.T, filename string, src string) {
	t.Run(fmt.Sprintf("syntax file: %v", filename), func(t *testing.T) {
		_, err := java2ssa.Frontend(src, false)
		require.Nil(t, err, "parse AST FrontEnd error : %v", err)
	})
}

func TestAllSyntaxForJava_G4(t *testing.T) {
	entry, err := codeFs.ReadDir("code")
	if err != nil {
		t.Fatalf("no embed syntax files found: %v", err)
	}
	for _, f := range entry {
		if f.IsDir() {
			continue
		}
		path := filepath.Join("syntax", f.Name())
		if !strings.HasSuffix(path, ".php") {
			continue
		}
		raw, err := codeFs.ReadFile(path)
		if err != nil {
			t.Fatalf("cannot found syntax fs: %v", path)
		}
		validateSource(t, path, string(raw))
	}
}

//func TestJavaBasicParse_MockFile(t *testing.T) {
//	entry, err := codeFs.ReadDir("code")
//	if err != nil {
//		t.Fatalf("no embed syntax files found: %v", err)
//	}
//	for _, f := range entry {
//		if f.IsDir() {
//			continue
//		}
//		path := path.Join("code", f.Name())
//		if !strings.HasSuffix(path, ".java") {
//			continue
//		}
//		raw, err := codeFs.ReadFile(path)
//		if err != nil {
//			t.Fatalf("cannot found syntax fs: %v", path)
//		}
//		t.Run(path, func(t *testing.T) {
//			//test.MockSSA(t, string(raw))
//
//		})
//	}
//}
