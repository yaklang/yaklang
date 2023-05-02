package yakdocument

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"testing"
	"yaklang.io/yaklang/common/yak"
	"yaklang.io/yaklang/common/yak/yaklang"

	_ "yaklang.io/yaklang/common/yak"
)

func TestEngineToLibDocuments(t *testing.T) {
	libs := yak.EngineToLibDocuments(yaklang.New())

	for _, lib := range libs {
		_ = lib
		fmt.Printf("YakLib: %12s  [%v]Variable [%v]Functions\n", lib.Name, len(lib.Variables), len(lib.Functions))
		raw, err := yaml.Marshal(lib)
		if err != nil {
			continue
		}
		_ = raw
	}
}
