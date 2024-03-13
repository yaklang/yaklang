package scripts

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yserx"
	"github.com/yaklang/yaklang/common/yso"
	"os"
	"strings"
	"testing"
)

func TestGenConst(t *testing.T) {
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
func TestCompress(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	fileInfos, err := utils.ReadDir("/tmp/ser-file")
	if err != nil {
		t.Fatal(err)
	}
	writeDataBlock := func(content []byte) {
		l := yserx.IntTo4Bytes(len(content))
		_, err := zw.Write(l)
		if err != nil {
			t.Fatal(err)
		}
		_, err = zw.Write(content)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, info := range fileInfos {
		if info.IsDir {
			continue
		}
		if !strings.HasSuffix(info.Name, ".ser") {
			continue
		}
		content, err := os.ReadFile("/tmp/ser-file/" + info.Name)
		if err != nil {
			t.Fatal(err)
		}
		writeDataBlock([]byte(info.Name))
		writeDataBlock(content)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile("/tmp/gadgets.bin", buf.Bytes(), 0777)
	if err != nil {
		t.Fatal(err)
	}
}
