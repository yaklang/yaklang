package main

import (
	"bytes"
	"encoding/gob"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

func CheckDocumentHelper(t *testing.T, helper *yakdoc.DocumentHelper) {
	t.Helper()

	checkFuncDecl := func(decl *yakdoc.FuncDecl) {
		t.Helper()
		libName, methodName := decl.LibName, decl.MethodName
		if decl.Decl == "" {
			t.Fatalf("%s.%s check failed: decl empty", libName, methodName)
		}
		if decl.Document == "" {
			t.Fatalf("%s.%s check failed: document empty", libName, methodName)
		}
		if decl.VSCodeSnippets == "" {
			t.Fatalf("%s.%s check failed: vscode snippets empty", libName, methodName)
		}
	}

	checkNormalFunction := func(libName string, methodName string) {
		t.Helper()
		var decl *yakdoc.FuncDecl
		var ok bool

		if libName == "" {
			decl, ok = helper.Functions[methodName]
			if !ok {
				t.Fatalf("%s.%s check failed: function not exist", libName, methodName)
			}
		} else {
			lib, ok := helper.Libs[libName]
			if !ok {
				t.Fatalf("%s.%s check failed: library not exist", libName, methodName)
			}
			decl, ok = lib.Functions[methodName]
			if !ok {
				t.Fatalf("%s.%s check failed: function not exist", libName, methodName)
			}
		}
		checkFuncDecl(decl)
	}

	checkStructMethods := func(structName string, method ...string) {
		lib, ok := helper.StructMethods[structName]
		if !ok {
			t.Fatalf("%s check failed: struct not exist", structName)
		}
		if len(method) == 0 {
			for _, decl := range lib.Functions {
				checkFuncDecl(decl)
			}
		} else {
			for _, methodName := range method {
				decl, ok := lib.Functions[methodName]
				if !ok {
					t.Fatalf("%s.%s check failed: function not exist", structName, methodName)
				}
				checkFuncDecl(decl)
			}
		}
	}

	checkNormalFunction("", "eval")
	checkNormalFunction("", "yakfmt")
	checkStructMethods("github.com/yaklang/yaklang/common/mutate.FuzzHTTPRequestIf", "Repeat")
}

func TestGenerateDoc(t *testing.T) {
	helper := yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	if err := encoder.Encode(&helper); err != nil {
		t.Fatal(err)
	}
	newBuf, err := utils.GzipCompress(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	var newHelper *yakdoc.DocumentHelper
	newBuf2, err := utils.GzipDeCompress(newBuf)
	if err != nil {
		t.Fatal(err)
	}

	decoder := gob.NewDecoder(bytes.NewReader(newBuf2))
	if err := decoder.Decode(&newHelper); err != nil {
		t.Fatalf("load embed yak document error: %v", err)
	}
	CheckDocumentHelper(t, newHelper)
}
