package mcp

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/utils"
)

func NewCommonCallToolResult(data any) (*mcp.CallToolResult, error) {
	var result string
	switch r := data.(type) {
	case string:
		result = r
	case []any:
		return &mcp.CallToolResult{
			Content: r,
		}, nil
	default:
		resultBytes, err := json.Marshal(data)
		if err != nil {
			return nil, utils.Wrap(err, "failed to marshal response")
		}
		result = string(resultBytes)
	}
	return &mcp.CallToolResult{
		Content: []any{
			mcp.TextContent{
				Type: "text",
				Text: string(result),
			},
		},
	}, nil
}

func decodeHook(from reflect.Type, to reflect.Type, v any) (any, error) {
	if to.Kind() == reflect.String {
		if from.Kind() == reflect.Slice {
			slice := utils.InterfaceToSliceInterface(v)
			stringSlice := lo.Map(slice, func(item any, _ int) string {
				return utils.InterfaceToString(item)
			})
			return strings.Join(stringSlice, "\n"), nil
		}
	} else if to.Kind() == reflect.Slice && to.Elem().Kind() == reflect.Uint8 {
		if from.Kind() == reflect.Slice {
			slice := utils.InterfaceToSliceInterface(v)
			bytesSlice := lo.Map(slice, func(item any, _ int) []byte {
				return utils.InterfaceToBytes(item)
			})
			return bytes.Join(bytesSlice, []byte("\n")), nil
		}
	}
	return v, nil
}
