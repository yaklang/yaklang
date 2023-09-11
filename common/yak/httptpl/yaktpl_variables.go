package httptpl

import (
	"bytes"
	"errors"
	"github.com/yaklang/yaklang/common/log"
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
	Tags []*NucleiTagData
}

func NewVar(v string) *Var {
	val := &Var{Data: v, Type: NucleiDslType}
	if strings.HasPrefix(v, string(FuzztagPrefix)) {
		val.Data = v[len(string(FuzztagPrefix)):]
		val.Type = FuzztagType
	}
	if strings.HasPrefix(v, string(RawPrefix)) {
		val.Data = v[len(string(RawPrefix)):]
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
	v.raw[key] = NewVar(value)
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
		Tags: ParseNucleiTag(value),
	}
}

func (v *YakVariables) AutoSet(key string, value string) {
	v.raw[key] = NewVar(value)
	if v.raw[key].Type == NucleiDslType && strings.Contains(value, "{{") {
		tags := ParseNucleiTag(value)
		v.raw[key].Tags = tags
	}
}

func (v *YakVariables) SetNucleiDSL(key string, items []*NucleiTagData) {
	v.raw[key] = &Var{
		Type: NucleiDslType,
		Tags: items,
	}
}

func (v *YakVariables) ToMap() map[string]interface{} {
	v.outputMutex.Lock()
	defer v.outputMutex.Unlock()

	if v.outputCache != nil {
		return v.outputCache
	}
	result := v.toMap()
	if result != nil {
		v.outputCache = result
	}
	return result
}

func (v *YakVariables) toMap() map[string]interface{} {
	m := make(map[string]interface{})

	var unfinishedVars []string

	count := 0
RETRY:
	for {
		count++
		if count > 100 {
			log.Warnf("vars resolve loop too many times, unfinished vars: %v", unfinishedVars)
			return m
		}

		for _, dep := range unfinishedVars {
			if _, ok := m[dep]; ok {
				continue
			}
			if val, ok := v.raw[dep]; ok {
				if val.Type == NucleiDslType {
					if result, ok, deps := ExecuteNucleiTags(val.Tags, v.nucleiSandbox, m); !ok {
						unfinishedVars = deps
						goto RETRY
					} else {
						m[dep] = result
					}
				} else {
					m[dep] = val.Data
				}
			}
		}

		var unresolved []string
		for k, f := range v.raw {
			if _, ok := m[k]; ok {
				continue
			}

			switch f.Type {
			case NucleiDslType:
				if result, ok, deps := ExecuteNucleiTags(f.Tags, v.nucleiSandbox, m); !ok {
					unresolved = append(unresolved, deps...)
				} else {
					m[k] = result
				}
			default:
				m[k] = f.Data
			}
		}
		unfinishedVars = utils.RemoveRepeatStringSlice(unresolved)
		if len(unresolved) == 0 {
			log.Tracef("fetch count: %d", count)
			return m
		}
	}
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
