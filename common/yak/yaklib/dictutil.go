package yaklib

import (
	"bytes"
	"fmt"
	"reflect"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/mixer"
)

func _dictParams(i ...interface{}) [][]string {
	var ret [][]string
	for _, param := range i {
		switch r := param.(type) {
		case string:
			ret = append(ret, utils.ParseStringToLines(r))
		case []byte:
			ret = append(ret, utils.ParseStringToLines(string(r)))
		case []string:
			ret = append(ret, r)
		case [][]byte:
			ret = append(ret, utils.ParseStringToLines(string(bytes.Join(r, []byte("\n")))))
		default:
			panic(fmt.Sprintf("unsupported dict params type: %v", reflect.TypeOf(param)))
		}
	}
	return ret
}

func _dictMix(raw ...interface{}) (chan []string, error) {
	var arrays = _dictParams(raw...)
	m, err := mixer.NewMixer(arrays...)
	if err != nil {
		return nil, utils.Errorf("create mixer failed: %s", err)
	}

	outC := make(chan []string)
	go func() {
		defer close(outC)
		for {
			outC <- m.Value()
			err := m.Next()
			if err != nil {
				break
			}
		}
	}()
	return outC, nil
}

var DictUtilExports = map[string]interface{}{
	"Mix": _dictMix,
}
