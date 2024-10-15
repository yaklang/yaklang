package tests

import (
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/javaclassparser/classes"
	"testing"
)

func TestLoop(t *testing.T) {
	classesContent, err := classes.FS.ReadFile("test/LoopTest.class")
	if err != nil {
		t.Fatal(err)
	}
	cf, err := javaclassparser.Parse(classesContent)
	if err != nil {
		t.Fatal(err)
	}
	sourceCode, err := cf.Dump()
	if err != nil {
		t.Fatal(err)
	}
	println(sourceCode)
}
