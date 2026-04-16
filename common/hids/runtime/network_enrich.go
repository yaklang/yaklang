//go:build hids && linux

package runtime

import (
	"strings"

	"github.com/yaklang/yaklang/common/hids/model"
	"github.com/yaklang/yaklang/common/hids/policy"
)

func enrichNetworkEvent(event model.Event) model.Event {
	if event.Network == nil {
		return event
	}

	data := cloneAnyMap(event.Data)
	if data == nil {
		data = map[string]any{}
	}

	sourceScope := policy.AddressScope(event.Network.SourceAddress)
	destScope := policy.AddressScope(event.Network.DestAddress)
	sourceService := policy.PortServiceName(event.Network.Protocol, event.Network.SourcePort)
	destService := policy.PortServiceName(event.Network.Protocol, event.Network.DestPort)
	processRoles := policy.ProcessRoles(processName(event.Process), processImage(event.Process), processCommand(event.Process))
	parentRoles := policy.ProcessRoles(parentName(event.Process), "", "")

	data["direction"] = inferNetworkDirection(event, sourceScope, destScope, data)
	data["source_scope"] = sourceScope
	data["dest_scope"] = destScope
	if sourceService != "" {
		data["source_service"] = sourceService
	}
	if destService != "" {
		data["dest_service"] = destService
	}
	if len(processRoles) > 0 {
		data["process_roles"] = cloneStringSlice(processRoles)
	}
	if len(parentRoles) > 0 {
		data["parent_roles"] = cloneStringSlice(parentRoles)
	}

	event.Data = data
	return event
}

func inferNetworkDirection(
	event model.Event,
	sourceScope string,
	destScope string,
	existing map[string]any,
) string {
	switch event.Type {
	case model.EventTypeNetworkAccept:
		if destScope == "loopback" {
			return "local"
		}
		if destScope == "private" {
			return "internal"
		}
		return "inbound"
	case model.EventTypeNetworkConnect:
		if sourceScope == "loopback" && destScope == "loopback" {
			return "local"
		}
		if destScope == "private" {
			return "internal"
		}
		if destScope == "public" {
			return "outbound"
		}
	case model.EventTypeNetworkState, model.EventTypeNetworkClose:
		if value := readStringMapValue(existing, "direction"); value != "" {
			return value
		}
	}

	if sourceScope == "loopback" && destScope == "loopback" {
		return "local"
	}
	if sourceScope == "private" && destScope == "private" {
		return "internal"
	}
	if destScope == "public" {
		return "outbound"
	}
	if sourceScope == "public" {
		return "inbound"
	}
	return "unknown"
}

func readStringMapValue(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	raw, ok := values[key]
	if !ok {
		return ""
	}
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func processName(process *model.Process) string {
	if process == nil {
		return ""
	}
	return process.Name
}

func processImage(process *model.Process) string {
	if process == nil {
		return ""
	}
	return process.Image
}

func processCommand(process *model.Process) string {
	if process == nil {
		return ""
	}
	return process.Command
}

func parentName(process *model.Process) string {
	if process == nil {
		return ""
	}
	return process.ParentName
}
