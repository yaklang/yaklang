package ssa

import (
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

type memberCallReadVisitKey struct {
	objectID     int64
	keyID        int64
	wantFunction bool
}

// memberCallReadVisitSet is the recursion guard for phi member reads and the
// result cache for one top-level read.
//
// Member reads nest only as deep as the phi chain in front of them, so a small
// inline stack beats allocating a map on every top-level read. The overflow map
// is only created if a pathological chain goes past the inline capacity, which
// keeps the guard allocation-free on the common path while staying unbounded-safe.
//
// The guard alone only stops a key from repeating on the active path. A phi DAG
// that reconverges (a branch merges two nodes that share a source) then expands
// every shared node once per path, which is exponential in the nesting depth.
// Minified bundles with hundreds of merged object members turned one member read
// into 16381 emitted phis at depth 12. results therefore reuses the value a
// node already produced inside this read: the read is pure (it emits instructions
// but never rewrites member tables), so the same key resolves to the same value.
type memberCallReadVisitSet struct {
	keys     [memberCallReadVisitStackDepth]memberCallReadVisitKey
	length   int
	overflow map[memberCallReadVisitKey]struct{}

	// results holds completed per-node results of this read. Entries reached
	// while a cycle fallback was used are not recorded: their value depends on
	// the active path and would be wrong for a later path.
	results    [memberCallReadVisitStackDepth]memberCallReadResult
	resultLen  int
	resultOver map[memberCallReadVisitKey]Value
	fallbacks  int
}

type memberCallReadResult struct {
	key   memberCallReadVisitKey
	value Value
}

const memberCallReadVisitStackDepth = 8

// phiEdgeIdentity replaces the formatted dedupe key without copying it.
// Constants still compare by their existing string. Undefined values compare
// by kind, verbose name and the same type text GetType() used to format.
// Every other value stays unique by ID. The map key reuses those strings.
type phiEdgeIdentityKind uint8

const (
	phiEdgeNil phiEdgeIdentityKind = iota
	phiEdgeConst
	phiEdgeUndefined
	phiEdgeID
)

type phiEdgeIdentity struct {
	kind     phiEdgeIdentityKind
	undef    UndefinedKind
	id       int64
	constStr string
	name     string
	typText  string
}

func phiEdgeDedupeIdentity(v Value) phiEdgeIdentity {
	if utils.IsNil(v) {
		return phiEdgeIdentity{kind: phiEdgeNil}
	}
	switch vv := v.(type) {
	case *ConstInst:
		return phiEdgeIdentity{kind: phiEdgeConst, constStr: vv.String()}
	case *Undefined:
		typText := ""
		if typ := vv.GetType(); typ != nil {
			// String() is what fmt used for %s. Reuse it; do not copy it again.
			typText = typ.String()
		}
		return phiEdgeIdentity{
			kind:    phiEdgeUndefined,
			undef:   vv.Kind,
			name:    vv.GetVerboseName(),
			typText: typText,
		}
	default:
		return phiEdgeIdentity{kind: phiEdgeID, id: v.GetId()}
	}
}

func (s *memberCallReadVisitSet) contains(k memberCallReadVisitKey) bool {
	for i := 0; i < s.length; i++ {
		if s.keys[i] == k {
			return true
		}
	}
	if s.overflow != nil {
		_, ok := s.overflow[k]
		return ok
	}
	return false
}

// push records k and reports whether it landed in the overflow map. The caller
// must pass that result back to pop: a key can legitimately be present both in
// the inline stack (shallower frame) and in the overflow map (deeper frame), so
// pop cannot infer the storage location from the key alone.
func (s *memberCallReadVisitSet) push(k memberCallReadVisitKey) (inOverflow bool) {
	if s.length < memberCallReadVisitStackDepth {
		s.keys[s.length] = k
		s.length++
		return false
	}
	if s.overflow == nil {
		s.overflow = make(map[memberCallReadVisitKey]struct{}, 8)
	}
	s.overflow[k] = struct{}{}
	return true
}

func (s *memberCallReadVisitSet) pop(k memberCallReadVisitKey, inOverflow bool) {
	if inOverflow {
		// Only this frame inserted the overflow entry; the inline copy of k, if
		// any, belongs to a shallower frame and must stay.
		if s.overflow != nil {
			delete(s.overflow, k)
		}
		return
	}
	if s.length > 0 && s.keys[s.length-1] == k {
		s.length--
	}
}

// lookup returns a value this read already produced for k.
func (s *memberCallReadVisitSet) lookup(k memberCallReadVisitKey) (Value, bool) {
	for i := 0; i < s.resultLen; i++ {
		if s.results[i].key == k {
			return s.results[i].value, true
		}
	}
	if s.resultOver != nil {
		value, ok := s.resultOver[k]
		return value, ok
	}
	return nil, false
}

func (s *memberCallReadVisitSet) remember(k memberCallReadVisitKey, value Value) {
	if s.resultLen < memberCallReadVisitStackDepth {
		s.results[s.resultLen] = memberCallReadResult{key: k, value: value}
		s.resultLen++
		return
	}
	if s.resultOver == nil {
		s.resultOver = make(map[memberCallReadVisitKey]Value, memberCallReadVisitStackDepth)
	}
	s.resultOver[k] = value
}

// ReadMemberCallMethodOrValue read member call method or value depends on type
func (b *FunctionBuilder) ReadMemberCallMethodOrValue(object, key Value) Value {
	res := checkCanMemberCallExist(object, key, false)
	if res.exist && res.typ.GetTypeKind() == FunctionTypeKind {
		return b.readMemberCallValueEx(object, key, true)
	}
	return b.readMemberCallValueEx(object, key, false)
}

// read member call variable, want method
func (b *FunctionBuilder) ReadMemberCallMethod(object, key Value) Value {
	return b.readMemberCallValueEx(object, key, true)
}

// read member call variable, want variable
func (b *FunctionBuilder) ReadMemberCallValue(object, key Value) Value {
	return b.readMemberCallValueEx(object, key, false)
}

func (b *FunctionBuilder) ReadMemberCallValueByName(object Value, key string) Value {
	if utils.IsNil(object) {
		return nil
	}
	var nameBuilder strings.Builder
	nameBuilder.Grow(len(key) + 24)
	nameBuilder.WriteByte('#')
	nameBuilder.WriteString(strconv.FormatInt(object.GetId(), 10))
	nameBuilder.WriteByte('.')
	nameBuilder.WriteString(key)
	name := nameBuilder.String()
	if ret := b.PeekValueInThisFunction(name); ret != nil {
		return ret
	}
	return nil
}

func (b *FunctionBuilder) readMemberCallValueEx(object, key Value, wantFunction bool) Value {
	// Stack allocated: the guard lives only for this read and its nested edges.
	var visited memberCallReadVisitSet
	return b.readMemberCallValueExWithVisited(object, key, wantFunction, &visited)
}

func (b *FunctionBuilder) readMemberCallValueExWithVisited(object, key Value, wantFunction bool, visited *memberCallReadVisitSet) Value {
	if res := b.CheckMemberCallNilValue(object, key, "readMemberCallVariableEx"); res != nil {
		return res
	}

	// to extern lib
	if extern, ok := ToExternLib(object); ok {
		return b.TryBuildExternLibValue(extern, key)
	}
	objectt := object
	if se, ok := ToSideEffect(objectt); ok {
		modify, ok := se.GetValueById(se.Value)
		if ok && modify != nil {
			if ref := modify.GetReference(); ref != nil {
				objectt = ref
			}
		}
	}

	// Phi value: read member from each edge and merge as Phi.
	if phi, ok := ToPhi(objectt); ok {
		vk := memberCallReadVisitKey{
			objectID:     objectt.GetId(),
			keyID:        key.GetId(),
			wantFunction: wantFunction,
		}
		if cached, ok := visited.lookup(vk); ok {
			return cached
		}
		if visited.contains(vk) {
			visited.fallbacks++
			return b.getFieldValue(objectt, key, wantFunction)
		}
		fallbacks := visited.fallbacks
		inOverflow := visited.push(vk)
		defer visited.pop(vk, inOverflow)

		res := checkCanMemberCallExist(objectt, key, wantFunction)
		value := b.readPhiMemberValue(objectt, phi, res, key, wantFunction, visited)
		if visited.fallbacks == fallbacks {
			visited.remember(vk, value)
		}
		return value
	}

	// normal member call
	return b.getFieldValue(objectt, key, wantFunction)
}

// readPhiMemberValue resolves one phi member read. The caller has already
// deduplicated this node for the current read and holds its active-path guard.
func (b *FunctionBuilder) readPhiMemberValue(objectt Value, phi *Phi, res checkMemberResult, key Value, wantFunction bool, visited *memberCallReadVisitSet) Value {
	if ret := b.PeekValueInThisFunction(res.name); ret != nil {
		return ret
	}

	edgeValues := make(Values, 0, len(phi.Edge))
	for _, edgeID := range phi.Edge {
		edgeValue, ok := objectt.GetValueById(edgeID)
		if !ok || edgeValue == nil {
			continue
		}
		edgeRes := checkCanMemberCallExist(edgeValue, key, wantFunction)
		if !edgeRes.exist {
			continue
		}
		memberValue := b.readMemberCallValueExWithVisited(edgeValue, key, wantFunction, visited)
		if memberValue != nil {
			edgeValues = append(edgeValues, memberValue)
		}
	}
	if len(edgeValues) > 1 {
		seen := make(map[phiEdgeIdentity]struct{}, len(edgeValues))
		deduped := make(Values, 0, len(edgeValues))
		for _, edgeValue := range edgeValues {
			identity := phiEdgeDedupeIdentity(edgeValue)
			if _, ok := seen[identity]; ok {
				continue
			}
			seen[identity] = struct{}{}
			deduped = append(deduped, edgeValue)
		}
		edgeValues = deduped
	}
	if len(edgeValues) == 0 {
		return b.getFieldValue(objectt, key, wantFunction)
	}
	if len(edgeValues) == 1 {
		// A single resolved edge value would drop the phi's own dataflow
		// sources; defer to the pre-existing field lookup so topdef
		// analysis keeps walking the object edges.
		return b.getFieldValue(objectt, key, wantFunction)
	}
	return b.EmitPhi(res.name, edgeValues)
}

// create member call variable
func (b *FunctionBuilder) CreateMemberCallVariable(object, key Value, cross ...bool) *Variable {
	iscross := false
	if len(cross) > 0 {
		iscross = cross[0]
	}

	createVariable := func(name string) *Variable {
		if iscross {
			return b.CreateVariableCross(name)
		} else {
			return b.CreateVariable(name)
		}
	}

	if utils.IsNil(object) || utils.IsNil(key) {
		log.Errorf("CreateMemberCallVariable: object or key is nil")
		return nil
	}
	// check
	if object.GetId() == -1 || key.GetId() == -1 {
		log.Infof("CreateMemberCallVariable: %v, %v", object.GetName(), key)
	}
	// extern lib
	if extern, ok := ToExternLib(object); ok {
		name := getExternLibMemberCall(object, key)
		ret := createVariable(name)
		ret.SetMemberCall(extern, key)
		return ret
	}
	objectt := object
	if se, ok := ToSideEffect(objectt); ok {
		modify, ok := se.GetValueById(se.Value)
		if ok && modify != nil {
			if ref := modify.GetReference(); ref != nil {
				objectt = ref
			}
		}
	}

	// normal member call
	// name := b.getFieldName(object, key)
	res := checkCanMemberCallExist(objectt, key)
	name := res.name
	if objectt.GetOpcode() != SSAOpcodeParameter {
		// if member not exist, create undefine member in object position
		b.checkAndCreateDefaultMember(res, objectt, key)
	}
	// log.Infof("CreateMemberCallVariable: %v, %v", retValue.GetName(), key)
	ret := createVariable(name)
	ret.SetMemberCall(objectt, key)
	return ret
}
