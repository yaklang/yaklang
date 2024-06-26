package rule

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"regexp"
	"strings"
)

type OpFlag string

const (
	OpInfo        OpFlag = "info"
	OpData        OpFlag = "data"
	OpExtractData OpFlag = "extract_data"
	OpPush        OpFlag = "push"
	OpOr          OpFlag = "or"
	OpAnd         OpFlag = "and"

	OpNot         OpFlag = "not"
	OpEqual       OpFlag = "equal"
	OpContains    OpFlag = "contains"
	OpRegexpMatch OpFlag = "regexp_match"
)

type OpCode struct {
	Op   OpFlag
	data []any
}

type matchedResult struct {
	ok   bool
	info *FingerprintInfo
}

func Execute(getter func(path string) (*MatchResource, error), codes []*OpCode) (*FingerprintInfo, error) {
	stack := utils.NewStack[any]()
	for i := 0; i < len(codes); i++ {
		code := codes[i]
		getData := func() (*MatchResource, error) {
			if len(code.data) == 0 {
				return nil, fmt.Errorf("no data")
			}
			webPath, ok := code.data[0].(string)
			if !ok {
				return nil, fmt.Errorf("invalid web path: %s", code.data[0])
			}
			res, err := getter(webPath)
			if err != nil {
				return nil, err
			}
			return res, nil
		}
		switch code.Op {
		case OpInfo:
			if v := stack.Pop().(*matchedResult); v.ok {
				if v.info != nil {
					stack.Push(v.info)
				} else {
					stack.Push(code.data[0].(*FingerprintInfo))
				}
			}
		case OpData:
			data, err := getData()
			if err != nil {
				return nil, err
			}
			stack.Push(string(data.Data))
		case OpExtractData:
			resource, err := getData()
			if err != nil {
				return nil, err
			}
			data := resource.Data
			switch code.data[1] {
			case "protocol":
				stack.Push(resource.Protocol)
			case "md5":
				_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(data)
				stack.Push(codec.Md5(body))
			case "header_item":
				stack.Push(lowhttp.GetHTTPPacketHeader(data, utils.InterfaceToString(code.data[2])))
			case "header", "headers":
				header, _ := lowhttp.SplitHTTPHeadersAndBodyFromPacket(data)
				stack.Push(string(header))
			case "body":
				_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(data)
				stack.Push(string(body))
			case "title":
				_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(data)
				title := utils.ExtractTitleFromHTMLTitle(string(body), "")
				stack.Push(title)
			case "server":
				server := lowhttp.GetHTTPPacketHeader(data, "server")
				stack.Push(server)
			case "raw", "banner":
				stack.Push(string(data))
			default:
				return nil, fmt.Errorf("not support var: %v", code.data[1])
			}
		case OpPush:
			stack.Push(code.data[0])
		case OpOr:
			if v := stack.Pop().(*matchedResult); v.ok {
				i += code.data[0].(int) - 1
				stack.Push(v)
			}
		case OpAnd:
			if v := stack.Pop().(*matchedResult); !v.ok {
				i += code.data[0].(int) - 1
				stack.Push(v)
			}
		//case OpJmp:
		//	i = code.data[0].(int) - 1
		//case OpJmpIfTrue:
		//	if stack.Pop().(bool) {
		//		i = code.data[0].(int) - 1
		//	}
		//case OpJmpIfFalse:
		//	if !stack.Pop().(bool) {
		//		i = code.data[0].(int) - 1
		//	}
		case OpEqual:
			d1 := stack.Pop().(string)
			d2 := stack.Pop().(string)
			stack.Push(&matchedResult{ok: d1 == d2})
		case OpContains:
			d := stack.PopN(2)
			d1 := d[1].(string)
			d2 := d[0].(string)
			stack.Push(&matchedResult{ok: strings.Contains(d1, d2)})
		case OpRegexpMatch:
			d := stack.PopN(2)
			d1 := d[1].(string)
			d2 := d[0].(string)
			re := regexp.MustCompile(d2)
			ok := re.MatchString(d1)
			if !ok {
				stack.Push(&matchedResult{ok: false})
				break
			}
			if len(code.data) == 6 {
				info := &FingerprintInfo{}
				res := re.FindAllStringSubmatch(d1, 1)
				getGroup := func(s *string, index int) {
					if index != 0 && len(res) > 0 && index < len(res[0]) {
						*s = res[0][index]
					}
				}
				getGroup(&info.CPE.Vendor, code.data[0].(int))
				getGroup(&info.CPE.Product, code.data[1].(int))
				getGroup(&info.CPE.Version, code.data[2].(int))
				getGroup(&info.CPE.Update, code.data[3].(int))
				getGroup(&info.CPE.Edition, code.data[4].(int))
				getGroup(&info.CPE.Language, code.data[5].(int))
				stack.Push(&matchedResult{ok: true, info: info})
			} else {
				stack.Push(&matchedResult{ok: true})
			}
		}
	}
	if stack.Size() == 0 {
		return nil, nil
	}
	return stack.Pop().(*FingerprintInfo), nil
}
