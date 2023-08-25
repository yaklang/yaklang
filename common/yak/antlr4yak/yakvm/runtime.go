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
	frame.GlobalVariables["runtime"] = lib
}
