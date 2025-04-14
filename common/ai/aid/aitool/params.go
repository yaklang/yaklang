package aitool

import (
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
)

type InvokeParams map[string]any

func (p InvokeParams) GetObject(key string) InvokeParams {
	if !utils.IsNil(p) {
		return utils.MapGetMapRaw(p, key)
	}
	return make(InvokeParams)
}

func (p InvokeParams) GetObjectArray(key string) []InvokeParams {
	result := make([]InvokeParams, 0)
	if !utils.IsNil(p) {
		r, ok := p[key]
		if !ok {
			return result
		}
		funk.ForEach(r, func(v any) {
			item := utils.InterfaceToGeneralMap(v)
			result = append(result, item)
		})
		return result
	}
	return result
}

func (p InvokeParams) GetString(key string, backups ...string) string {
	var results string
	if !utils.IsNil(p) {
		results = utils.MapGetString(p, key)
	}
	if len(backups) <= 0 {
		return results
	}
	if results == "" {
		for _, i := range backups {
			if i != "" {
				return i
			}
		}
	}
	return ""
}

func (p InvokeParams) GetStringSlice(key string, backups ...[]string) []string {
	var results []string
	if !utils.IsNil(p) {
		results = utils.MapGetStringSlice(p, key)
	}
	if len(backups) <= 0 {
		return results
	}
	if len(results) == 0 {
		for _, i := range backups {
			if len(i) > 0 {
				return i
			}
		}
	}
	return results
}

func (p InvokeParams) GetInteger(key string, backups ...int) int {
	var result int
	if !utils.IsNil(p) {
		result = utils.MapGetInt(p, key)
	}
	if len(backups) <= 0 {
		return result
	}
	if result == 0 {
		for _, i := range backups {
			if i != 0 {
				return i
			}
		}
	}
	return result
}

func (p InvokeParams) GetInt(key string, backups ...int64) int64 {
	var result int64
	if !utils.IsNil(p) {
		result = utils.MapGetInt64(p, key)
	}
	if len(backups) <= 0 {
		return result
	}
	if result == 0 {
		for _, i := range backups {
			if i != 0 {
				return i
			}
		}
	}
	return result
}

func (p InvokeParams) GetFloat(key string, backups ...float64) float64 {
	var result float64
	if !utils.IsNil(p) {
		result = utils.MapGetFloat64(p, key)
	}
	if len(backups) <= 0 {
		return result
	}
	if result == 0 {
		for _, i := range backups {
			if i != 0 {
				return i
			}
		}
	}
	return result
}

func (p InvokeParams) GetBool(key string, backups ...bool) bool {
	var result bool
	if !utils.IsNil(p) {
		result = utils.MapGetBool(p, key)
	}
	if len(backups) <= 0 {
		return result
	}
	if !result {
		for _, i := range backups {
			if i {
				return i
			}
		}
	}
	return result
}
