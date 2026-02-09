package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yserx"
	"github.com/yaklang/yaklang/common/yso"
	"os"
	"path"
	"strings"
)

func main() {
	app := cli.NewApp()
	app.Name = "code_generator"
	app.Description = "用于生成配置文件中的常量，压缩payload"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "compress,c",
			Usage: "压缩文件",
		},
		cli.StringFlag{
			Name:  "output,o",
			Usage: "保存位置",
		},
	}
	app.Action = func(c *cli.Context) {
		if c.String("compress") != "" {
			compress(c.String("compress"), c.String("output"))
		} else {
			genConst()
		}
	}
}
func genConst() {
	case2Camel := func(name string) string {
		name = strings.Replace(name, "-", " ", -1)
		name = strings.Title(name)
		return strings.Replace(name, " ", "", -1)
	}

	allParams := make(map[yso.ClassParamType]struct{})
	allClassNames := make(map[yso.ClassType]struct{})
	for className, config := range yso.YsoConfigInstance.Classes {
		allClassNames[className] = struct{}{}
		for _, param := range config.Params {
			allParams[param.Name] = struct{}{}
		}
	}
	allGadgetName := make(map[string]struct{})
	for s, config := range yso.YsoConfigInstance.Gadgets {
		_ = s
		allGadgetName[config.Name] = struct{}{}
	}
	code := "package yso\n\n"
	newConstantsCode := func(typ string, kvs map[string]string) string {
		tmp := `const (
%s)`
		constsCode := ""
		for k, v := range kvs {
			constsCode += fmt.Sprintf("\t%s %s = \"%s\"\n", k, typ, v)
		}
		return fmt.Sprintf("type %s string\n", typ) + fmt.Sprintf(tmp, constsCode)
	}
	allClassNamesKV := make(map[string]string)
	for name, _ := range allClassNames {
		allClassNamesKV["Class"+case2Camel(string(name))] = string(name)
	}
	code += newConstantsCode("ClassType", allClassNamesKV) + "\n"
	allParamsKV := make(map[string]string)
	for name, _ := range allParams {
		Kname := case2Camel(string(name))
		allParamsKV["ClassParam"+Kname] = string(name)
	}
	code += newConstantsCode("ClassParamType", allParamsKV) + "\n"

	allGadgetNameKV := make(map[string]string)
	for name, _ := range allGadgetName {
		allGadgetNameKV["Gadget"+case2Camel(string(name))] = name
	}
	code += newConstantsCode("GadgetType", allGadgetNameKV)
	println(code)
}
func compress(src string, out string) {
	if out == "" {
		out = "gadgets.bin"
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	fileInfos, err := utils.ReadDir(src)
	if err != nil {
		log.Errorf("read dir %s failed: %v", src, err)
		return
	}
	writeDataBlock := func(content []byte) {
		l := yserx.IntTo4Bytes(len(content))
		_, err := zw.Write(l)
		if err != nil {
			log.Errorf("write len failed: %v", err)
			return
		}
		_, err = zw.Write(content)
		if err != nil {
			log.Errorf("write content failed: %v", err)
			return
		}
	}
	for _, info := range fileInfos {
		if info.IsDir {
			continue
		}
		if !strings.HasSuffix(info.Name, ".ser") {
			continue
		}
		content, err := os.ReadFile(path.Join(src, info.Name))
		if err != nil {
			log.Errorf("read file %s failed: %v", info.Name, err)
			return
		}
		writeDataBlock([]byte(info.Name))
		writeDataBlock(content)
	}
	if err := zw.Close(); err != nil {
		log.Errorf("gzip close failed: %v", err)
		return
	}
	err = os.WriteFile(out, buf.Bytes(), 0777)
	if err != nil {
		log.Errorf("write file failed: %v", err)
	}
}
