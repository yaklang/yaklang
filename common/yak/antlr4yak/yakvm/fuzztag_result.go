package yakvm

import "github.com/yaklang/yaklang/common/log"

func (v *Frame) pushFuzzTagResult(result []string, err error) {
	if err != nil {
		log.Error(err)
		result = []string{}
	}
	v.push(NewStringSliceValue(result))
}
