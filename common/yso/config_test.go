package yso

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yserx"
	"testing"
)

// TestGetConfig assert config is valid
func TestGetConfig(t *testing.T) {
	config, err := getConfig()
	if err != nil {
		t.Fatal(err)
	}
	for name, classInfo := range config.Classes {
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
	for name, gadgetInfo := range config.Gadgets {
		gadgetIns, err := yserx.ParseJavaSerialized(gadgetInfo.Template)
		if err != nil {
			t.Fatal(utils.Errorf("parse class %s failed: %s", name, err))
		}
		_ = gadgetIns
	}
}
