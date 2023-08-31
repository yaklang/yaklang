package httptpl

import (
	"bytes"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type Var struct {
	Type string
	Data string
	Tags []*NucleiTagData
}

type YakVariables struct {
	nucleiSandbox         *NucleiDSL
	nucleiSandboxInitOnce *sync.Once
	raw                   map[string]*Var

	outputCache map[string]interface{}
	outputMutex *sync.Mutex
}

func (v *YakVariables) Set(key string, value string) {
	v.raw[key] = &Var{Data: value}
}
func (v *YakVariables) SetWithType(key string, value string, typeName string) {
	v.raw[key] = &Var{Data: value, Type: typeName}
}

func (v *YakVariables) SetAsNucleiTags(key string, value string) {
	v.raw[key] = &Var{
		Type: "nuclei-dsl",
		Tags: ParseNucleiTag(value),
	}
}

func (v *YakVariables) AutoSet(key string, value string) {
	if strings.Contains(value, "{{") {
		v.raw[key] = &Var{
			Type: "nuclei-dsl",
			Tags: ParseNucleiTag(value),
		}
		return
	}
	v.raw[key] = &Var{Data: value}
}

func (v *YakVariables) SetNucleiDSL(key string, items []*NucleiTagData) {
	v.raw[key] = &Var{
		Type: "nuclei-dsl",
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
				if val.Type == "nuclei-dsl" {
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
			case "nuclei-dsl":
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
