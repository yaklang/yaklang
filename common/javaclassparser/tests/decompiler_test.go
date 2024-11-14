package tests

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/javaclassparser/classes"
	"testing"
)

func TestDecompiler(t *testing.T) {
	testCase := []struct {
		name string
	}{
		{
			"TryCatch",
		},
		{
			name: "LogicalOperation",
		},
		{
			name: "TernaryExpressionTest",
		},
		{
			name: "SwitchTest",
		},
	}
	for _, testItem := range testCase {
		t.Run(testItem.name, func(t *testing.T) {
			t.Parallel()
			classRaw, err := classes.FS.ReadFile(testItem.name + ".class")
			if err != nil {
				t.Fatal(err)
			}
			sourceCode, err := classes.FS.ReadFile(testItem.name + ".java")
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
