package bash2ssa

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"mvdan.cc/sh/v3/pattern"
	"mvdan.cc/sh/v3/syntax"
	"sort"
	"strconv"
	"strings"
)

// TODO(ssa-bash): a non-constant subscript cannot select a concrete element; the read stays a member-call value.
func (ab *astBuilder) arrayKey(name string, index syntax.ArithmExpr) ssa.Value {
	if word, ok := index.(*syntax.Word); ok && ab.assoc[name] {
		return ab.buildWord(word)
	}
	return ab.buildArithm(index)
}

func (ab *astBuilder) arrayLiteral(name string, arr *syntax.ArrayExpr) ssa.Value {
	var keys, vals []ssa.Value
	var next ssa.Value = ab.EmitConstInst(0)
	for _, elem := range arr.Elems {
		key := next
		if elem.Index != nil {
			key = ab.arrayKey(name, elem.Index)
		}
		value := ab.buildWord(elem.Value)
		if value == nil {
			value = ab.EmitConstInst("")
		}
		keys = append(keys, key)
		vals = append(vals, value)
		if !ab.assoc[name] {
			next = ab.EmitBinOp(ssa.OpAdd, key, ab.EmitConstInst(1))
		}
	}
	return ab.CreateObjectWithMap(keys, vals)
}

// Copy before an element write. Mutating the entering Make would also change
// the other branch of an if, or the parent of a command substitution.
func (ab *astBuilder) assignElement(name string, key, value ssa.Value, appendValue bool) ssa.Value {
	old := ab.PeekValue(name)
	var keys, vals []ssa.Value
	if old != nil {
		for _, pair := range ssa.GetLastWinsMemberPairs(old) {
			if pair.Key.String() != key.String() {
				keys = append(keys, pair.Key)
				vals = append(vals, pair.Member)
			}
		}
		if appendValue {
			value = ab.concat(ab.ReadMemberCallValue(old, key), value)
		}
	}
	obj := ab.CreateObjectWithMap(append(keys, key), append(vals, value))
	ab.put(name, obj)
	return value
}

// An argument vector has separate fields for the count and joined string.
// Neither is a positional argument. A call still exposes its original Args.
func (ab *astBuilder) argv(values []ssa.Value) ssa.Value {
	vec := ab.CreateObjectWithSlice(values)
	ab.argvFrames[vec.GetId()] = values
	ab.AssignVariable(ab.CreateMemberCallVariable(vec, ab.EmitConstInst("#")), ab.EmitConstInst(len(values)))
	sep := " "
	if ifs, ok := ssa.ToConstInst(ab.PeekValue("IFS")); ok {
		sep = ""
		for _, r := range ifs.String() {
			sep = string(r)
			break
		}
	}
	ab.AssignVariable(ab.CreateMemberCallVariable(vec, ab.EmitConstInst("*")), ab.join(values, sep))
	return vec
}

func (ab *astBuilder) positional(name string) ssa.Value {
	if name == "0" {
		return ab.readOrCreate("$0")
	}
	vec := ab.readOrCreate("$@")
	if name == "@" {
		return vec
	}
	var key ssa.Value
	if n, err := strconv.ParseUint(name, 10, 63); err == nil && n > 0 {
		if fields, ok := ab.argvFrames[vec.GetId()]; ok {
			if n > uint64(len(fields)) {
				return ab.EmitConstInst("")
			}
			return fields[n-1]
		}
		key = ab.EmitConstInst(int64(n - 1))
	} else {
		key = ab.EmitConstInst(name)
	}
	if _, ok := ssa.ToUndefined(vec); ok {
		return ab.readOrCreate("$" + name)
	}
	return ab.ReadMemberCallValue(vec, key)
}

func (ab *astBuilder) spread(v ssa.Value) ([]ssa.Value, bool) {
	if fields, ok := ab.argvFrames[v.GetId()]; ok {
		return fields, false
	}
	if _, ok := ssa.ToMake(v); ok {
		pairs := ssa.GetLastWinsMemberPairs(v)
		sort.SliceStable(pairs, func(i, j int) bool {
			a, aerr := strconv.Atoi(pairs[i].Key.String())
			b, berr := strconv.Atoi(pairs[j].Key.String())
			return aerr == nil && berr == nil && a < b
		})
		vals := make([]ssa.Value, 0, len(pairs))
		for _, p := range pairs {
			vals = append(vals, p.Member)
		}
		return vals, false
	}
	return []ssa.Value{v}, true
}

