package preprocess

// macroTables holds function-like and object-like macros for expansion.
type macroTables struct {
	function map[string]functionMacro
	object   map[string]string
}

func newMacroTables() macroTables {
	return macroTables{
		function: make(map[string]functionMacro),
		object:   make(map[string]string),
	}
}

func cloneMacroTables(base macroTables) macroTables {
	out := newMacroTables()
	for k, v := range base.function {
		out.function[k] = v
	}
	for k, v := range base.object {
		out.object[k] = v
	}
	return out
}

func (t macroTables) mergeFrom(other macroTables) {
	for k, v := range other.function {
		t.function[k] = v
	}
	for k, v := range other.object {
		t.object[k] = v
	}
}
