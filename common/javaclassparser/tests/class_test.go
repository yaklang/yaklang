package tests

import (
	"github.com/yaklang/yaklang/common/javaclassparser"
	"os"
	"testing"
)

type A struct {
	a []int
}

func TestDecompilerClass(t *testing.T) {
	classesContent, _ := os.ReadFile("/Users/z3/Downloads/cfr-master/target/classes/org/benf/cfr/reader/entityfactories/ContiguousEntityFactory.class")
	cf, err := javaclassparser.Parse(classesContent)
	if err != nil {
		t.Fatal(err)
	}
	source, err := cf.Dump()
	if err != nil {
		t.Fatal(err)
	}
	println(source)
}
