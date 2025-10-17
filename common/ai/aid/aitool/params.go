package aitool

import (
	"bytes"
	"reflect"
	"strings"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
)

type InvokeParams map[string]any

func (r InvokeParams) Dump() string {
	if r == nil || len(r) == 0 {
		return ""
	}

	var buf bytes.Buffer
	for k, v := range r {
		buf.WriteString(utils.InterfaceToString(k))
		buf.WriteString(":")
		vStr := utils.EscapeInvalidUTF8Byte([]byte(utils.InterfaceToString(v)))
		if vStr == "" {
			buf.WriteString(` ""`)
		} else if strings.Contains(vStr, `\n`) {
			buf.WriteString(`\n`)
			buf.WriteString(utils.PrefixLines(vStr, `  `))
		} else {
			buf.WriteString(" ")
			buf.WriteString(vStr)
		}
		buf.WriteString("\n")
	}

	return buf.String()
}

func (p InvokeParams) GetObject(key string) InvokeParams {
	if !utils.IsNil(p) {
		return utils.MapGetMapRaw(p, key)
	}
	return make(InvokeParams)
}

func (p InvokeParams) Set(k string, v any) InvokeParams {
	if utils.IsNil(p) {
		p = make(InvokeParams)
	}
	p[k] = v
	return p
}

func (p InvokeParams) GetObjectArray(key string) []InvokeParams {
	result := make([]InvokeParams, 0)
	if !utils.IsNil(p) {
		r, ok := p[key]
		if !ok {
			return result
		}
		var arrType = reflect.ValueOf(r).Type()
		if arrType.Kind() == reflect.Slice || arrType.Kind() == reflect.Array {
			funk.ForEach(r, func(v any) {
				item := utils.InterfaceToGeneralMap(v)
				result = append(result, item)
			})
		} else if arrType.Kind() == reflect.Map {
			funk.ForEach(r, func(k any, v any) {
				item := utils.InterfaceToGeneralMap(v)
				result = append(result, item)
			})
		} else {
			result = append(result, utils.InterfaceToGeneralMap(r))
		}
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

var specialAttr = []string{
	"value",
	"const",
	"description",
	"desc",
}

func invokeParamAnyToString(rawValue any) string {
	if utils.IsNil(rawValue) {
		return ""
	}
	switch v := rawValue.(type) {
	case string:
		return v
	default:
		resMap := InvokeParams(utils.InterfaceToGeneralMap(rawValue))
		for _, s := range specialAttr {
			if res := resMap.GetString(s); res != "" {
				return res
			}
		}
	}
	return ""
}

func (p InvokeParams) GetAnyToString(key string, backups ...string) string {
	var results string
	if !utils.IsNil(p) {
		results = invokeParamAnyToString(utils.MapGetRaw(p, key))
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

func (p InvokeParams) Has(key string) bool {
	if !utils.IsNil(p) {
		_, ok := p[key]
		return ok
	}
	return false
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
