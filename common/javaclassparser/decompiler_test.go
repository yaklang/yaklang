package javaclassparser

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/classes"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler"
	"os"
	"strings"
	"testing"
)

func TestAddSupperInterface(t *testing.T) {
	classesContent, _ := os.ReadFile("/Users/z3/Downloads/cfr-master/src/org/benf/cfr/reader/Demo.class")
	cf, err := Parse(classesContent)
	if err != nil {
		t.Fatal(err)
	}
	cf.ConstantPool = append(cf.ConstantPool)
}
func TestDecompiler(t *testing.T) {
	//classesContent, err := classes.FS.ReadFile("Demo.class")
	classesContent, err := os.ReadFile("/Users/z3/Downloads/cfr-master/src/org/benf/cfr/reader/Demo.class")
	if err != nil {
		t.Fatal(err)
	}
	cf, err := Parse(classesContent)
	if err != nil {
		t.Fatal(err)
	}
	source, err := cf.Dump()
	if err != nil {
		t.Fatal(err)
	}
	println(source)
}
func TestModifyOpcode(t *testing.T) {
	classesContent, err := classes.FS.ReadFile("Demo.class")
	if err != nil {
		t.Fatal(err)
	}
	cf, err := Parse(classesContent)
	if err != nil {
		t.Fatal(err)
	}
	codeAttr := cf.Methods[1].Attributes[0].(*CodeAttribute)
	ParseBytesCode(nil, codeAttr)
}
func TestParseRawType(t *testing.T) {
	content, _ := classes.FS.ReadFile("raw_type.json")
	data := []*decompiler.RawJavaType{}
	json.Unmarshal(content, &data)
	items := []string{}
	for _, datum := range data {
		items = append(items, fmt.Sprintf(`RT_%s = NewRawJavaType("%v","%v",%v,%v,"%v",%v,%v,%v,%v)`,
			strings.ToUpper(datum.Name), datum.Name, datum.SuggestedVarName, "ST_"+strings.ToUpper(datum.StackType.Name),
			datum.UsableType, datum.BoxedName, datum.IsNumber, datum.IsObject, datum.IntMin, datum.IntMax))
	}
	println(strings.Join(items, "\n"))
}

func TestParseStackType(t *testing.T) {
	content, _ := classes.FS.ReadFile("stack_type.json")
	data := []*decompiler.StackType{}
	json.Unmarshal(content, &data)
	items := []string{}
	for _, datum := range data {
		items = append(items, fmt.Sprintf(`ST_%s = NewStackType(%v,%v,"%v")`, strings.ToUpper(datum.Name), datum.ComputationCategory, datum.Closed, datum.Name))
	}
	println(strings.Join(items, "\n"))
}
