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

//go:embed strconv.class
var strconvClass []byte

//go:embed badstrconv.class
var badstrconvClass []byte

func TestStrconv2(t *testing.T) {
	results, err := javaclassparser.Decompile(badstrconvClass)
	if err != nil {
		t.Fatal(err)
	}
	checkJavaCode(t, results)
	assert.Contains(t, results, `ement [" + this.tag + "] near line " + Action.getLineNumber(this.intercon));`)
}

func TestStrconv(t *testing.T) {
	results, err := javaclassparser.Decompile(strconvClass)
	if err != nil {
		t.Fatal(err)
	}
	checkJavaCode(t, results)
	assert.Contains(t, results, `he value \"\" is not a legal value for attribute \""`)
}

func TestEnumBasic(t *testing.T) {
	results, err := javaclassparser.Decompile(enumClass)
	if err != nil {
		t.Fatal(err)
	}
	checkJavaCode(t, results)
	fmt.Println(results)
	assert.Contains(t, results, "enum Node$Type")
	assert.Contains(t, results, "\tLITERAL,\n\tVARIABLE;\n")
}

func TestInterfaceExtends(t *testing.T) {
	results, err := javaclassparser.Decompile(interfaceExtends)
	if err != nil {
		t.Fatal(err)
	}
	checkJavaCode(t, results)
	fmt.Println(results)
	assert.Contains(t, results, "NavigableSet extends SortedSet")
}

// checkjavacode
func checkJavaCode(t *testing.T, code string) {
	fmt.Println(code)
	ssatest.CheckJava(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		return nil
	})
	fmt.Println(code)
}
