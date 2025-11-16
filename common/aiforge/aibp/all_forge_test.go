package aibp

import (
	"fmt"
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var IsMockCallback = false

func ExecuteForge(forgeName string, i any, iopts ...any) (any, error) {
	yakit.CallPostInitDatabase()
	var aiCallback aicommon.AICallbackType
	if IsMockCallback {
		data, err := os.ReadFile(fmt.Sprintf("/tmp/%s.json", forgeName))
		if err != nil {
			return nil, err
		}
		aiCallback = aiforge.MockAICallbackByRecord(data)
	} else {
		aiCallbackRecorder, saveToFile := aiforge.AICallbackRecorder(aiforge.GetOpenRouterAICallback(), fmt.Sprintf("/tmp/%s.json", forgeName))
		defer saveToFile()
		aiCallback = aiCallbackRecorder
	}
	iopts = append(iopts, aicommon.WithAICallback(aiCallback))
	iopts = append(iopts, aicommon.WithDebug(true))
	iopts = append(iopts, aicommon.WithAgreeYOLO(true))
	return yak.ExecuteForge(forgeName, i, iopts...)
}

var TestList = map[string]func(t *testing.T){
	// "PIMatrix": TestPIMatrix,
}

func TestAllForgeByMock(t *testing.T) {
	IsMockCallback = true
	for _, test := range TestList {
		test(t)
	}
}

//func TestAllForge(t *testing.T) {
//	IsMockCallback = false
//	for name, test := range TestList {
//		t.Run(name, test)
//	}
//}
