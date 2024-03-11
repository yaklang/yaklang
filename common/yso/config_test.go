package yso

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yserx"
	"testing"
)

// TestGetConfig assert config is valid, test load serialized payload and class payload
func TestGetConfig(t *testing.T) {
	for name, classInfo := range YsoConfigInstance.Classes {
		classIns, err := javaclassparser.Parse(classInfo.Template)
		if err != nil {
			t.Fatal(utils.Errorf("parse class %s failed: %s", name, err))
		}
		for _, param := range classInfo.Params {
			constant := classIns.FindConstStringFromPool(fmt.Sprintf("{{%s}}", param.Name))
			if constant == nil {
				t.Fatalf("param %s not found in class %s", param.Name, name)
			}
		}
	}
	for name, gadgetInfo := range YsoConfigInstance.Gadgets {
		if gadgetInfo.IsTemplateImpl {
			_, err := yserx.ParseJavaSerialized(gadgetInfo.Template)
			if err != nil {
				t.Fatal(utils.Errorf("parse class %s failed: %s", name, err))
			}
		} else {
			for k, templ := range gadgetInfo.ChainTemplate {
				if len(templ) == 0 {
					continue
				}
				_, err := yserx.ParseJavaSerialized(templ)
				if err != nil {
					t.Fatal(utils.Errorf("parse class %s failed: %s", k, err))
				}
			}
		}
	}
}
