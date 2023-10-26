package httptpl

import (
	"errors"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/fuzztagx/parser"
	"github.com/yaklang/yaklang/common/log"
)

type NucleiTag struct {
	parser.BaseTag
	ExecDSL    func(s string) (string, error)
	Payload    map[string][]string
	AttackMode string
}

func (n *NucleiTag) Exec(raw *parser.FuzzResult, params ...map[string]*parser.TagMethod) ([]*parser.FuzzResult, error) {
	if n.AttackMode == "pitchfork" {
		n.Labels = []string{"1"}
	}
	if n.ExecDSL == nil {
		return nil, errors.New("not set NucleiTag.exec")
	}
	s := string(raw.GetData())
	// payload 直接渲染
	if v, ok := n.Payload[s]; ok {
		result := []*parser.FuzzResult{}
		for _, v1 := range v {
			result = append(result, parser.NewFuzzResultWithData(v1))
		}
		return result, nil
	}
	// var 需要执行
	res, err := n.ExecDSL(s)
	return []*parser.FuzzResult{parser.NewFuzzResultWithData(res)}, err
}
func ExecNucleiTag(raw string, vars map[string]any) (string, error) {
	res, err := execNucleiTag(raw, nil, func(s string) (string, error) {
		v, ok := vars[s]
		if !ok {
			return "", errors.New("not found var:" + s)
		}
		return toString(v), nil
	})
	if len(res) == 0 {
		return "", errors.New("generate error")
	}
	return string(res[0]), err
}

func FuzzNucleiTag(raw string, vars map[string]any, payload map[string][]string, mode string) ([][]byte, error) {
	return execNucleiTag(raw, payload, func(s string) (string, error) {
		v, ok := vars[s]
		if !ok {
			return "", errors.New("not found var:" + s)
		}
		return toString(v), nil
	})
}

func execNucleiTag(raw string, payloads map[string][]string, getVar func(s string) (string, error)) ([][]byte, error) {
	nodes, err := parser.Parse(raw,
		parser.NewTagDefine("nucleiTag", "{{", "}}", &NucleiTag{
			Payload: payloads,
			ExecDSL: func(s string) (string, error) {
				res, err := NewNucleiDSLYakSandbox().ExecuteWithOnGetVar(s, func(name string) (any, bool) {
					if s == "randstr" {
						return ksuid.New().String(), true
					}
					if getVar == nil {
						return nil, false
					}
					res, err := getVar(name)
					if err != nil {
						log.Error(err)
						return nil, false
					}
					return res, true
				})
				return toString(res), err
			},
		}),
	)
	if err != nil {
		return nil, err
	}
	res := [][]byte{}
	gener := parser.NewGenerator(nodes, map[string]*parser.TagMethod{})
	for gener.Next() {
		result := gener.Result()
		res = append(res, result.GetData())
	}
	return res, nil
}

func VariablesToMap(v *YakVariables) map[string][]byte {
	res := map[string][]byte{}
	var getVar func(v *Var) ([]byte, error)
	getVar = func(s *Var) ([]byte, error) {
		switch s.Type {
		case NucleiDslType:
			res, err := execNucleiTag(s.Data, nil, func(s string) (string, error) {
				if v, ok := res[s]; ok {
					return string(v), nil
				}
				v, err := getVar(v.raw[s])
				if err != nil {
					return "", err
				}
				res[s] = v
				return string(v), nil
			})
			if err != nil {
				return nil, err
			}
			return res[0], err
		case RawType:
			return []byte(s.Data), nil
		default:
			return nil, errors.New("unsupported var type")
		}
	}
	for k, v := range v.raw {
		if res[k] != nil {
			continue
		}
		val, err := getVar(v)
		if err != nil {
			log.Error(err)
			continue
		}
		res[k] = val
	}
	return res
}
