package httptpl

import (
	"errors"
	"strings"
	"sync"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type TemplateVarTypePrefix string

const (
	FuzztagPrefix TemplateVarTypePrefix = "@fuzztag"
	RawPrefix     TemplateVarTypePrefix = "@raw"
)

type TemplateVarType string

const (
	FuzztagType       TemplateVarType = "fuzztag"
	RawType           TemplateVarType = "raw"
	NucleiDslType     TemplateVarType = "nuclei-dsl"
	NucleiDynDataType TemplateVarType = "nuclei-dyn-data"
)

type Var struct {
	Type TemplateVarType // 需要在保证nuclei中可以正确解析的情况下，携带类型信息，所以对于除nuclei-dsl类型的变量，在值前增加@raw、@fuzztag标记类型
	Data any
}

func NewVar(v any) *Var {
	val := &Var{Data: v, Type: NucleiDslType}
	switch i := v.(type) {
	case string:
		if strings.HasPrefix(i, string(FuzztagPrefix)) { // 指定为fuzztag类型
			val.Data = i[len(string(FuzztagPrefix)):]
			val.Type = FuzztagType
		} else if strings.HasPrefix(i, string(RawPrefix)) { // 指定为raw类型
			val.Data = i[len(string(RawPrefix)):]
			val.Type = RawType
		} else if strings.Contains(i, "{{") { // 自动类型解析
			val.Type = NucleiDslType
		} else {
			val.Type = RawType
		}
	default:
	}
	return val
}

func (v *Var) GetValue() string {
	switch v.Type {
	case FuzztagType:
		return string(FuzztagPrefix) + codec.AnyToString(v.Data)
	case RawType:
		return string(RawPrefix) + codec.AnyToString(v.Data)
	default:
		return codec.AnyToString(v.Data)
	}
}

type YakVariables struct {
	nucleiSandbox         *NucleiDSL
	nucleiSandboxInitOnce *sync.Once
	raw                   *orderedmap.OrderedMap

	exprKeyCache map[string]struct{}
	exprCache    map[string]any
	outputMutex  *sync.Mutex
}

func (v *YakVariables) Set(key string, value any) {
	if v == nil {
		return
	}
	v.SetWithType(key, value, string(RawType))
}

func (v *YakVariables) SetWithType(key string, value any, typeName string) error {
	if v == nil {
		return nil
	}
	v.outputMutex.Lock()
	defer v.outputMutex.Unlock()
	var tempType TemplateVarType
	switch typeName {
	case string(FuzztagType):
		tempType = FuzztagType
	case string(RawType):
		tempType = RawType
	case string(NucleiDslType):
		tempType = NucleiDslType
	default:
		return errors.New("unknown type")
	}
	v.set(key, &Var{Data: value, Type: tempType})
	return nil
}

func (v *YakVariables) SetAsNucleiTags(key string, value any) {
	if v == nil {
		return
	}
	v.outputMutex.Lock()
	defer v.outputMutex.Unlock()
	v.set(key, &Var{
		Type: "nuclei-dsl",
		Data: value,
	})
}

func (v *YakVariables) AutoSet(key string, value any) {
	if v == nil {
		return
	}
	v.outputMutex.Lock()
	defer v.outputMutex.Unlock()
	v.set(key, NewVar(value))
}

func (v *YakVariables) SetNucleiDSL(key string, value any) {
	if v == nil {
		return
	}
	v.SetAsNucleiTags(key, value)
}

func (v *YakVariables) set(key string, value *Var) {
	v.raw.Set(key, value)
	// remove cache key if exists, so value should be re-calculate
	if _, ok := v.exprKeyCache[key]; ok {
		delete(v.exprKeyCache, key)
	}
}

func (v *YakVariables) isCacheKey(key string) bool {
	if v == nil {
		return false
	}
	_, ok := v.exprKeyCache[key]
	return ok
}

func (v *YakVariables) Keys() []string {
	if v == nil {
		return make([]string, 0)
	}
	return v.raw.Keys()
}

func (v *YakVariables) Foreach(f func(key string, value *Var)) {
	if v == nil {
		return
	}
	v.raw.ForEach(func(key string, iValue any) {
		value, ok := iValue.(*Var)
		if !ok {
			log.Errorf("BUG: nuclei variables not *Vars, but %T", iValue)
			return
		}
		f(key, value)
	})
}

func (v *YakVariables) Get(key string) (*Var, bool) {
	if v == nil {
		return nil, false
	}
	iValue, exists := v.raw.Get(key)
	if !exists {
		return nil, false
	}
	value, ok := iValue.(*Var)
	if !ok {
		log.Errorf("BUG: nuclei variables not *Vars, but %T", iValue)
		return nil, false
	}
	return value, true
}

func (v *YakVariables) Len() int {
	if v == nil {
		return 0
	}
	return v.raw.Len()
}

func (v *YakVariables) ToMap() map[string]any {
	if v == nil {
		return make(map[string]any)
	}
	res := map[string]any{}
	if v == nil {
		return res
	}
	v.outputMutex.Lock()
	defer v.outputMutex.Unlock()

	getVar := func(s *Var) (any, error) {
		switch s.Type {
		case FuzztagType:
			results, err := mutate.FuzzTagExec(s.Data, mutate.Fuzz_WithParams(res))
			if err != nil {
				return s.Data, err
			} else {
				return results, nil
			}
		case NucleiDslType:
			ret, err := execNucleiDSL(codec.AnyToString(s.Data), func(s string) (any, error) {
				if variable, ok := res[s]; ok {
					return variable, nil
				} else {
					return "", errors.New("not found var " + s)
				}
			})
			// fallback to string
			if ret == nil || err != nil {
				rets, err := FuzzNucleiTag(codec.AnyToString(s.Data), res, lo.MapEntries(res, func(key string, value any) (string, []string) {
					return key, []string{toString(value)}
				}), "")
				if err != nil {
					return nil, err
				}
				if len(rets) == 1 {
					return toString(rets[0]), nil
				}
				return lo.Map(rets, func(item []byte, index int) string { return toString(item) }), nil
			}
			return ret, err
		case RawType:
			return s.Data, nil
		default:
			return nil, errors.New("unsupported var type")
		}
	}
	v.raw.ForEach(func(key string, iValue any) {
		value, ok := iValue.(*Var)
		if !ok {
			log.Errorf("BUG: nuclei variables not *Vars, but %T", iValue)
			return
		}

		if v.isCacheKey(key) {
			res[key] = v.exprCache[key]
			return
		}
		val, err := getVar(value)
		if err != nil {
			log.Error(err)
			return
		}

		res[key] = val
		v.exprCache[key] = val
		v.exprKeyCache[key] = struct{}{}
	})

	return res
}

func NewVars() *YakVariables {
	return &YakVariables{
		raw:                   orderedmap.New(),
		nucleiSandboxInitOnce: new(sync.Once),
		outputMutex:           new(sync.Mutex),
		exprKeyCache:          make(map[string]struct{}),
		exprCache:             make(map[string]any),
	}
}
