package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/javaclassparser/classes"
	"gotest.tools/v3/assert"
)

func TestParseAnnotation(t *testing.T) {
	content, err := classes.FS.ReadFile("AnnotationTest.class")
	if err != nil {
		t.Fatal(err)
	}
	parser := javaclassparser.NewClassParser(content)
	class, err := parser.Parse()
	if err != nil {
		t.Fatal(err)
	}
	dumpedClass := class.Bytes()

	assert.Equal(t, dumpedClass, content)
}
