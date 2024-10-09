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
	classesContent, _ := os.ReadFile("/Users/z3/Downloads/cfr-master/src/org/benf/cfr/reader/LoopTest.class")
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
