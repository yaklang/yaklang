package ssaapi

import "github.com/yaklang/yaklang/common/syntaxflow/sfvm"

type NativeCallDocument struct {
	Name        string
	Description string
	Function    sfvm.NativeCallFunc
}

var NativeCallDocuments = make(map[string]*NativeCallDocument)

func nc_desc(description string) func(*NativeCallDocument) {
	return func(n *NativeCallDocument) {
		n.Description = description
	}
}

func nc_func(f sfvm.NativeCallFunc) func(*NativeCallDocument) {
	return func(n *NativeCallDocument) {
		n.Function = f
	}
}
