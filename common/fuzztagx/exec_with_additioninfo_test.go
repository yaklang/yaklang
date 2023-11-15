package fuzztagx

import (
	"fmt"
	parser "github.com/yaklang/yaklang/common/fuzztagx/parser"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strings"
	"testing"
)

var methods = []*parser.TagMethod{}

func regMethod(name string, method func(s string, yield func(any)) error) {
	methods = append(methods, &parser.TagMethod{
		Name: name,
		Fun: func(s string) ([]*parser.FuzzResult, error) {
			res := []*parser.FuzzResult{}
			err := method(s, func(bs any) {
				res = append(res, parser.NewFuzzResultWithData(bs))
			})
			if err != nil {
				return nil, err
			}
			return res, nil
		},
	})
}
func regMethodWithVerbose(name string, method func(s string, yield func(any, string)) error) {
	methods = append(methods, &parser.TagMethod{
		Name: name,
		Fun: func(s string) ([]*parser.FuzzResult, error) {
			res := []*parser.FuzzResult{}
			err := method(s, func(bs any, v string) {
				res = append(res, parser.NewFuzzResultWithDataVerbose(bs, v))
			})
			if err != nil {
				return nil, err
			}
			return res, nil
		},
	})
}
func newGenerate(code string) (*parser.Generator, error) {
	nodes, err := ParseFuzztag(code,false)
	if err != nil {
		return nil, err
	}
	table := map[string]*parser.TagMethod{}
	for _, method := range methods {
		table[method.Name] = method
	}
	return parser.NewGenerator(nodes, table), nil
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
	gener, err := newGenerate("{{hex({{base64({{url(1)}}{{url(2)}}{{url(3)}})}})}}")
	if err != nil {
		t.Fatal(err)
	}
	gener.Next()
	res := gener.Result()
	data := res.GetData()
	verbose := strings.Join(res.GetVerbose(), ",")
	fmt.Printf("data: %s\nverbose record: %s\n", string(data), verbose)
	if string(data) != "4a544d784a544d794a544d7a" {
		t.Fatal("get data error")
	}
	if verbose != "hex(JTMxJTMyJTMz),base64(%31%32%33),url(1),url(2),url(3)" {
		t.Fatal("get verbose error")
	}
}
