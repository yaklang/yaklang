package yaklib

import (
	"bytes"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/mixer"
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
			ret = append(ret, utils.InterfaceToStringSlice(param))
		}
	}
	return ret
}

// Mix 对多个字典做笛卡尔积组合，按字典序流式产出每一种组合
// 参数:
//   - raw: 一个或多个字典，每个可为字符串(按行拆分)、[]string、[]byte 或 [][]byte
//
// 返回值:
//   - 输出组合结果的通道，每个元素是各字典各取一个值组成的 []string
//   - 创建混合器失败时返回的错误
//
// Example:
// ```
// // VARS: 把两个字典做笛卡尔积
// ch = dictutil.Mix(["a", "b"], ["1", "2"])~
// all = []
//
//	for v = range ch {
//	    all = append(all, v)
//	}
//
// // STDOUT: 打印第一种组合
// println(all[0])   // OUT: [a 1]
// // assert: 2x2 共四种组合
// assert len(all) == 4, "mix of two 2-item dicts should produce 4 combinations"
// ```
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
