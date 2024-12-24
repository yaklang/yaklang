package tests

import (
	_ "embed"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

//go:embed interfaceExtends.class
var interfaceExtends []byte

//go:embed enum.class
var enumClass []byte

func TestEnumBasic(t *testing.T) {
	results, err := javaclassparser.Decompile(enumClass)
	if err != nil {
		t.Fatal(err)
	}
	checkJavaCode(t, results)
	fmt.Println(results)
	assert.Contains(t, results, "enum Node$Type")
	assert.Contains(t, results, "\tLITERAL,\n\rVARIABLE;\n")
}

func TestInterfaceExtends(t *testing.T) {
	results, err := javaclassparser.Decompile(interfaceExtends)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(results)
	assert.Contains(t, results, "NavigableSet extends SortedSet")
}

// checkjavacode
func checkJavaCode(t *testing.T, path string) {
	ssatest.CheckJava(t, path, func(prog *ssaapi.Program) error {
		prog.Show()
		return nil
	})
}
