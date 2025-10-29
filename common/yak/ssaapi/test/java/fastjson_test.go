package java

import (
	_ "embed"
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/log"

	"github.com/stretchr/testify/assert"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

//go:embed sample/fastjson/ParserConfig.java
var parserConfigCode []byte

func TestFastjson(t *testing.T) {
	prog, err := ssaapi.Parse(string(parserConfigCode), ssaapi.WithLanguage(ssaconfig.JAVA))
	if err != nil {
		t.Fatal(err)
	}
	res, err := prog.SyntaxFlowWithError("deserializers.put(,,* as $deserializer) as $call", ssaapi.QueryWithEnableDebug(true))
	if err != nil {
		t.Fatal(err)
	}
	log.Infof("result: %v", res)
	deserializerList := res.GetValues("deserializer")
	//deserializerNames := []string{}
	deserializerSet := utils.NewSet[string]()
	for _, value := range deserializerList {
		name := value.GetObject().GetName()
		if name == "" {
			fmt.Println(fmt.Sprintf("value:[%s] get obj name is null", value.GetName()))
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
