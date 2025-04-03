package aid

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"

	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type Action struct {
	name   string
	params map[string]any
}

func (q *Action) Name() string {
	return q.name
}

func (q *Action) GetInt(key string, defaults ...int) int {
	if v, ok := q.params[key]; ok {
		return utils.InterfaceToInt(v)
	}
	if len(defaults) > 0 {
		return defaults[0]
	}
	return 0
}

func (q *Action) GetString(key string, defaults ...string) string {
	if v, ok := q.params[key]; ok {
		return utils.InterfaceToString(v)
	}
	if len(defaults) > 0 {
		return defaults[0]
	}
	return ""
}

func (q *Action) GetBool(key string, defaults ...bool) bool {
	if v, ok := q.params[key]; ok {
		return utils.InterfaceToBoolean(v)
	}
	if len(defaults) > 0 {
		return defaults[0]
	}
	return false
}

func (q *Action) GetInvokeParams(key string) aitool.InvokeParams {
	if v, ok := q.params[key]; ok {
		return utils.InterfaceToGeneralMap(v)
	}
	return make(aitool.InvokeParams, 0)
}

func extractAction(i string, actionName string, alias ...string) (*Action, error) {
	ac := &Action{
		name:   actionName,
		params: make(map[string]any),
	}
	for _, pairs := range jsonextractor.ExtractObjectIndexes(i) {
		start, end := pairs[0], pairs[1]
		jsonRaw := i[start:end]
		var i = make(map[string]any)
		err := json.Unmarshal([]byte(jsonRaw), &i)
		if err != nil {
			log.Warnf("try to unmarshal action[%#v] failed: %v", jsonRaw, err)
			continue
		}
		if rawData, ok := i["@action"]; ok && fmt.Sprint(rawData) != "" {
			keys := []string{actionName}
			keys = append(keys, alias...)
			matched := false
			action := fmt.Sprint(rawData)
			for _, key := range keys {
				if action == key {
					matched = true
					break
				}
			}
			if !matched {
				log.Errorf("action[%#v] not matched", action)
				continue
			}
			ac.name = action
			ac.params = i
			if ac.params == nil {
				ac.params = make(map[string]any)
			}
			return ac, nil
		}
	}
	return nil, utils.Errorf("cannot extract action from: %v", utils.ShrinkString(i, 100))
}
