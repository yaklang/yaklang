package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// 此文件负责把 yak 脚本中以 dict 形式声明的 action options 转换成
// []aitool.ToolOption。这一步是把 Yak 表达力比较弱的 schema 描述
// （由于 yak 不直接持有 aitool.PropertyOption 类型）适配到 Go 端真实结构。
//
// 关键词: yak focus mode action options, dict to ToolOption schema

// ParseFocusModeActionOptions 解析 yak 中形如下面的 options 列表：
//
//	options = [
//	    {
//	        "name": "target",
//	        "type": "string",       // string / int / float / number / bool / array / string_array / object
//	        "description": "目标主机",
//	        "required": true,
//	        "default": "",
//	        "enum": ["a", "b"],     // 仅 string
//	        "max_length": 64,
//	        "min_length": 1,
//	        "pattern": "...",
//	        "max": 10, "min": 0,    // 仅数字型
//	        "title": "...",
//	        "example": "...",
//	    },
//	    ...
//	]
//
// 关键词: action options parser, dict to schema, optional flags
func ParseFocusModeActionOptions(items []any) []aitool.ToolOption {
	if len(items) == 0 {
		return nil
	}
	var opts []aitool.ToolOption
	for idx, raw := range items {
		entry := utils.InterfaceToMapInterface(raw)
		if len(entry) == 0 {
			log.Warnf("yak focus mode: action options[%d] is not a dict, skip", idx)
			continue
		}
		name := utils.MapGetString(entry, "name")
		if name == "" {
			log.Warnf("yak focus mode: action options[%d] missing 'name', skip", idx)
			continue
		}

		paramType := utils.MapGetString(entry, "type")
		if paramType == "" {
			paramType = "string"
		}

		var propOpts []aitool.PropertyOption
		if desc := utils.MapGetString(entry, "description"); desc != "" {
			propOpts = append(propOpts, aitool.WithParam_Description(desc))
		}
		if title := utils.MapGetString(entry, "title"); title != "" {
			propOpts = append(propOpts, aitool.WithParam_Title(title))
		}
		if def := utils.MapGetRaw(entry, "default"); !utils.IsNil(def) {
			propOpts = append(propOpts, aitool.WithParam_Default(def))
		}
		if example := utils.MapGetRaw(entry, "example"); !utils.IsNil(example) {
			propOpts = append(propOpts, aitool.WithParam_Example(example))
		}
		if utils.MapGetBool(entry, "required") {
			propOpts = append(propOpts, aitool.WithParam_Required(true))
		}

		// enum / pattern / lengths / numeric bounds 仅在合适类型才生效
		switch paramType {
		case "string", "string_array":
			if enumRaw := utils.MapGetRaw(entry, "enum"); !utils.IsNil(enumRaw) {
				if list, ok := enumRaw.([]any); ok && len(list) > 0 {
					strs := make([]string, 0, len(list))
					for _, v := range list {
						strs = append(strs, utils.InterfaceToString(v))
					}
					propOpts = append(propOpts, aitool.WithParam_EnumString(strs...))
				}
			}
			if maxLen := utils.MapGetInt(entry, "max_length"); maxLen > 0 {
				propOpts = append(propOpts, aitool.WithParam_MaxLength(maxLen))
			}
			if minLen := utils.MapGetInt(entry, "min_length"); minLen > 0 {
				propOpts = append(propOpts, aitool.WithParam_MinLength(minLen))
			}
			if pattern := utils.MapGetString(entry, "pattern"); pattern != "" {
				propOpts = append(propOpts, aitool.WithParam_Pattern(pattern))
			}
		case "integer", "int", "number", "float":
			if max := utils.MapGetFloat64(entry, "max"); max != 0 {
				propOpts = append(propOpts, aitool.WithParam_Max(max))
			}
			if min := utils.MapGetFloat64(entry, "min"); min != 0 {
				propOpts = append(propOpts, aitool.WithParam_Min(min))
			}
		}

		switch paramType {
		case "string":
			opts = append(opts, aitool.WithStringParam(name, propOpts...))
		case "integer", "int":
			opts = append(opts, aitool.WithIntegerParam(name, propOpts...))
		case "float", "number":
			opts = append(opts, aitool.WithNumberParam(name, propOpts...))
		case "bool", "boolean":
			opts = append(opts, aitool.WithBoolParam(name, propOpts...))
		case "string_array":
			opts = append(opts, aitool.WithStringArrayParam(name, propOpts...))
		default:
			log.Warnf("yak focus mode: unsupported action option type %q for %q, fallback to string", paramType, name)
			opts = append(opts, aitool.WithStringParam(name, propOpts...))
		}
	}
	return opts
}
