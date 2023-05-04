package mutate

import (
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"strconv"
	"strings"
)

/*
设置 CODEC 回调函数
*/
var codecCaller func(name string, s interface{}) (string, error)

func InitCodecCaller(caller func(name string, s interface{}) (string, error)) {
	codecCaller = caller
}

func parseUint(s string) (uint, error) {
	i, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(i), nil
}

func RepeatFunc(i uint, handler func() bool) {
	var f uint
	for f = 0; f < i; f++ {
		if !handler() {
			break
		}
	}
}

func ParseStringToParams(i string, separator string) (map[string][]string, error) {
	var m = make(map[string][]string)
	err := json.Unmarshal([]byte(i), &m)
	if err == nil {
		return m, nil
	}

	var mid = make(map[string]interface{})
	err = json.Unmarshal([]byte(i), &mid)
	if err != nil {
		return nil, err
	}

	for key, value := range mid {
		switch ret := value.(type) {
		case string:
			m[key] = strings.Split(ret, separator)
		case bool:
			if ret {
				m[key] = []string{"true"}
			} else {
				m[key] = []string{"false"}
			}
		default:
			m[key] = []string{fmt.Sprintf("%v", ret)}
		}
	}
	return m, nil
}

func MutateWithExtraParams(raw map[string][]string) *RegexpMutateCondition {
	h := func(_ *gorm.DB, s string) ([]string, error) {
		if raw == nil {
			return []string{s}, nil
		}

		data, ok := raw[s]
		if !ok {
			return nil, utils.Errorf("no params[%v] existed in: %v", s, spew.Sdump(raw))
		}
		if data == nil {
			return []string{s}, nil
		}
		return data, nil
	}

	return &RegexpMutateCondition{
		Verbose: "PARAM",
		TagName: "params",
		//Regexp:  paramRegexpCondition,
		Handle: h,
	}
}
