package httptpl

import (
	"errors"
	"github.com/yaklang/yaklang/common/log"
	"strings"
	"sync"
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
	Data string
}

func NewVar(v string) *Var {
	val := &Var{Data: v, Type: NucleiDslType}
	if strings.HasPrefix(v, string(FuzztagPrefix)) { // 指定为fuzztag类型
		val.Data = v[len(string(FuzztagPrefix)):]
		val.Type = FuzztagType
	} else if strings.HasPrefix(v, string(RawPrefix)) { // 指定为raw类型
		val.Data = v[len(string(RawPrefix)):]
		val.Type = RawType
	} else if strings.Contains(v, "{{") { // 自动类型解析
		val.Type = NucleiDslType
	} else {
		val.Type = RawType
	}
	return val
}
func (v *Var) GetValue() string {
	switch v.Type {
	case FuzztagType:
		return string(FuzztagPrefix) + v.Data
	case RawType:
		return string(RawPrefix) + v.Data
	default:
		return v.Data
	}
}

type YakVariables struct {
	nucleiSandbox         *NucleiDSL
	nucleiSandboxInitOnce *sync.Once
	raw                   map[string]*Var

	exprCache   map[string]any
	outputMutex *sync.Mutex
}

func (v *YakVariables) Set(key string, value string) {
	v.SetWithType(key, value, string(RawType))
}
func (v *YakVariables) SetWithType(key string, value string, typeName string) error {
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
	v.raw[key] = &Var{Data: value, Type: tempType}
	return nil
}

func (v *YakVariables) SetAsNucleiTags(key string, value string) {
	v.outputMutex.Lock()
	defer v.outputMutex.Unlock()
	v.raw[key] = &Var{
		Type: "nuclei-dsl",
		Data: value,
	}
}

func (v *YakVariables) AutoSet(key string, value string) {
	v.outputMutex.Lock()
	defer v.outputMutex.Unlock()
	v.raw[key] = NewVar(value)
}

func (v *YakVariables) SetNucleiDSL(key string, value string) {
	v.outputMutex.Lock()
	defer v.outputMutex.Unlock()
	v.raw[key] = &Var{
		Type: NucleiDslType,
		Data: value,
	}
}
func (v *YakVariables) GetRaw() map[string]*Var {
	return v.raw
}

func (v *YakVariables) ToMap() map[string]any {
	res := map[string]any{}
	if v == nil {
		return nil
	}
	v.outputMutex.Lock()
	defer v.outputMutex.Unlock()

	var getVar, getVarAndWriteCache func(v *Var) (any, error)
	getVar = func(s *Var) (any, error) {
		switch s.Type {
		case NucleiDslType:
			res, err := execNucleiTag(s.Data, nil, func(s string) (string, error) {
				if v, ok := res[s]; ok {
					return toString(v), nil
				}
				if v, ok := v.raw[s]; ok {
					v, err := getVarAndWriteCache(v)
					if err != nil {
						return "", err
					}
					res[s] = toString(v)
					return toString(v), nil
				} else {
					return "", errors.New("not found var " + s)
				}

			}, "")
			if err != nil {
				return nil, err
			}
			return toString(res[0]), err
		case RawType:
			return s.Data, nil
		default:
			return nil, errors.New("unsupported var type")
		}
	}
	getVarAndWriteCache = func(yakVar *Var) (any, error) {
		if yakVar.Type == NucleiDslType {
			if val, ok := v.exprCache[yakVar.Data]; ok {
				return toBytes(val), nil
			}
		}
		res, err := getVar(yakVar)
		if yakVar.Type == NucleiDslType {
			v.exprCache[yakVar.Data] = res
		}
		return res, err
	}
	for k, v := range v.raw {
		if _, ok := res[k]; ok {
			continue
		}
		val, err := getVarAndWriteCache(v)
		if err != nil {
			log.Error(err)
			continue
		}
		res[k] = val
	}
	return res
}

func NewVars() *YakVariables {
	return &YakVariables{
		raw:                   make(map[string]*Var),
		nucleiSandboxInitOnce: new(sync.Once),
		outputMutex:           new(sync.Mutex),
		exprCache:             make(map[string]any),
	}
}