func listExpansion(w *syntax.Word) *syntax.ParamExp {
	if len(w.Parts) != 1 {
		return nil
	}
	p := w.Parts[0]
	if q, ok := p.(*syntax.DblQuoted); ok {
		if len(q.Parts) != 1 {
			return nil
		}
		p = q.Parts[0]
	}
	e, ok := p.(*syntax.ParamExp)
	if !ok || e.Length || e.Exp != nil || e.Slice != nil {
		return nil
	}
	if e.Param.Value == "@" {
		return e
	}
	if i, ok := e.Index.(*syntax.Word); ok && i.Lit() == "@" {
		return e
	}
	return nil
}

func (ab *astBuilder) words(words []*syntax.Word) ([]ssa.Value, bool) {
	var fields []ssa.Value
	spread := false
	add := func(v ssa.Value) {
		if v == nil {
			v = ab.EmitUndefined("word")
		}
		fields = append(fields, v)
	}
	for _, w := range words {
		if listExpansion(w) != nil {
			v := ab.buildWord(w)
			if v == nil {
				v = ab.EmitUndefined("word")
			}
			vs, dyn := ab.spread(v)
			if dyn && len(words) > 1 {
				vs = []ssa.Value{ab.intrinsic("spread", v)}
			}
			fields = append(fields, vs...)
			spread = spread || dyn
			continue
		}
		// A single unquoted expansion splits on the shell whitespace. A known
		// constant is resolved into concrete fields; otherwise the split is
		// marked with an intrinsic. Quoted words and assignments are not split.
		if len(w.Parts) == 1 {
			if part, ok := bareExpansion(w); ok {
				v := ab.buildWord(w)
				if v == nil {
					v = ab.EmitUndefined("word")
				}
				if ab.appendKnownFields(v, &fields) {
					continue
				}
				switch p := part.(type) {
				case *syntax.ParamExp:
					// TODO(ssa-bash): a runtime IFS value is passed to the fields intrinsic; the split is not resolved statically.
					if paramExpSplits(p) {
						v = ab.intrinsic("param-fields", v, ab.ifsValue())
						spread = true
					}
				case *syntax.CmdSubst:
					v = ab.intrinsic("cmd-fields", v, ab.ifsValue())
					spread = true
				}
				fields = append(fields, v)
				continue
			}
		}
		v := ab.buildArgWord(w)
		// Quoted empty fields survive; a literal with glob meta expands to paths.
		if len(w.Parts) == 1 {
			if part, ok := w.Parts[0].(*syntax.Lit); ok && pattern.HasMeta(part.Value, pattern.Filenames) {
				if v == nil {
					v = ab.EmitUndefined("word")
				}
				v = ab.intrinsic("pathname-fields", v)
				spread = true
			}
		}
		add(v)
	}
	return fields, spread
}

// bareExpansion reports a word that is exactly one unquoted expansion.
func bareExpansion(w *syntax.Word) (syntax.WordPart, bool) {
	if w == nil || len(w.Parts) != 1 {
		return nil, false
	}
	switch w.Parts[0].(type) {
	case *syntax.ParamExp, *syntax.CmdSubst:
		return w.Parts[0], true
	default:
		return nil, false
	}
}

// appendKnownFields statically splits an expansion whose value is already a
// constant. It reports whether the value was consumed.
func (ab *astBuilder) appendKnownFields(v ssa.Value, fields *[]ssa.Value) bool {
	c, ok := ssa.ToConstInst(v)
	if !ok || !c.IsString() {
		return false
	}
	ifs := ab.ifsValue()
	sep, ok := ssa.ToConstInst(ifs)
	if !ok || sep.String() != " \t\n" || strings.ContainsAny(c.String(), "*?[") {
		return false
	}
	for _, s := range strings.FieldsFunc(c.String(), func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' }) {
		*fields = append(*fields, ab.EmitConstInst(s))
	}
	return true
}

func (ab *astBuilder) packFields(fields []ssa.Value, spread bool) ssa.Value {
	if !spread {
		return ab.argv(fields)
	}
	if len(fields) == 1 {
		return fields[0]
	}
	return ab.intrinsic("concat-argv", fields...)
}

func (ab *astBuilder) needsArgv(fn *ssa.Function) bool {
	if !fn.IsFinished() || len(fn.ParameterMembers) > 0 {
		return true
	}
	if len(fn.Params) == 1 {
		p, ok := fn.GetValueById(fn.Params[0])
		return ok && p.HasUsers()
	}
	return false
}

func positionalName(name string) bool {
	if name == "@" || name == "*" || name == "#" {
		return true
	}
	_, err := strconv.ParseUint(name, 10, 63)
	return err == nil
}
