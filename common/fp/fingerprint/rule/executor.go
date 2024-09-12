package rule

import (
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/schema"
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

const (
	ConstProtocol = "protocol"
	ConstMd5      = "md5"
	ConstHeader   = "header"
	ConstHeaders  = "headers"
	ConstBody     = "body"
	ConstTitle    = "title"
	ConstServer   = "server"
	ConstBanner   = "banner"
	ConstPort     = "port"
	ConstPath     = "path"
)

type OpCode struct {
	Op   OpFlag
	data []any
}

type matchedResult struct {
	ok         bool
	RebuildCPE func(*schema.CPE)
}

func Execute(getter func(path string) (*MatchResource, error), rule *FingerPrintRule) (cpe *schema.CPE, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = utils.Error(e)
		}
	}()
	codes := rule.ToOpCodes()
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
				info := code.data[0].(*schema.CPE)
				if v.RebuildCPE != nil {
					v.RebuildCPE(info)
				}
				stack.Push(info)
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
			case ConstPort:
				stack.Push(resource.Port)
			case ConstPath:
				stack.Push(resource.Path)
			case ConstProtocol:
				stack.Push(resource.Protocol)
			case ConstMd5:
				_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(data)
				stack.Push(codec.Md5(body))
			case "header_item":
				var vals []string
				lowhttp.SplitHTTPPacket(data, nil, nil, func(line string) string {
					if k, v := lowhttp.SplitHTTPHeader(line); k != "" {
						if strings.Contains(strings.ToLower(k), strings.ToLower(utils.InterfaceToString(code.data[2]))) {
							vals = append(vals, v)
						}
					}
					return line
				})
				stack.Push(vals)
			case ConstHeader, ConstHeaders:
				header, _ := lowhttp.SplitHTTPHeadersAndBodyFromPacket(data)
				stack.Push(string(header))
			case ConstBody:
				_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(data)
				stack.Push(string(body))
			case ConstTitle:
				_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(data)
				title := utils.ExtractTitleFromHTMLTitle(string(body), "")
				stack.Push(title)
			case ConstServer:
				server := lowhttp.GetHTTPPacketHeader(data, "server")
				stack.Push(server)
			case ConstBanner:
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
		case OpEqual:
			d1 := stack.Pop()
			d2 := stack.Pop()
			stack.Push(&matchedResult{ok: d1 == d2})
		case OpContains:
			d := stack.PopN(2)
			d1 := utils.InterfaceToString(d[1])
			d2 := utils.InterfaceToString(d[0])
			stack.Push(&matchedResult{ok: strings.Contains(d1, d2)})
		case OpNot:
			res := stack.Pop().(*matchedResult)
			res.ok = !res.ok
			stack.Push(res)
		case OpRegexpMatch:
			d := stack.PopN(2)
			datas := []string{}
			switch ret := d[1].(type) {
			case string:
				datas = append(datas, ret)
			case []string:
				datas = ret
			default:
				return nil, errors.New("invalid data type")
			}
			pattern := d[0].(string)
			if pattern == "" {
				ok := false
				for _, data := range datas {
					if data != "" {
						ok = true
					}
				}
				stack.Push(&matchedResult{ok: ok})
				continue
			}
			re := regexp.MustCompile(pattern)
			matchOk := false
			var matchedData string
			for _, data := range datas {
				if data == "" {
					continue
				}
				matchOk = re.MatchString(data)
				if !matchOk {
					continue
				}
				matchedData = data
				break
			}
			if matchOk {
				if len(code.data) == 6 {
					stack.Push(&matchedResult{ok: true, RebuildCPE: func(info *schema.CPE) {
						res := re.FindAllStringSubmatch(matchedData, 1)
						getGroup := func(s *string, index int) {
							if index != 0 && len(res) > 0 && index < len(res[0]) {
								*s = res[0][index]
							}
						}
						getGroup(&info.Vendor, code.data[0].(int))
						getGroup(&info.Product, code.data[1].(int))
						getGroup(&info.Version, code.data[2].(int))
						getGroup(&info.Update, code.data[3].(int))
						getGroup(&info.Edition, code.data[4].(int))
						getGroup(&info.Language, code.data[5].(int))
					}})
				} else {
					stack.Push(&matchedResult{ok: true})
				}
			} else {
				stack.Push(&matchedResult{ok: false})
			}
		}
	}
	if stack.Size() == 0 {
		return nil, nil
	}
	return stack.Pop().(*schema.CPE), nil
}
