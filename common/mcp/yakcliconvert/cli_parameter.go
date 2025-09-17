package yakcliconvert

import (
	"encoding/json"
	"fmt"
	"maps"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func ConvertCliParameterToTool(toolName string, prog *ssaapi.Program) *mcp.Tool {
	properties := make(map[string]any)
	description := ""
	requiredProperties := make([]string, 0)

	getConstString := func(v *ssaapi.Value) string {
		if str, ok := v.GetConstValue().(string); ok {
			return str
		}
		return ""
	}
	getConstBool := func(v *ssaapi.Value) bool {
		if b, ok := v.GetConstValue().(bool); ok {
			return b
		}
		return false
	}

	handleJsonSchemaType := func(result map[string]any, funcName string) bool {
		addType := func(jsonType string, description ...string) {
			result["type"] = jsonType
			if len(description) > 0 {
				result["description"] = description[0]
			}
		}
		addArrayItemType := func(jsonType string) {
			result["items"] = map[string]any{"type": jsonType}
		}

		switch funcName {
		case "cli.String", "cli.Text":
			addType("string")
		case "cli.Bool":
			addType("boolean")
		case "cli.Int", "cli.Integer":
			addType("integer")
		case "cli.Double", "cli.Float":
			addType("number")
		case "cli.File":
			addType("string", "(filepath)")
		case "cli.FileNames", "cli.LineDict":
			addType("array", "(multi-filepaths)")
			addArrayItemType("string")
		case "cli.FolderName":
			addType("string", "(folder-path)")
		case "cli.FileOrContent":
			addType("string", "(filepath or content)")
		case "cli.StringSlice":
			addType("array")
			addArrayItemType("string")
		case "cli.YakCode":
			addType("string", "(code)")
		case "cli.HTTPPacket":
			addType("string", "(http-packet)")
		case "cli.Url", "cli.Urls":
			addType("array", "(url)")
			addArrayItemType("string")
		case "cli.Port", "cli.Ports":
			addType("array", "(port, allow ranges, e.g. 5-10)")
			addArrayItemType("string")
		case "cli.Net", "cli.Network", "cli.Host", "cli.Hosts":
			addType("array", "(host, allow cidr, e.g. 192.168.1.0/24)")
			addArrayItemType("string")
		case "cli.Json":
		default:
			return false
		}
		return true
	}

	handleOption := func(field map[string]any, opt *ssaapi.Value) {
		if !opt.IsCall() {
			// skip no function call
			return
		}
		arg1 := getConstString(opt.GetOperand(1))
		var enum []string

		switch opt.GetOperand(0).GetName() {
		case "cli.setHelp":
			if desc, ok := field["description"]; ok {
				field["description"] = fmt.Sprintf("%s %s", arg1, desc)
			} else {
				field["description"] = arg1
			}
		case "cli.setRequired":
			field["required"] = getConstBool(opt.GetOperand(1))
		case "cli.setDefault":
			field["default"] = opt.GetOperand(1).GetConstValue()
		case "cli.setSelectOption":
			enum = append(enum, getConstString(opt.GetOperand(2)))
		case "cli.setJsonSchema":
			schema := make(map[string]any)
			err := json.Unmarshal([]byte(arg1), &schema)
			if err == nil {
				maps.Copy(field, schema)
			}
		}

		if len(enum) > 0 {
			old, ok := field["enum"].([]string)
			if ok {
				field["enum"] = append(old, enum...)
			} else {
				field["enum"] = enum
			}
		}
	}

	parseCliParameterFunc := func(v *ssaapi.Value, funcName string) {
		v.GetUsers().Filter(
			func(v *ssaapi.Value) bool {
				// only function call and must be reachable
				return v.IsCall() && v.IsReachable() != -1
			},
		).ForEach(func(v *ssaapi.Value) {
			if funcName == "cli.help" {
				// skip help function
				description = getConstString(v.GetOperand(1))
				return
			}

			nameValue := v.GetOperand(1)
			paramName := ""
			if nameValue.IsConstInst() {
				if c := nameValue.GetConst(); c.IsString() {
					paramName = c.VarString()
				}
			} else {
				paramName = nameValue.String()
			}

			if paramName == "" {
				return
			}

			field := make(map[string]any)
			valid := handleJsonSchemaType(field, funcName)
			// skip if meet cli options
			if !valid {
				return
			}

			opLen := len(v.GetOperands())
			// handler option
			for i := 2; i < opLen; i++ {
				handleOption(field, v.GetOperand(i))
			}

			// Remove required from property schema and add to InputSchema.required
			if required, ok := field["required"].(bool); ok && required {
				delete(field, "required")
				requiredProperties = append(requiredProperties, paramName)
			}

			properties[paramName] = field
		})
	}

	prog.Ref("cli").GetOperands().ForEach(func(v *ssaapi.Value) {
		if !v.IsFunction() {
			return
		}
		funcName := v.GetName()
		if funcName == "cli.UI" {
			return
		}

		// cli parameter
		parseCliParameterFunc(v, v.GetName())
	})

	tool := mcp.NewTool(toolName)
	tool.Description = description
	tool.InputSchema.Required = requiredProperties

	// 将 map[string]any 转换为 OrderedMap
	orderedProps := omap.NewEmptyOrderedMap[string, any]()
	for k, v := range properties {
		orderedProps.Set(k, v)
	}
	tool.InputSchema.Properties = orderedProps

	return tool
}
