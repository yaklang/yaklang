package antlr4nasl

import (
	"fmt"
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	nasl "github.com/yaklang/yaklang/common/yak/antlr4nasl/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/visitors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func WalkScript(path string, action func(path, script string)) error {
	return filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() || (!strings.HasSuffix(path, ".antlr4nasl") && !strings.HasSuffix(path, ".inc")) {
			return nil
		}
		//这几个脚本有明显语法错误
		if strings.Contains(path, "glsa") {
			return nil
		}
		if path == "/Users/z3/Downloads/mageni-master/src/backend/scanner/plugins/2022/confluence/mageni_atlassian_confluence_cve_2022_26134.antlr4nasl" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			log.Errorf("read file %s failed: %v", path, err)
		}
		action(path, string(data))
		return nil
	})
}
func TestCompileScripts(t *testing.T) {
	engine := New()

	//unknowId := map[string][]string{}
	unknowIdSimple := map[string]string{}
	var currentFie string
	idHook := func(compiler *visitors.Compiler, ctx antlr.ParserRuleContext) {
		if v, ok := ctx.(*nasl.IdentifierExpressionContext); ok {
			text := v.GetText()
			if _, ok := compiler.GetSymbolTable().GetSymbolByVariableName(text); !ok {
				if _, ok := compiler.GetExternalVariablesNamesMap()[text]; !ok {
					if _, ok := unknowIdSimple[text]; !ok {
						unknowIdSimple[text] = currentFie
					}
				}
			}
		}
	}
	_ = idHook
	includeFiles := map[string]struct{}{}
	includeHook := func(compiler *visitors.Compiler, ctx antlr.ParserRuleContext) {
		if v, ok := ctx.(*nasl.CallExpressionContext); ok {
			exp := v.SingleExpression()
			if exp.GetText() == "include" {
				fname := v.ArgumentList().GetText()
				fname = fname[1 : len(fname)-1]
				if !strings.HasSuffix(fname, ".inc") {
					return
				}
				if _, ok := includeFiles[fname]; !ok {
					includeFiles[fname] = struct{}{}
				}
			}
		}
	}
	_ = includeHook
	engine.GetCompiler().AddVisitHook(includeHook)
	total := 0
	paths := []string{}
	_ = paths
	start := true
	err := filepath.Walk("/Users/z3/Downloads/mageni-master/src/backend/scanner/plugins", func(path string, info os.FileInfo, err error) error {
		currentFie = path
		//if strings.HasSuffix(path, ".inc") {
		//	println(path)
		//}
		if info.IsDir() || !strings.HasSuffix(path, ".antlr4nasl") {
			return nil
		}
		//这几个脚本有明显语法错误
		if strings.Contains(path, "glsa") {
			return nil
		}
		if path == "/Users/z3/Downloads/mageni-master/src/backend/scanner/plugins/2022/confluence/mageni_atlassian_confluence_cve_2022_26134.antlr4nasl" {
			return nil
		}

		//if path == "/Users/z3/Downloads/mageni-master/src/backend/scanner/plugins/pre2008/fw1_udp_DoS.antlr4nasl" {
		//	start = true
		//}
		if !start {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			log.Errorf("read file %s failed: %v", path, err)
		}
		engine.Compile(string(data))

		total += 1
		//if total == 50 {
		//	start = false
		//}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("共加载 %d 个脚本\n", total)
	for fname, _ := range includeFiles {
		println(fname)
	}
	//b, err := json.Marshal(unknowIdSimple)
	//if err != nil {
	//	fmt.Println("json.Marshal failed:", err)
	//	return
	//}
	//os.WriteFile("/Users/z3/Downloads/unknowId.json", b, 0644)

	//if len(paths) > 0 {
	//	fmt.Printf("加载出错脚本：\n%s", strings.Join(paths, "\n"))
	//}
}
func TestCompileScript(t *testing.T) {
	path := "/Users/z3/Downloads/mageni-master/src/backend/scanner/plugins/gb_cisco_wsa_web_detect.antlr4nasl"
	enigne := New()
	codeBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	err = enigne.Compile(string(codeBytes))
	if err != nil {
		t.Fatal(err)
	}
	err = enigne.RunFile(path)
	if err != nil {
		log.Error(err)
	}
}
func TestScript(t *testing.T) {
	path := "/Users/z3/Downloads/mageni-master/src/backend/scanner/plugins/gb_cisco_wsa_web_detect.antlr4nasl"
	err := ExecFile(path)
	if err != nil {
		t.Fatal("exec failed")
	}
}
func TestScript1(t *testing.T) {
	path := "/Users/z3/Downloads/mageni-master/src/backend/scanner/plugins/http_func.inc"
	err := ExecFile(path)
	if err != nil {
		t.Fatal("exec failed")
	}
}

func TestSaveNaslScript(t *testing.T) {
	path := "/Users/z3/Downloads/mageni-master/src/backend/scanner/plugins/DDI_FTP_Any_User_Login.antlr4nasl"
	engine := New()
	engine.Init()
	//engine.SetDescription(true)
	engine.SetIncludePath("/Users/z3/Downloads/mageni-master/src/backend/scanner/incs")
	err := engine.RunFile(path)
	if err != nil {
		log.Error(err)
		return
	}
}

func TestLoadScriptInfo(t *testing.T) {
	engine := New()
	engine.Init()
	engine.SetDescription(true)
	engine.SetIncludePath("/Users/z3/Downloads/mageni-master/src/backend/scanner/incs")
	WalkScript("/Users/z3/Downloads/mageni-master/src/backend/scanner/plugins", func(path, script string) {
		err := engine.SafeRunFile(path)
		defer func() {
			if e := recover(); e != nil {
				log.Errorf("load script %s failed: %v", path, e)
				panic(e)
			}
		}()
		if err != nil {
			log.Errorf("run file %s failed: %v", path, err)
			return
		}
	})
}

// 统计所有 script description 相关函数
func TestWalkScriptMethod(t *testing.T) {
	//re1, err := regexp.Compile("(script_.*?)\\(.*")
	re2, err := regexp.Compile("ACT_([A-Z]|_)+")
	if err != nil {
		t.Fatal(err)
	}
	resMap := map[string]string{}
	WalkScript("/Users/z3/Downloads/mageni-master/src/backend/scanner/plugins", func(path, script string) {
		scriptMethods := re2.FindString(script)
		if _, ok := resMap[scriptMethods]; !ok {
			resMap[scriptMethods] = path
		}
		//for _, method := range scriptMethods {
		//if len(method) < 2 {
		//	continue
		//}
		//if _, ok := resMap[method[1]]; !ok {
		//	resMap[method[1]] = path
		//}
		//}
	})
	spew.Dump(resMap)
}
