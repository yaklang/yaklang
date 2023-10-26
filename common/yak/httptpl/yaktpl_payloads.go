package httptpl

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"reflect"
)

type YakPayloads struct {
	raw map[string]*YakPayload
}

type YakPayload struct {
	FromFile string
	Data     []string
}

func (y *YakPayloads) GetData() map[string][]string {
	res := map[string][]string{}
	for k, v := range y.raw {
		res[k] = v.Data
	}
	return res
}
func (y *YakPayloads) GetRawPayloads() map[string]*YakPayload {
	return y.raw
}
func (y *YakPayloads) GetRawMap() map[string]any {
	res := map[string]any{}
	for k, payload := range y.raw {
		if payload.FromFile != "" {
			res[k] = payload.FromFile
		} else {
			res[k] = payload.Data
		}
	}
	return res
}
func (y *YakPayloads) AddPayloads(data map[string]any) error {
	for k, v := range data {
		if reflect.TypeOf(v).Kind() == reflect.Slice {
			y.raw[k] = &YakPayload{
				Data: utils.InterfaceToStringSlice(v),
			}
		} else {
			payload := &YakPayload{
				FromFile: toString(v),
			}
			if utils.GetFirstExistedFile(payload.FromFile) != "" {
				payload.Data = utils.ParseStringToLines(payload.FromFile)
				y.raw[k] = payload
			} else {
				err := utils.Errorf("nuclei template payloads file not found: %s", payload.FromFile)
				return err
			}
		}
	}
	return nil
}
func LoadPayloads(data map[string]any) map[string][]string {
	res := map[string][]string{}
	for k, v := range data {
		if reflect.TypeOf(v).Kind() == reflect.Slice {
			res[k] = utils.InterfaceToStringSlice(v)
		} else {
			file := toString(v)
			if utils.GetFirstExistedFile(file) != "" {
				res[k] = utils.ParseStringToLines(file)
			} else {
				err := utils.Errorf("nuclei template payloads file not found: %s", file)
				log.Error(err)
			}
		}
	}
	return res
}
