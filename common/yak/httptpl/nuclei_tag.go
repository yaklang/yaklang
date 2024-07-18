package httptpl

import (
	"context"
	"errors"
	"fmt"

	"github.com/yaklang/yaklang/common/fuzztagx/parser"
	"github.com/yaklang/yaklang/common/log"
)

type NucleiTag struct {
	parser.BaseTag
	GetVar     func(s string) (string, bool)
	ExecDSL    func(s string) (string, error)
	Payload    map[string][]string
	AttackMode string
}

func (n *NucleiTag) Exec(ctx context.Context, raw *parser.FuzzResult, yield func(result *parser.FuzzResult), params map[string]*parser.TagMethod) error {
	// 变量渲染
	if n.GetVar != nil {
		if v, ok := n.GetVar(string(raw.GetData())); ok {
			yield(parser.NewFuzzResultWithData(v))
			return nil
		} else {
			yield(parser.NewFuzzResultWithData("{{" + string(raw.GetData()) + "}}"))
			return nil
		}
	}
	s := string(raw.GetData())
	// payload 直接渲染
	if v, ok := n.Payload[s]; ok {
		if n.AttackMode == "pitchfork" || n.AttackMode == "sync" {
			n.Labels = []string{"1"}
		}
		for _, v1 := range v {
			yield(parser.NewFuzzResultWithData(v1))
		}
		return nil
	}
	// 沙箱执行
	if n.ExecDSL != nil {
		res, err := n.ExecDSL(s)
		if err != nil {
			yield(parser.NewFuzzResultWithData("{{" + string(raw.GetData()) + "}}"))
			return nil
		}
		yield(parser.NewFuzzResultWithData(res))
		return nil
	}
	yield(parser.NewFuzzResultWithData("{{" + string(raw.GetData()) + "}}"))
	return nil
}

// RenderNucleiTagWithVar 渲染变量 （只渲染变量不执行）
func RenderNucleiTagWithVar(raw string, vars map[string]any) (result string, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", err)
		}
	}()
	nodes, err := parser.Parse(raw,
		parser.NewTagDefine("nucleiTag", "{{", "}}", &NucleiTag{
			GetVar: func(s string) (string, bool) {
				if v, ok := vars[s]; ok {
					return toString(v), true
				}
				return "", false
			},
		}),
	)
	gener := parser.NewGenerator(nil, nodes, map[string]*parser.TagMethod{})
	gener.Next()
	res := gener.Result()
	return string(res.GetData()), nil
}

// ExecNucleiTag 执行包含tag的字符串
func ExecNucleiTag(raw string, vars map[string]any) (result string, err error) {
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
				var getVarError error
				res, err := NewNucleiDSLYakSandbox().ExecuteWithOnGetVar(s, func(name string) (any, bool) {
					if getVar == nil {
						return nil, false
					}
					res, err := getVar(name)
					if err != nil {
						getVarError = err
						log.Error(err)
						return nil, false
					}
					return res, true
				})
				if getVarError != nil {
					return "", getVarError
				}
				return toString(res), err
			},
		}),
	)
	if err != nil {
		return nil, err
	}
	res := [][]byte{}
	gener := parser.NewGenerator(nil, nodes, map[string]*parser.TagMethod{})
	for gener.Next() {
		result := gener.Result()
		res = append(res, result.GetData())
	}
	return res, nil
}
