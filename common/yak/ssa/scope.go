package ssa

import (
	"encoding/json"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
	"strconv"
)

type Scope struct {
	*ssautil.ScopedVersionedTable[Value]
}

type ScopeIF ssautil.ScopedVersionedTableIF[Value]

var _ ssautil.ScopedVersionedTableIF[Value] = (*Scope)(nil)

func NewScope(name string) *Scope {
	s := &Scope{
		ScopedVersionedTable: ssautil.NewRootVersionedTable[Value](name, NewVariable),
	}
	s.SetThis(s)
	return s
}

func (s *Scope) CreateSubScope() ssautil.ScopedVersionedTableIF[Value] {
	scope := &Scope{
		ScopedVersionedTable: s.ScopedVersionedTable.CreateSubScope().(*ssautil.ScopedVersionedTable[Value]),
	}
	scope.SetThis(scope)
	return scope
}

func SyncFromDatabase(s *Scope) error {
	if !s.ShouldSaveToDatabase() {
		return nil
	}

	node, err := ssadb.GetIrScope(s.GetPersistentId())
	if err != nil {
		return utils.Wrapf(err, "failed to get tree node")
	}

	// handle persistent id
	var params = make(map[string]any)
	if err := json.Unmarshal([]byte(node.ExtraInfo), &params); err != nil {
		return utils.Wrapf(err, "failed to unmarshal extra info")
	}

	// load to database
	s.SetScopeLevel(utils.MapGetInt(params, "level"))

	quotedValues := utils.MapGetString(params, "values")
	quotedVariable := utils.MapGetString(params, "variable")
	quotedCaptured := utils.MapGetString(params, "captured")
	quotedIncomingPhi := utils.MapGetString(params, "incomingPhi")
	s.SetSpinRaw(utils.MapGetBool(params, "spin"))
	s.SetParentId(utils.MapGetInt64(params, "parent"))

	values, err := strconv.Unquote(quotedValues)
	if err != nil {
		return utils.Wrapf(err, "unquote values error")
	}
	if gres := gjson.Parse(values); gres.IsObject() {
		gres.ForEach(func(key, value gjson.Result) bool {
			if element := gjson.Parse(value.Raw); element.IsArray() {
				m := omap.NewOrderedMap(make(map[string]ssautil.VersionedIF[Value]))
				for _, versioned := range element.Array() {
					var v ssautil.VersionedIF[Value] = new(ssautil.Versioned[Value])
					if err := v.UnmarshalJSON([]byte(versioned.Raw)); err != nil {
						return false
					}
					if v.GetScope() == nil {
						v.SetScope(s)
					}
					m.Push(v)
				}
				s.GetValues().Set(key.String(), m)
			}
			return true
		})
	}

	variable, err := strconv.Unquote(quotedVariable)
	if err != nil {
		return utils.Wrapf(err, "unquote variable error")
	}
	if gres := gjson.Parse(variable); gres.IsObject() {
		gres.ForEach(func(key, value gjson.Result) bool {
			var v []ssautil.VersionedIF[Value]
			if arr := gjson.Parse(value.Raw); arr.IsArray() {
				for _, result := range arr.Array() {
					var versioned = new(ssautil.Versioned[Value])
					err := json.Unmarshal([]byte(result.Raw), versioned)
					if err != nil {
						return false
					}
					if versioned.GetScope() == nil {
						versioned.SetScope(s)
					}
					v = append(v, versioned)
				}
			}
			var k = new(Value)
			err = json.Unmarshal([]byte(key.Raw), k)
			if err != nil {
				log.Warnf("failed to unmarshal key(T): %v", err)
				return false
			}
			s.GetVariable().Set(*k, v)
			return true
		})
	}

	captured, err := strconv.Unquote(quotedCaptured)
	if err != nil {
		return utils.Wrapf(err, "unquote captured error")
	}
	if gres := gjson.Parse(captured); gres.IsObject() {
		gres.ForEach(func(key, value gjson.Result) bool {
			var v ssautil.VersionedIF[Value] = new(ssautil.Versioned[Value])
			err := json.Unmarshal([]byte(value.Raw), v)
			if err != nil {
				return false
			}
			if v.GetScope() == nil {
				v.SetScope(s)
			}
			s.GetCaptured().Set(key.String(), v)
			return true
		})
	}

	incomingPhi, err := strconv.Unquote(quotedIncomingPhi)
	if err != nil {
		return utils.Wrapf(err, "unquote incomingPhi error")
	}
	if gres := gjson.Parse(incomingPhi); gres.IsObject() {
		gres.ForEach(func(key, value gjson.Result) bool {
			var v ssautil.VersionedIF[Value] = new(ssautil.Versioned[Value])
			err := json.Unmarshal([]byte(value.Raw), &v)
			if err != nil {
				return false
			}
			if v.GetScope() == nil {
				v.SetScope(s)
			}
			s.GetIncomingPhi().Set(key.String(), v)
			return true
		})
	}

	return nil
}

func GetScopeFromIrScopeId(i int64) *Scope {
	node, err := ssadb.GetIrScope(i)
	if err != nil {
		log.Warnf("failed to get ir scope: %v", err)
		return nil
	}
	c := NewScope(node.ProgramName)
	c.SetPersistentId(i)
	if err != nil {
		log.Errorf("failed to sync from database: %v", err)
		return nil
	}

	err = SyncFromDatabase(c)
	if err != nil {
		log.Errorf("failed to sync from database: %v", err)
	}
	return c
}
