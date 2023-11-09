package main

import (
	"fmt"
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

func GenerateSingleFile(basepath string, lib *yakdoc.ScriptLib) {
	file, err := os.Create(path.Join(basepath, lib.Name+".md"))
	if err != nil {
		log.Errorf("create file error: %v", err)
	}
	defer file.Close()
	file.WriteString("# " + lib.Name + "\n\n")
	file.WriteString("|成员函数|函数描述/介绍|\n")
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
			doc = strings.ReplaceAll(doc, "<", "&lt;")
			doc = strings.ReplaceAll(doc, ">", "&gt;")
			document = doc + document[exampleIndex:]
		} else {
			document = strings.ReplaceAll(document, "<", "&lt;")
			document = strings.ReplaceAll(document, ">", "&gt;")
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

		exampleIndex = strings.Index(document, "Example:")
		if exampleIndex != -1 {
			document = strings.ReplaceAll(document[:exampleIndex], "\n", "\n\n") + document[exampleIndex:]
		}
		lowerMethodName := strings.ToLower(fun.MethodName)
		file.WriteString(fmt.Sprintf("| [%s.%s](#%s) |%s|\n", fun.LibName, fun.MethodName, lowerMethodName, simpleDocument))
		buf := strings.Builder{}
		buf.WriteString(fmt.Sprintf("### %s\n\n", fun.MethodName))
		buf.WriteString(fmt.Sprintf("#### 详细描述\n%s\n\n", document))
		buf.WriteString(fmt.Sprintf("#### 定义\n\n`%s`\n\n", fun.Decl))
		if len(fun.Params) > 0 {
			buf.WriteString("#### 参数\n")
			buf.WriteString("|参数名|参数类型|参数解释|\n")
			buf.WriteString("|:-----------|:---------- |:-----------|\n")
			for _, param := range fun.Params {
				buf.WriteString(fmt.Sprintf("| %s | `%s` |   |\n", param.Name, param.Type))
			}
			buf.WriteString("\n")
		}
		if len(fun.Results) > 0 {
			buf.WriteString("#### 返回值\n")
			buf.WriteString("|返回值(顺序)|返回值类型|返回值解释|\n")
			buf.WriteString("|:-----------|:---------- |:-----------|\n")
			for _, result := range fun.Results {
				buf.WriteString(fmt.Sprintf("| %s | `%s` |   |\n", result.Name, result.Type))
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

	helper := yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())
	for _, lib := range helper.Libs {
		GenerateSingleFile(basepath, lib)
	}
}
