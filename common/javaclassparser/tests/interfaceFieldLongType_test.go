package tests

import (
	_ "embed"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"testing"
)

//go:embed interfaceFieldLongType.class
var interfaceFieldLongType []byte

func TestInterfaceFieldLongType(t *testing.T) {
	results, err := javaclassparser.Decompile(interfaceFieldLongType)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(results)
	assert.Contains(t, results, "final long TIME_UNAVAILABLE = -1L")
}
