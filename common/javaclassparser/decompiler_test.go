package javaclassparser

import (
	"github.com/yaklang/yaklang/common/javaclassparser/classes"
	"testing"
)

func TestDecompiler(t *testing.T) {
	classesContent, err := classes.FS.ReadFile("RuntimeExec.class")
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
