package yakcmds

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yakdoc/doc"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"github.com/yaklang/yaklang/common/yakdocument"
	"gopkg.in/yaml.v2"
)

var DocCommands = []*cli.Command{
	{
		Name:  "doc",
		Usage: "Show Help Information for coding, document in YakLang",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "lib,extlib,l,t",
				Usage: "展示特定第三方扩展包的定义和帮助信息",
			},
			cli.StringFlag{
				Name:  "func,f",
				Usage: "展示特定第三方扩展包函数的定义",
			},
			cli.BoolFlag{
				Name:  "all-lib,all-libs,libs",
				Usage: "展示所有第三方包的帮助信息",
			},
		},
		Action: func(c *cli.Context) error {
			helper := doc.GetDefaultDocumentHelper()

			if c.Bool("all-lib") {
				for _, libName := range helper.GetAllLibs() {
					helper.ShowLibHelpInfo(libName)
				}
				return nil
			}

			extLib := c.String("extlib")
			function := c.String("func")
			if extLib == "" && function != "" {
				extLib = "__GLOBAL__"
			}

			if extLib == "" {
				helper.ShowHelpInfo()
				return nil
			}

			if function != "" {
				if info := helper.LibFuncHelpInfo(extLib, function); info == "" {
					log.Errorf("palm script engine no such function in %s: %v", extLib, function)
					return nil
				} else {
					helper.ShowLibFuncHelpInfo(extLib, function)
				}
			} else {
				if info := helper.LibHelpInfo(extLib); info == "" {
					log.Errorf("palm script engine no such extlib: %v", extLib)
					return nil
				} else {
					helper.ShowLibHelpInfo(extLib)
				}
			}

			return nil
		},
	},
	// gendoc / build doc
	{
		Name:  "gendoc",
		Usage: "Generate Basic Yaml Structure for YakLang",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "dir",
				Usage: "生成的文档路径",
				Value: "docs",
			},
		},
		Action: func(c *cli.Context) error {
			libs := yak.EngineToLibDocuments(yaklang.New())
			baseDir := filepath.Join(".", c.String("dir"))

			_ = os.MkdirAll(baseDir, 0o777)
			for _, lib := range libs {
				targetFile := filepath.Join(baseDir, fmt.Sprintf("%v.yakdoc.yaml", lib.Name))
				existed := yakdocument.LibDoc{}
				if utils.GetFirstExistedPath(targetFile) != "" {
					raw, _ := ioutil.ReadFile(targetFile)
					_ = yaml.Unmarshal(raw, &existed)
				}

				lib.Merge(&existed)
				raw, _ := yaml.Marshal(lib)
				_ = ioutil.WriteFile(targetFile, raw, os.ModePerm)
			}

			for _, s := range yakdocument.LibsToRelativeStructs(libs...) {
				targetFile := filepath.Join(baseDir, "structs", fmt.Sprintf("%v.struct.yakdoc.yaml", s.StructName))
				dir, _ := filepath.Split(targetFile)
				_ = os.MkdirAll(dir, 0o777)
				existed := yakdocument.StructDocForYamlMarshal{}
				if utils.GetFirstExistedPath(targetFile) != "" {
					raw, err := ioutil.ReadFile(targetFile)
					if err != nil {
						log.Errorf("cannot find file[%s]: %s", targetFile, err)
						continue
					}
					err = yaml.Unmarshal(raw, &existed)
					if err != nil {
						log.Errorf("unmarshal[%s] failed: %s", targetFile, err)
					}
				}

				if existed.StructName != "" {
					s.Merge(&existed)
				}
				raw, _ := yaml.Marshal(s)
				_ = ioutil.WriteFile(targetFile, raw, os.ModePerm)
			}
			return nil
		},
	},
	{
		Name:  "builddoc",
		Usage: "Build Markdown Documents for YakLang(From Structured Yaml Text)",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "from",
				Usage: "生成的文档源文件路径",
				Value: "docs",
			},
			cli.StringFlag{
				Name:  "to",
				Usage: "生成 Markdown 内容",
				Value: "build/yakapis/",
			},
			cli.StringFlag{
				Name:  "to-vscode-data,tovd",
				Value: "build/yaklang-completion.json",
			},
		},
		Action: func(c *cli.Context) error {
			libs := yak.EngineToLibDocuments(yaklang.New())
			baseDir := filepath.Join(".", c.String("from"))

			outputDir := filepath.Join(".", c.String("to"))
			_ = os.MkdirAll(outputDir, os.ModePerm)

			_ = os.MkdirAll(baseDir, os.ModePerm)
			for _, lib := range libs {
				targetFile := filepath.Join(baseDir, fmt.Sprintf("%v.yakdoc.yaml", lib.Name))
				existed := yakdocument.LibDoc{}
				if utils.GetFirstExistedPath(targetFile) != "" {
					raw, _ := ioutil.ReadFile(targetFile)
					_ = yaml.Unmarshal(raw, &existed)
				}

				lib.Merge(&existed)

				outputFileName := filepath.Join(outputDir, fmt.Sprintf("%v.md", strings.ReplaceAll(lib.Name, ".", "_")))
				_ = outputFileName

				results := lib.ToMarkdown()
				if results == "" {
					return utils.Errorf("markdown empty... for %v", lib.Name)
				}
				err := ioutil.WriteFile(outputFileName, []byte(results), os.ModePerm)
				if err != nil {
					return err
				}
			}

			completionJsonRaw, err := yakdocument.LibDocsToCompletionJson(libs...)
			if err != nil {
				return err
			}
			err = ioutil.WriteFile(c.String("to-vscode-data"), completionJsonRaw, os.ModePerm)
			if err != nil {
				return utils.Errorf("write vscode auto-completions json failed: %s", err)
			}
			return nil
		},
	},
}
