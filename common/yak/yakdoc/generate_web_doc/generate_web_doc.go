package main

import (
	"fmt"
	"html"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

func specialPatchValueStr(ins *yakdoc.LibInstance) string {
	valueStr := ins.ValueStr
	if ins.LibName == "os" && ins.InstanceName == "Args" {
		valueStr = "Command line arguments"
	}
	return html.EscapeString(valueStr)
}

func CheckDocCodeBlockMatched() {
	helper := yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())
	failCount := 0
	checkFunc := func(f *yakdoc.FuncDecl) {
		if len(f.Document) == 0 {
			return
		}
		if count := strings.Count(f.Document, "```"); count%2 != 0 {
			failCount++
			fmt.Printf("%s.%s code block not matched\n", f.LibName, f.MethodName)
		}
	}

	for _, lib := range helper.Libs {
		for _, f := range lib.Functions {
			checkFunc(f)
		}
	}

	for _, f := range helper.Functions {
		checkFunc(f)
	}

	for _, lib := range helper.StructMethods {
		for _, f := range lib.Functions {
			checkFunc(f)
		}
	}

	if failCount > 0 {
		panic("code block check not passed")
	}
}

func GenerateSingleFile(basepath string, lib *yakdoc.ScriptLib) {
	file, err := os.Create(path.Join(basepath, lib.Name+".md"))
	if err != nil {
		log.Errorf("create file error: %v", err)
	}
	defer file.Close()
	file.WriteString("# " + lib.Name + "\n\n")
	if len(lib.Instances) > 0 {
		file.WriteString("|实例名|实例描述|\n")
		file.WriteString("|:------|:--------|\n")
		keys := lo.Keys(lib.Instances)
		sort.Strings(keys)
		for _, key := range keys {
			ins := lib.Instances[key]
			file.WriteString(fmt.Sprintf("%s|(%s) %s|\n",
				html.EscapeString(ins.InstanceName),
				html.EscapeString(ins.Type),
				specialPatchValueStr(ins),
			))
		}
		file.WriteString("\n")
	}

	file.WriteString("|函数名|函数描述/介绍|\n")
	file.WriteString("|:------|:--------|\n")

	// 将Functions转成list
	funcList := lo.MapToSlice(lib.Functions, func(key string, value *yakdoc.FuncDecl) *yakdoc.FuncDecl {
		return value
	})
	sort.SliceStable(funcList, func(i, j int) bool {
		return funcList[i].MethodName < funcList[j].MethodName
	})
	bufList := make([]strings.Builder, 0, len(funcList))

	for _, fun := range funcList {
		document := fun.Document
		exampleIndex := strings.Index(document, "Example:")
		if exampleIndex != -1 {
			// Example 代码块不应该替换<和>
			doc := document[:exampleIndex]
			doc = html.EscapeString(doc)
			document = doc + document[exampleIndex:]
		} else {
			document = html.EscapeString(document)
		}

		// 简略的描述，去除\r，替换\n，删除Example:后面的内容，转义|，截取150个字符
		simpleDocument := document
		simpleDocument = strings.ReplaceAll(simpleDocument, "\r", "")
		simpleDocument = strings.ReplaceAll(simpleDocument, "\n", " ")
		exampleIndex = strings.Index(simpleDocument, "Example:")
		if exampleIndex != -1 {
			simpleDocument = simpleDocument[:exampleIndex]
		}
		ellipsisRunes := []rune(simpleDocument)
		if len(ellipsisRunes) > 150 {
			simpleDocument = fmt.Sprintf("%s...", string(ellipsisRunes[:150]))
			simpleDocument = strings.ReplaceAll(simpleDocument, "|", "\\|")
		}

		// exampleIndex = strings.Index(document, "Example:")
		// if exampleIndex != -1 {
		// 	document = strings.ReplaceAll(document[:exampleIndex], "\n", "\n\n") + document[exampleIndex:]
		// }
		lowerMethodName := strings.ToLower(fun.MethodName)
		file.WriteString(fmt.Sprintf("| [%s.%s](#%s) |%s|\n",
			html.EscapeString(fun.LibName),
			html.EscapeString(fun.MethodName),
			html.EscapeString(lowerMethodName),
			html.EscapeString(simpleDocument),
		))
		buf := strings.Builder{}
		buf.WriteString(fmt.Sprintf("### %s\n\n", html.EscapeString(fun.MethodName)))
		buf.WriteString(fmt.Sprintf("#### 详细描述\n%s\n\n", html.EscapeString(document)))
		buf.WriteString(fmt.Sprintf("#### 定义\n\n`%s`\n\n", html.EscapeString(fun.Decl)))
		if len(fun.Params) > 0 {
			buf.WriteString("#### 参数\n")
			buf.WriteString("|参数名|参数类型|参数解释|\n")
			buf.WriteString("|:-----------|:---------- |:-----------|\n")
			for _, param := range fun.Params {
				buf.WriteString(fmt.Sprintf("| %s | `%s` |   |\n", html.EscapeString(param.Name), html.EscapeString(param.Type)))
			}
			buf.WriteString("\n")
		}
		if len(fun.Results) > 0 {
			buf.WriteString("#### 返回值\n")
			buf.WriteString("|返回值(顺序)|返回值类型|返回值解释|\n")
			buf.WriteString("|:-----------|:---------- |:-----------|\n")
			for _, result := range fun.Results {
				buf.WriteString(fmt.Sprintf("| %s | `%s` |   |\n", html.EscapeString(result.Name), html.EscapeString(result.Type)))
			}
			buf.WriteString("\n")
		}
		buf.WriteString("\n")
		bufList = append(bufList, buf)
	}
	file.WriteString("\n\n")
	file.WriteString("## 函数定义\n")
	for _, buf := range bufList {
		file.WriteString(buf.String())
	}
}

func main() {
	if len(os.Args) < 2 {
		return
	}
	basepath := os.Args[1]
	if !utils.IsDir(basepath) {
		if err := os.MkdirAll(basepath, 0o777); err != nil {
			log.Errorf("create dir error: %v", err)
			return
		}
	}

	CheckDocCodeBlockMatched()

	helper := yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())
	for _, lib := range helper.Libs {
		GenerateSingleFile(basepath, lib)
	}
}
