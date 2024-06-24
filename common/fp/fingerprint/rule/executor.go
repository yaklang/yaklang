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
	OpJmpIfTrue   OpFlag = "jmp_true"
	OpJmp         OpFlag = "jmp"
	OpJmpIfFalse  OpFlag = "jmp_false"
	OpEqual       OpFlag = "equal"
	OpContains    OpFlag = "contains"
	OpRegexpMatch OpFlag = "regexp_match"
)

type OpCode struct {
	Op   OpFlag
	data []any
}

func Execute(data []byte, codes []*OpCode) (*FingerprintInfo, error) {
	stack := utils.NewStack[any]()
	for i := 0; i < len(codes); i++ {
		code := codes[i]
		switch code.Op {
		case OpInfo:
			if stack.Pop().(bool) {
				stack.Push(code.data[0].(*FingerprintInfo))
			}
		case OpData:
			stack.Push(string(data))
		case OpExtractData:
			switch code.data[0] {
			case "md5":
				stack.Push(codec.Md5(data))
			case "header_item":
				stack.Push(lowhttp.GetHTTPPacketHeader(data, utils.InterfaceToString(code.data[1])))
			case "header":
				header, _ := lowhttp.SplitHTTPHeadersAndBodyFromPacket(data)
				stack.Push(string(header))
			case "body":
				_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(data)
				stack.Push(string(body))
			default:
				return nil, fmt.Errorf("not support var: %v", code.data[0])
			}
		case OpPush:
			stack.Push(code.data[0])
		case OpJmp:
			i = code.data[0].(int) - 1
		case OpJmpIfTrue:
			if stack.Pop().(bool) {
				i = code.data[0].(int) - 1
			}
		case OpJmpIfFalse:
			if !stack.Pop().(bool) {
				i = code.data[0].(int) - 1
			}
		case OpEqual:
			d1 := stack.Pop().(string)
			d2 := stack.Pop().(string)
			stack.Push(d1 == d2)
		case OpContains:
			d := stack.PopN(2)
			d1 := d[1].(string)
			d2 := d[0].(string)
			stack.Push(strings.Contains(d1, d2))
		case OpRegexpMatch:
			d := stack.PopN(2)
			d1 := d[1].(string)
			d2 := d[0].(string)
			stack.Push(regexp.MustCompile(d2).MatchString(d1))
		}
	}
	if stack.Size() == 0 {
		return nil, nil
	}
	return stack.Pop().(*FingerprintInfo), nil
}
