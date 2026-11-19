package cpp2ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// StdlibType describes one standard-library name family that the frontend knows
// without reading the real headers. The declarations are placeholders: they give
// types and callable member names so real projects do not fail on a missing type,
// but they carry no behaviour.
// TODO(ssa-cpp): model standard-library behaviour; placeholder members return
// undefined and dataflow does not cross library calls.
type StdlibType struct {
	// Names are the spellings accepted for this family, without the std:: prefix.
	Names []string
	// Headers lists the standard headers that introduce the family.
	Headers []string
	// Methods are the member names registered as callable placeholders.
	Methods []string
	// Fields are the data member names registered as placeholder values.
	Fields []string
}

// stdlibCatalog is the single baseline table for standard-library support. It is
// intentionally one list (not one per C++ standard) so a project never has to
// pick a header set per language version.
var stdlibCatalog = []StdlibType{
	{
		Names:   []string{"string", "basic_string", "wstring", "u16string", "u32string"},
		Headers: []string{"string"},
		Methods: []string{"size", "length", "empty", "clear", "append", "push_back", "pop_back", "substr", "find", "rfind", "compare", "c_str", "data", "at", "insert", "erase", "replace", "begin", "end", "front", "back", "resize", "reserve", "capacity", "swap", "operator[]"},
	},
	{
		Names:   []string{"string_view"},
		Headers: []string{"string_view"},
		Methods: []string{"size", "length", "empty", "substr", "find", "compare", "data", "begin", "end", "front", "back", "remove_prefix", "remove_suffix"},
	},
	{
		Names:   []string{"vector"},
		Headers: []string{"vector"},
		Methods: []string{"push_back", "pop_back", "emplace_back", "emplace", "insert", "erase", "size", "max_size", "capacity", "empty", "clear", "resize", "reserve", "at", "front", "back", "begin", "end", "data", "swap", "operator[]"},
	},
	{
		Names:   []string{"array", "deque", "list", "forward_list", "initializer_list"},
		Headers: []string{"array", "deque", "list", "forward_list", "initializer_list"},
		Methods: []string{"push_back", "pop_back", "push_front", "pop_front", "emplace_back", "emplace_front", "insert", "erase", "size", "empty", "clear", "at", "front", "back", "begin", "end", "data", "swap", "operator[]"},
	},
	{
		Names:   []string{"map", "unordered_map", "multimap", "unordered_multimap"},
		Headers: []string{"map", "unordered_map"},
		Methods: []string{"insert", "emplace", "erase", "find", "count", "at", "size", "empty", "clear", "begin", "end", "contains", "operator[]"},
	},
	{
		Names:   []string{"set", "unordered_set", "multiset", "unordered_multiset"},
		Headers: []string{"set", "unordered_set"},
		Methods: []string{"insert", "emplace", "erase", "find", "count", "size", "empty", "clear", "begin", "end", "contains"},
	},
	{
		Names:   []string{"pair", "tuple"},
		Headers: []string{"utility", "tuple"},
		Fields:  []string{"first", "second"},
	},
	{
		Names:   []string{"unique_ptr", "shared_ptr", "weak_ptr", "auto_ptr"},
		Headers: []string{"memory"},
		Methods: []string{"get", "reset", "release", "swap", "use_count", "expired", "lock"},
	},
	{
		Names:   []string{"optional"},
		Headers: []string{"optional"},
		Methods: []string{"has_value", "value", "value_or", "reset", "swap"},
	},
	{
		Names:   []string{"stack", "queue", "priority_queue"},
		Headers: []string{"stack", "queue"},
		Methods: []string{"push", "pop", "top", "front", "back", "size", "empty", "emplace"},
	},
	{
		Names:   []string{"istream", "ostream", "iostream", "istringstream", "ostringstream", "stringstream", "ifstream", "ofstream", "fstream", "wistream", "wostream"},
		Headers: []string{"iostream", "sstream", "fstream"},
		Methods: []string{"read", "write", "get", "getline", "put", "peek", "tellg", "tellp", "seekg", "seekp", "flush", "good", "bad", "eof", "fail", "is_open", "open", "close", "str", "clear", "operator<<", "operator>>"},
	},
	{
		Names:   []string{"exception", "runtime_error", "logic_error", "out_of_range", "invalid_argument", "length_error", "domain_error", "range_error", "overflow_error", "underflow_error", "bad_alloc", "bad_cast", "system_error"},
		Headers: []string{"exception", "stdexcept", "new", "typeinfo", "system_error"},
		Methods: []string{"what", "code", "message"},
	},
	{
		Names:   []string{"thread", "mutex", "recursive_mutex", "condition_variable", "atomic", "future", "promise"},
		Headers: []string{"thread", "mutex", "condition_variable", "atomic", "future"},
		Methods: []string{"lock", "unlock", "try_lock", "join", "detach", "get", "wait", "notify_one", "notify_all", "load", "store"},
	},
	{
		Names:   []string{"function", "bind", "reference_wrapper"},
		Headers: []string{"functional"},
		Methods: []string{"operator()", "target", "get"},
	},
	{
		Names:   []string{"iterator", "const_iterator", "reverse_iterator"},
		Headers: []string{"iterator"},
		Methods: []string{"begin", "end", "next", "prev", "advance", "distance"},
	},
	{
		Names:   []string{"size_t", "ssize_t", "ptrdiff_t", "intptr_t", "uintptr_t"},
		Headers: []string{"cstddef", "cstdint"},
	},
}

