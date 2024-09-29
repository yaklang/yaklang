package tests

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/javaclassparser/classes"
	"testing"
)

func TestIfElse(t *testing.T) {
	classesContent, err := classes.FS.ReadFile("test/IfTest.class")
	if err != nil {
		t.Fatal(err)
	}
	expectSource, err := classes.FS.ReadFile("test/IfTest.java")
	if err != nil {
		t.Fatal(err)
	}
	cf, err := javaclassparser.Parse(classesContent)
	if err != nil {
		t.Fatal(err)
	}
	source, err := cf.Dump()
	if err != nil {
		t.Fatal(err)
	}
	println(source)
	assert.Equal(t, string(expectSource), source)
}
