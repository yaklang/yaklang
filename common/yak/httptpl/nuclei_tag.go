package httptpl

import (
	"errors"
	"fmt"
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
	if n.ExecDSL == nil {
		return nil, errors.New("not set NucleiTag.exec")
	}
	s := string(raw.GetData())
	// payload 直接渲染
	if v, ok := n.Payload[s]; ok {
		if n.AttackMode == "pitchfork" || n.AttackMode == "sync" {
			n.Labels = []string{"1"}
		}
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

// ExecNucleiTag 执行包含tag的字符串
func ExecNucleiTag(raw string, vars map[string]any) (result string, err error) {
	vars["randstr"] = func() string {
		return ksuid.New().String()
	}
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", err)
		}
	}()
	res, err := execNucleiTag(raw, nil, func(s string) (string, error) {
		v, ok := vars[s]
		if !ok {
			return "", errors.New("not found var:" + s)
		}
		switch ret := v.(type) {
		case func() string:
			return ret(), nil
		default:
			return toString(v), nil
		}
	}, "")
	if len(res) == 0 {
		return "", errors.New("generate error")
	}
	return string(res[0]), err
}

// FuzzNucleiTag 使用payload对包含tag的字符串进行fuzz
func FuzzNucleiTag(raw string, vars map[string]any, payload map[string][]string, mode string) (result [][]byte, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", err)
		}
	}()
	return execNucleiTag(raw, payload, func(s string) (string, error) {
		v, ok := vars[s]
		if !ok {
			return "", errors.New("not found var:" + s)
		}
		return toString(v), nil
	}, mode)
}

func execNucleiTag(raw string, payloads map[string][]string, getVar func(s string) (string, error), mode string) ([][]byte, error) {
	nodes, err := parser.Parse(raw,
		parser.NewTagDefine("nucleiTag", "{{", "}}", &NucleiTag{
			AttackMode: mode,
			Payload:    payloads,
			ExecDSL: func(s string) (string, error) {
				res1, err := getVar(s)
				if err == nil {
					return res1, nil
				}
				res, err := NewNucleiDSLYakSandbox().ExecuteWithOnGetVar(s, func(name string) (any, bool) {
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