// stdlibIndex maps an accepted spelling to its catalog entry.
var stdlibIndex = func() map[string]*StdlibType {
	index := make(map[string]*StdlibType)
	for i := range stdlibCatalog {
		for _, name := range stdlibCatalog[i].Names {
			index[name] = &stdlibCatalog[i]
		}
	}
	return index
}()

// StdlibTypeNames lists the standard-library spellings with placeholder
// declarations. It is exported for the design document and for tests that guard
// the baseline table.
func StdlibTypeNames() []string {
	names := make([]string, 0, len(stdlibIndex))
	for name := range stdlibIndex {
		names = append(names, name)
	}
	return names
}

// StdlibHeaders lists the standard headers covered by the baseline table.
func StdlibHeaders() []string {
	seen := make(map[string]bool)
	headers := make([]string, 0)
	for _, entry := range stdlibCatalog {
		for _, header := range entry.Headers {
			if seen[header] {
				continue
			}
			seen[header] = true
			headers = append(headers, header)
		}
	}
	return headers
}

func lookupStdlibType(name string) *StdlibType {
	return stdlibIndex[name]
}

// ensureStdlibBlueprint creates (once) the std:: blueprint for a catalog entry
// and registers its placeholder methods and fields.
func (b *singleFileBuilder) ensureStdlibBlueprint(entry *StdlibType, spelling string) *ssa.Blueprint {
	if entry == nil || b == nil || b.FunctionBuilder == nil {
		return nil
	}
	fullName := "std::" + spelling
	if bp := b.GetBluePrint(fullName); bp != nil {
		return bp
	}
	bp := b.CreateBlueprint(fullName)
	bp.SetKind(ssa.BlueprintClass)
	for _, field := range entry.Fields {
		value := b.EmitUndefined(field)
		bp.RegisterNormalMember(field, value)
	}
	for _, method := range entry.Methods {
		bp.RegisterNormalMethod(method, b.newPlaceholderMethod(bp, method))
	}
	return bp
}

// newPlaceholderMethod builds a callable that returns an undefined value. The
// body is empty so the call graph stays honest: the frontend does not claim to
// know what a library method does.
func (b *singleFileBuilder) newPlaceholderMethod(bp *ssa.Blueprint, name string) *ssa.Function {
	fn := b.NewFunc(name)
	fn.SetMethodName(name)
	previous := b.FunctionBuilder
	b.FunctionBuilder = b.PushFunction(fn)
	this := b.NewParam("this")
	if !utils.IsNil(bp) {
		this.SetType(bp)
	}
	b.EmitReturn([]ssa.Value{b.EmitUndefined(name + "-return")})
	b.Finish()
	b.FunctionBuilder = b.PopFunction()
	b.FunctionBuilder = previous
	return fn
}

// stdlibBaseName reports the catalog base name of a type spelling, ignoring the
// std:: prefix and any template arguments.
func stdlibBaseName(text string) string {
	base, _ := splitTemplateSpelling(text)
	base = strings.TrimPrefix(base, "std::")
	base = strings.TrimPrefix(base, "::")
	return base
}
