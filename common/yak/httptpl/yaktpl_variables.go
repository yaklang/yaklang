package httptpl

import (
	"bytes"
	"errors"
	"github.com/yaklang/yaklang/common/utils"
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
	FuzztagType   TemplateVarType = "fuzztag"
	RawType       TemplateVarType = "raw"
	NucleiDslType TemplateVarType = "nuclei-dsl"
)

type Var struct {
	Type TemplateVarType // 需要在保证nuclei中可以正确解析的情况下，携带类型信息，所以对于除nuclei-dsl类型的变量，在值前增加@raw、@fuzztag标记类型
	Data string
	//Tags []*NucleiTagData
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

	outputCache map[string]interface{}
	outputMutex *sync.Mutex
}

func (v *YakVariables) Set(key string, value string) {
	v.SetWithType(key, value, string(RawType))
}
func (v *YakVariables) SetWithType(key string, value string, typeName string) error {
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
	v.raw[key] = &Var{
		Type: "nuclei-dsl",
		Data: value,
	}
}

func (v *YakVariables) AutoSet(key string, value string) {
	v.raw[key] = NewVar(value)
}

func (v *YakVariables) SetNucleiDSL(key string, value string) {
	v.raw[key] = &Var{
		Type: NucleiDslType,
		Data: value,
	}
}
func (v *YakVariables) GetRaw() map[string]*Var {
	return v.raw
}
func (v *YakVariables) ToMap() map[string]interface{} {
	v.outputMutex.Lock()
	defer v.outputMutex.Unlock()

	if v.outputCache != nil {
		return v.outputCache
	}
	res := map[string]any{}
	for k, v := range VariablesToMap(v) {
		res[k] = v
	}
	return res
}

func ExecuteNucleiTags(tags []*NucleiTagData, sandbox *NucleiDSL, vars map[string]interface{}) (string, bool, []string) {
	var buf bytes.Buffer
	var deps []string
	for _, tag := range tags {
		if tag.IsExpr {
			if result, missed := IsExprReady(tag.Content, vars); !result {
				deps = append(deps, missed...)
			} else {
				exprResult, _ := sandbox.Execute(tag.Content, vars)
				buf.WriteString(toString(exprResult))
			}
		} else {
			buf.WriteString(tag.Content)
		}
	}
	if len(deps) > 0 {
		return "", false, utils.RemoveRepeatStringSlice(deps)
	}
	return buf.String(), true, nil
}

func NewVars() *YakVariables {
	return &YakVariables{
		raw:                   make(map[string]*Var),
		nucleiSandboxInitOnce: new(sync.Once),
		outputMutex:           new(sync.Mutex),
	}
}
