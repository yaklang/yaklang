package codegrpc

import (
	"reflect"
	"strconv"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func CodecFlowExec(req *ypb.CodecRequestFlow) (resp *ypb.CodecResponse, err error) {
	getParamsInfo := func(funcName string) []*yakdoc.Field {
		return CodecLibs.Functions[funcName].Params
	}

	covertParamType := func(param string, fieldType reflect.Type) (any, error) {
		switch fieldType.Kind() {
		case reflect.String:
			return param, nil
		case reflect.Int:
			return strconv.Atoi(param)
		case reflect.Int64:
			return strconv.ParseInt(param, 10, 64)
		case reflect.Float64:
			return strconv.ParseFloat(param, 64)
		case reflect.Bool:
			return codec.Atob(param), nil
		case reflect.Slice:
			if fieldType.Elem().Kind() == reflect.Uint8 {
				return []byte(param), nil
			}
		}
		return nil, utils.Errorf("not support type %v", fieldType.Kind())
	}
	var text []byte
	if req.GetInputBytes() != nil {
		text = req.GetInputBytes()
	} else {
		text = []byte(req.GetText())
	}

	codecFlow := NewCodecExecFlow(text, req.GetWorkFlow())
	flowValue := reflect.ValueOf(codecFlow)
	for _, work := range codecFlow.Flow {
		methodValue := flowValue.MethodByName(work.CodecType)
		methodType := methodValue.Type()

		params := make(map[string]string)
		for _, param := range work.Params {
			params[param.Key] = param.Value
		}

		var callParams []reflect.Value
		paramsInfo := getParamsInfo(work.CodecType)
		for i := 0; i < methodType.NumIn(); i++ {
			fieldType := methodType.In(i)
			if param, ok := params[paramsInfo[i].Name]; ok {
				value, err := covertParamType(param, fieldType)
				if err != nil {
					return nil, err
				}
				callParams = append(callParams, reflect.ValueOf(value))
			} else {
				return nil, utils.Errorf("codec param %v not found", paramsInfo[i].Name)
			}
		}
		ret := methodValue.Call(callParams)
		if len(ret) != 1 {
			return nil, utils.Error("codec return invalid")
		}
		if err, ok := ret[0].Interface().(error); ok {
			return nil, err
		}
	}

	result := codec.UTF8AndControlEscapeForEditorView(codecFlow.Text)
	resultVerbose := codec.UTF8SafeEscape(codecFlow.Text)
	isFalseAppearance := (codec.Md5(result) != codec.Md5(codecFlow.Text)) || (codec.Md5(resultVerbose) != codec.Md5(codecFlow.Text))

	return &ypb.CodecResponse{
		IsFalseAppearance: isFalseAppearance,
		ResultVerbose:     resultVerbose,
		Result:            result,
		RawResult:         codecFlow.Text,
	}, nil
}
