package yakvm

import (
	"fmt"
)

var runtimeLib = map[string]func(frame *Frame) interface{}{
	"GetInfo": func(frame *Frame) interface{} {
		return func(infoType string, args ...interface{}) (interface{}, error) {
			switch infoType {
			case "line":
				return frame.CurrentCode().StartLineNumber, nil
			case "runtimeId":
				result, ok := frame.GlobalVariables.Load("runtimeId")
				if !ok {
					return "", nil
				}
				return result, nil
			default:
				return nil, fmt.Errorf("unknown info type: %s", infoType)
			}
		}
	},
}

func ImportRuntimeLib(frame *Frame) {
	lib := map[string]interface{}{}
	for k, v := range runtimeLib {
		lib[k] = v(frame)
	}
	frame.GlobalVariables.Store("runtime", lib)
}
