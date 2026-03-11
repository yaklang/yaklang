package ssaapi

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (v *Value) lookupMembersOnObject(key *Value) Values {
	if v == nil || key == nil || v.getValue() == nil || key.getValue() == nil {
		return nil
	}
	ret := make(Values, 0)
	for _, member := range ssa.GetMembersByKey(v.getValue(), key.getValue()) {
		if utils.IsNil(member) {
			continue
		}
		ret = append(ret, v.NewValue(member))
	}
	return MergeValues(ret)
}

func (v *Value) lookupMembersOnType(key *Value) Values {
	if v == nil || key == nil || v.ParentProgram == nil || utils.IsNil(v.GetType()) {
		return nil
	}
	ret := make(Values, 0)
	keyName := ssa.GetKeyString(key.getValue())
	if keyName == "" || !syntaxFlowIdentifierPattern.MatchString(keyName) {
		return nil
	}
	rawType := GetBareType(v.GetType())
	if bp, ok := ssa.ToBluePrintType(rawType); ok {
		for _, member := range bp.Read(keyName) {
			if !utils.IsNil(member) {
				ret = append(ret, v.NewValue(member))
			}
		}
	}
	for _, candidate := range v.lookupObjectsByTypeName() {
		ret = append(ret, candidate.lookupMembersOnObject(key)...)
	}
	return MergeValues(ret)
}

func (v *Value) lookupObjectsByTypeName() Values {
	if v == nil || v.ParentProgram == nil || utils.IsNil(v.GetType()) {
		return nil
	}
	seen := map[int64]struct{}{}
	ret := make(Values, 0)
	add := func(values Values) {
		for _, item := range values {
			if item == nil || item.getValue() == nil {
				continue
			}
			if _, ok := seen[item.GetId()]; ok {
				continue
			}
			seen[item.GetId()] = struct{}{}
			ret = append(ret, item)
		}
	}
	candidates := make([]string, 0)
	rawType := GetBareType(v.GetType())
	if rawType == nil {
		return nil
	}
	if name := rawType.String(); name != "" {
		candidates = append(candidates, name)
	}
	candidates = append(candidates, rawType.GetFullTypeNames()...)
	for _, raw := range candidates {
		if raw == "" {
			continue
		}
		parts := strings.FieldsFunc(raw, func(r rune) bool { return r == '.' || r == '/' || r == '\\' })
		if len(parts) > 0 {
			add(v.ParentProgram.Ref(parts[len(parts)-1]))
		}
		add(v.ParentProgram.Ref(raw))
	}
	return ret
}

var syntaxFlowIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var syntaxFlowDottedPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)
var syntaxFlowReservedBases = map[string]struct{}{
	"any":      {},
	"bool":     {},
	"call":     {},
	"check":    {},
	"constant": {},
	"dict":     {},
	"e":        {},
	"else":     {},
	"function": {},
	"g":        {},
	"have":     {},
	"in":       {},
	"list":     {},
	"opcode":   {},
	"phi":      {},
	"r":        {},
	"return":   {},
	"str":      {},
	"then":     {},
	"type":     {},
}

func isQueryableSyntaxFlowBase(name string) bool {
	if !syntaxFlowDottedPattern.MatchString(name) {
		return false
	}
	for _, part := range strings.Split(name, ".") {
		if _, ok := syntaxFlowReservedBases[part]; ok {
			return false
		}
	}
	return true
}

func collectSyntaxFlowBases(name string) []string {
	name = strings.TrimSpace(name)
	name = strings.TrimLeft(name, "*&")
	name = strings.Trim(name, "[]")
	if name == "" {
		return nil
	}
	bases := make([]string, 0, 2)
	seen := map[string]struct{}{}
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || !isQueryableSyntaxFlowBase(candidate) {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		bases = append(bases, candidate)
	}
	add(name)
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '.' || r == '/' || r == '\\' })
	if len(parts) > 0 {
		add(parts[len(parts)-1])
	}
	return bases
}

func (v *Value) queryMemberCandidates(actx *AnalyzeContext, key *Value) Values {
	if v == nil || key == nil || actx == nil || actx.Query == nil {
		return nil
	}
	keyName := ssa.GetKeyString(key.getValue())
	if keyName == "" || !syntaxFlowIdentifierPattern.MatchString(keyName) {
		return nil
	}
	bases := map[string]struct{}{}
	addBase := func(name string) {
		for _, candidate := range collectSyntaxFlowBases(name) {
			bases[candidate] = struct{}{}
		}
	}
	addBase(v.GetName())
	addBase(v.GetVerboseName())
	if typ := GetBareType(v.GetType()); !utils.IsNil(typ) {
		addBase(typ.String())
		for _, full := range typ.GetFullTypeNames() {
			addBase(full)
			parts := strings.FieldsFunc(full, func(r rune) bool { return r == '.' || r == '/' || r == '\\' })
			if len(parts) > 0 {
				addBase(parts[len(parts)-1])
			}
		}
		if bp, ok := ssa.ToBluePrintType(typ); ok {
			addBase(bp.Name)
			for _, parent := range bp.GetAllParentsBlueprint() {
				if parent != nil {
					addBase(parent.Name)
				}
			}
		}
	}
	queries := make([]string, 0, len(bases))
	for base := range bases {
		queries = append(queries, fmt.Sprintf("%s.%s", base, keyName))
	}
	sort.Strings(queries)
	ret := make(Values, 0)
	for _, query := range queries {
		ret = append(ret, actx.Query(query)...)
		if len(ret) > 0 {
			break
		}
	}
	return MergeValues(ret)
}

func filterOutMember(values Values, current *Value) Values {
	ret := make(Values, 0, len(values))
	for _, item := range values {
		if item == nil {
			continue
		}
		if current != nil && ValueCompare(item, current) {
			continue
		}
		ret = append(ret, item)
	}
	return MergeValues(ret)
}

func filterOutDestructor(values Values) Values {
	ret := make(Values, 0, len(values))
	for _, item := range values {
		if item == nil {
			continue
		}
		if isDestructorLikeValue(item) {
			continue
		}
		ret = append(ret, item)
	}
	return MergeValues(ret)
}
