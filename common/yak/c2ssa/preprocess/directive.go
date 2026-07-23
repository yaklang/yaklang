package preprocess

// MacroEnvironment is a scoped macro table with optional parent for #include nesting.
type MacroEnvironment struct {
	parent *MacroEnvironment
	tables MacroTables
}

func NewMacroEnvironment(parent *MacroEnvironment) *MacroEnvironment {
	var base MacroTables
	if parent != nil {
		base = parent.Flatten()
	} else {
		base = NewMacroTables()
	}
	return &MacroEnvironment{
		parent: parent,
		tables: base.Clone(),
	}
}

// Flatten merges parent chain into a single MacroTables (child overrides parent).
func (e *MacroEnvironment) Flatten() MacroTables {
	if e.parent == nil {
		return e.tables.Clone()
	}
	out := e.parent.Flatten()
	out.MergeFrom(e.tables)
	return out
}

// LookupObject finds an object-like macro in this environment (child overrides parent).
// Equivalent to Flatten().Object[name] without allocating a full merged table.
func (e *MacroEnvironment) LookupObject(name string) (string, bool) {
	for cur := e; cur != nil; cur = cur.parent {
		if v, ok := cur.tables.Object[name]; ok {
			return v, true
		}
	}
	return "", false
}

func (e *MacroEnvironment) ApplyDefineLine(line string) bool {
	return ApplyDefineLine(line, &e.tables, false)
}

func (e *MacroEnvironment) ApplyUndef(name string) {
	delete(e.tables.Function, name)
	delete(e.tables.Object, name)
}

func (e *MacroEnvironment) IsDefined(name string) bool {
	if _, ok := e.tables.Function[name]; ok {
		return true
	}
	if _, ok := e.tables.Object[name]; ok {
		return true
	}
	if e.parent != nil {
		return e.parent.IsDefined(name)
	}
	return false
}

func (e *MacroEnvironment) MergeFromIncluded(other *MacroEnvironment) {
	e.MergeFrom(other)
}

func (e *MacroEnvironment) MergeFrom(other *MacroEnvironment) {
	if other == nil {
		return
	}
	e.tables.MergeFrom(other.Flatten())
}
