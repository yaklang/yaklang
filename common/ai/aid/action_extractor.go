package aid

import (
	"encoding/json"
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type Action struct {
	name   string
	params aitool.InvokeParams
}

func (q *Action) Name() string {
	return q.name
}

func (q *Action) GetInt(key string, defaults ...int) int {
	return int(q.params.GetInt(key, lo.Map(defaults, func(item int, index int) int64 {
		return int64(item)
	})...))
}

// GetFloat
func (a *Action) GetFloat(key string, defaults ...float64) float64 {
	return a.params.GetFloat(key, defaults...)
}

func (q *Action) GetString(key string, defaults ...string) string {
	return q.params.GetString(key, defaults...)
}

func (q *Action) GetBool(key string, defaults ...bool) bool {
	return q.params.GetBool(key, defaults...)
}

func (q *Action) GetInvokeParams(key string) aitool.InvokeParams {
	return q.params.GetObject(key)
}

func (q *Action) GetInvokeParamsArray(key string) []aitool.InvokeParams {
	return q.params.GetObjectArray(key)
}

func ExtractAction(i string, actionName string, alias ...string) (*Action, error) {
	ac := &Action{
		name:   actionName,
		params: make(map[string]any),
	}

	actions := []string{actionName}
	actions = append(actions, alias...)
	sigchan := make(chan struct{})
	allFinished := make(chan struct{})
	var err error
	go func() {
		defer func() {
			utils.TryCloseChannel(allFinished)
		}()
		defer func() {
			utils.TryCloseChannel(sigchan)
		}()

		stopped := utils.NewBool(false)
		fmt.Println(string(i))
		err = jsonextractor.ExtractStructuredJSON(i, jsonextractor.WithObjectCallback(func(data map[string]any) {
			if stopped.IsSet() {
				return
			}

			target, ok := data["@action"]
			if !ok {
				return
			}
			for _, name := range actions {
				if target == name {
					ac.name = name
					ac.params = data
					if ac.params == nil {
						ac.params = make(map[string]any)
					}
					close(sigchan)
					stopped.Set()
					return
				}
			}
		}))
		if err != nil {
			log.Error("Failed to extract action", "action", i, "error", err)
		}
	}()
	<-sigchan

	if len(ac.params) > 0 {
		return ac, nil
	}

	<-allFinished
	if err != nil {
		return nil, err
	}
	return nil, utils.Errorf("cannot extract action from: %v", utils.ShrinkString(i, 100))

	//for _, pairs := range jsonextractor.ExtractObjectIndexes(i) {
	//	start, end := pairs[0], pairs[1]
	//	jsonRaw := i[start:end]
	//	var i = make(map[string]any)
	//	err := json.Unmarshal([]byte(jsonRaw), &i)
	//	if err != nil {
	//		log.Warnf("try to unmarshal action[%#v] failed: %v", jsonRaw, err)
	//		continue
	//	}
	//	if rawData, ok := i["@action"]; ok && fmt.Sprint(rawData) != "" {
	//		keys := []string{actionName}
	//		keys = append(keys, alias...)
	//		matched := false
	//		action := fmt.Sprint(rawData)
	//		for _, key := range keys {
	//			if action == key {
	//				matched = true
	//				break
	//			}
	//		}
	//		if !matched {
	//			log.Errorf("action[%#v] not matched in %v", action, keys)
	//			continue
	//		}
	//		ac.name = action
	//		ac.params = i
	//		if ac.params == nil {
	//			ac.params = make(map[string]any)
	//		}
	//		return ac, nil
	//	}
	//}
	//return nil, utils.Errorf("cannot extract action from: %v", utils.ShrinkString(i, 100))
}

func ExtractAllAction(i string) []*Action {
	acs := []*Action{}
	for _, pairs := range jsonextractor.ExtractObjectIndexes(i) {
		ac := &Action{
			params: make(map[string]any),
		}
		start, end := pairs[0], pairs[1]
		jsonRaw := i[start:end]
		var i = make(map[string]any)
		err := json.Unmarshal([]byte(jsonRaw), &i)
		if err != nil {
			continue
		}
		if rawData, ok := i["@action"]; ok && fmt.Sprint(rawData) != "" {
			action := fmt.Sprint(rawData)
			ac.name = action
			ac.params = i
			if ac.params == nil {
				ac.params = make(map[string]any)
			}
			acs = append(acs, ac)
		}
	}
	return acs
}
