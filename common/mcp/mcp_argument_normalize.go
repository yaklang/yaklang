package mcp

import (
	"encoding/base64"
	"maps"

	"github.com/yaklang/yaklang/common/utils"
)

// normalizeMCPArguments fixes common MCP argument shapes before mapstructure decode.
func normalizeMCPArguments(arguments map[string]any) map[string]any {
	if arguments == nil {
		return nil
	}
	args := maps.Clone(arguments)

	if nested, ok := args["request"].(map[string]any); ok {
		for k, v := range nested {
			if _, exists := args[k]; !exists {
				args[k] = v
			}
		}
		delete(args, "request")
	}

	if group, ok := args["group"].(map[string]any); ok {
		for k, v := range group {
			if _, exists := args[k]; !exists {
				args[k] = v
			}
		}
		delete(args, "group")
	}

	if rule, ok := args["rule"]; ok {
		if _, exists := args["syntaxFlowInput"]; !exists {
			args["syntaxFlowInput"] = rule
		}
	}
	if fp, ok := args["fingerprint"]; ok {
		if _, exists := args["rule"]; !exists {
			args["rule"] = fp
		}
		delete(args, "fingerprint")
	}
	if obj, ok := args["object"].(map[string]any); ok {
		if data, ok := obj["data"]; ok {
			if _, exists := args["data"]; !exists {
				args["data"] = data
			}
		}
		delete(args, "object")
	}

	if gadget := utils.InterfaceToString(args["gadget"]); gadget == "" {
		args["gadget"] = "URLDNS"
	}

	if rules, ok := args["rules"]; ok {
		if m, ok := rules.(map[string]any); ok && len(m) == 0 {
			args["rules"] = []any{}
		}
	}

	if raw, ok := args["jsonRaw"]; ok {
		switch v := raw.(type) {
		case string:
			args["jsonRaw"] = []byte(v)
		case []any:
			args["jsonRaw"] = utils.InterfaceToBytes(v)
		}
	}

	if data, ok := args["data"]; ok {
		switch v := data.(type) {
		case string:
			if raw, err := base64.StdEncoding.DecodeString(v); err == nil && len(raw) > 0 {
				args["data"] = raw
			}
		}
	}

	if _, ok := args["filter"]; !ok {
		if _, hasPagination := args["pagination"]; hasPagination {
			args["filter"] = map[string]any{}
		}
	}

	return args
}
