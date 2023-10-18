package fuzztagx

import (
	"fmt"
	standard_parser "github.com/yaklang/yaklang/common/fuzztagx/standard-parser"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"testing"
)

var methods = []*standard_parser.TagMethod{}

func regMethod(name string, method func(s string, yield func(any)) error) {
	methods = append(methods, &standard_parser.TagMethod{
		Name: name,
		Fun: func(s string) ([]*standard_parser.FuzzResult, error) {
			res := []*standard_parser.FuzzResult{}
			err := method(s, func(bs any) {
				res = append(res, standard_parser.NewFuzzResultWithData(bs))
			})
			if err != nil {
				return nil, err
			}
			return res, nil
		},
	})
}
func regMethodWithVerbose(name string, method func(s string, yield func(any, string)) error) {
	methods = append(methods, &standard_parser.TagMethod{
		Name: name,
		Fun: func(s string) ([]*standard_parser.FuzzResult, error) {
			res := []*standard_parser.FuzzResult{}
			err := method(s, func(bs any, v string) {
				res = append(res, standard_parser.NewFuzzResultWithDataVerbose(bs, v))
			})
			if err != nil {
				return nil, err
			}
			return res, nil
		},
	})
}
func newGenerate(code string) (*standard_parser.Generator, error) {
	nodes, err := ParseFuzztag(code)
	if err != nil {
		return nil, err
	}
	return standard_parser.NewGenerator(nodes, methods), nil
}
func TestAdditionInfo(t *testing.T) {
	regMethodWithVerbose("url", func(s string, yield func(any, string)) error {
		yield(codec.EncodeUrlCode(s), fmt.Sprintf("url(%s)", s))
		return nil
	})
	regMethodWithVerbose("base64", func(s string, yield func(any, string)) error {
		yield(codec.EncodeBase64(s), fmt.Sprintf("base64(%s)", s))
		return nil
	})
	regMethodWithVerbose("hex", func(s string, yield func(any, string)) error {
		yield(codec.EncodeToHex(s), fmt.Sprintf("hex(%s)", s))
		return nil
	})
	gener, err := newGenerate("{{hex({{base64({{url(1)}},{{url(2)}},{{url(3)}})}})}}")
	if err != nil {
		t.Fatal(err)
	}
	for gener.Next() {
		res := gener.Result()
		data := res.GetData()
		verbose := res.GetVerbose()
		fmt.Printf("data: %s\nverbose record: %s\n", string(data), verbose)
	}
}
