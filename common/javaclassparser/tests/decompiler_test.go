package tests

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/javaclassparser/classes"
	"testing"
)

func TestDecompiler(t *testing.T) {
	testCase := []string{
		"SwitchTest",
	}
	for _, s := range testCase {
		t.Run(s, func(t *testing.T) {
			t.Parallel()
			classRaw, err := classes.FS.ReadFile(s + ".class")
			if err != nil {
				t.Fatal(err)
			}
			sourceCode, err := classes.FS.ReadFile(s + ".java")
			if err != nil {
				t.Fatal(err)
			}
			ins, err := javaclassparser.Parse(classRaw)
			if err != nil {
				t.Fatal(err)
			}
			source, err := ins.Dump()
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, string(sourceCode), source)
		})
	}

}
