package yakdocument

import (
	"fmt"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"gopkg.in/yaml.v2"
	"testing"

	_ "github.com/yaklang/yaklang/common/yak"
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
