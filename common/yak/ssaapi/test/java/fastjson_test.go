package java

import (
	_ "embed"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

//go:embed sample/fastjson/ParserConfig.java
var parserConfigCode []byte

func TestFastjson(t *testing.T) {
	prog, err := ssaapi.Parse(string(parserConfigCode), ssaapi.WithLanguage(ssaapi.JAVA))
	if err != nil {
		t.Fatal(err)
	}
	matched, err := prog.SyntaxFlowWithError("deserializers.put(,* as $deserializer) as $call", sfvm.WithEnableDebug(false))
	if err != nil {
		t.Fatal(err)
	}
	log.Infof("result: %v", matched)
	deserializerList, ok := matched["deserializer"]
	if !ok {
		t.Fatal(errors.New("deserializer not found"))
	}
	//deserializerNames := []string{}
	deserializerSet := utils.NewSet[string]()
	for _, value := range deserializerList {
		name := value.GetObject().GetName()
		if name == "" {
			continue
		}
		deserializerSet.Add(name)
	}
	deserializerNames := deserializerSet.List()
	println(len(deserializerNames))
	for _, name := range deserializerNames {
		println(name)
	}
	assert.Equal(t, 23, len(deserializerNames))
}
