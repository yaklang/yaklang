package java

import (
	_ "embed"
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"
)

//go:embed sample/fastjson/ParserConfig.java
var parserConfigCode []byte

func TestFastjson(t *testing.T) {
	prog, err := ssaapi.Parse(string(parserConfigCode), ssaapi.WithLanguage(ssaapi.JAVA))
	if err != nil {
		t.Fatal(err)
	}
	matched, err := prog.SyntaxFlowWithError("deserializers.put(* as $className,* as $deserializer) as $call", sfvm.WithEnableDebug(true))
	if err != nil {
		t.Fatal(err)
	}
	deserializerList, ok := matched["deserializer"]
	if !ok {
		t.Fatal(errors.New("deserializer not found"))
	}
	//deserializerNames := []string{}
	deserializerSet := utils.NewSet[string]()
	for _, value := range deserializerList {
		deserializerSet.Add(value.GetObject().GetName())
	}
	deserializerNames := deserializerSet.List()
	println(len(deserializerNames))
	for _, name := range deserializerNames {
		println(name)
	}
	assert.Equal(t, 23, len(deserializerNames))
}
