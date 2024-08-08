package httptpl

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/fuzztagx/parser"
)

type NucleiTag struct {
	parser.BaseTag
	Variables  map[string]any
	Payload    map[string][]string
	AttackMode string
}

func (n *NucleiTag) Exec(ctx context.Context, raw *parser.FuzzResult, yield func(result *parser.FuzzResult), params map[string]*parser.TagMethod) error {
	s := string(raw.GetData())

	// Variables 直接读取
	if v, ok := n.Variables[s]; ok {
		yield(parser.NewFuzzResultWithData(v))
		return nil
	}
	// 沙箱执行
	res, err := NewNucleiDSLYakSandbox().Execute(s, n.Variables)
	if err == nil && res != nil {
		yield(parser.NewFuzzResultWithData(res))
		return nil
	}
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

	yield(parser.NewFuzzResultWithData("{{" + string(raw.GetData()) + "}}"))
	return nil
}

// QuickFuzzNucleiTag 渲染变量 （只渲染变量不执行）
func QuickFuzzNucleiTag(raw string, vars map[string]any) (result string, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", err)
		}
	}()
	nodes, err := parser.Parse(raw,
		parser.NewTagDefine("nucleiTag", "{{", "}}", &NucleiTag{
			Variables: vars,
		}),
	)
	gener := parser.NewGenerator(nil, nodes, map[string]*parser.TagMethod{})
	gener.Next()
	res := gener.Result()
	return string(res.GetData()), nil
}

// FuzzNucleiTag 使用payload对包含tag的字符串进行fuzz
func FuzzNucleiTag(raw string, vars map[string]any, payload map[string][]string, mode string) (result [][]byte, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", err)
		}
	}()
	nodes, err := parser.Parse(raw,
		parser.NewTagDefine("nucleiTag", "{{", "}}", &NucleiTag{
			Payload:    payload,
			AttackMode: mode,
			Variables:  vars,
		}),
	)
	res := [][]byte{}
	gener := parser.NewGenerator(nil, nodes, map[string]*parser.TagMethod{})
	for gener.Next() {
		result := gener.Result()
		res = append(res, result.GetData())
	}
	return res, nil
}

// ExecNucleiDSL 执行包含tag的字符串
func ExecNucleiDSL(raw string, vars map[string]any) (result any, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", err)
		}
	}()
	res, err := execNucleiDSL(raw, func(s string) (any, error) {
		v, ok := vars[s]
		if !ok {
			return "", errors.New("not found var:" + s)
		}
		return v, nil
	})
	return res, err
}

func execNucleiDSL(raw string, getVar func(s string) (any, error)) (any, error) {
	raw = strings.TrimPrefix(raw, "{{")
	raw = strings.TrimSuffix(raw, "}}")

	res, err := NewNucleiDSLYakSandbox().ExecuteWithOnGetVar(raw, func(name string) (any, bool) {
		if getVar == nil {
			return nil, false
		}
		res, err := getVar(name)
		if err != nil {
			return nil, false
		}
		return res, true
	})
	return res, err
}
