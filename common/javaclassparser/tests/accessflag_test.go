package tests

import (
	_ "embed"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"testing"
)

//go:embed accessflag.class
var interfaceFlag []byte

func TestAccessFlag(t *testing.T) {
	results, err := javaclassparser.Decompile(interfaceFlag)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(results)
	assert.Contains(t, results, "public interface RegexpMatcher")
	assert.Contains(t, results, "throws BuildException;")
	assert.Contains(t, results, "matches(String var")
}
