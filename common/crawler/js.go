package crawler

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strconv"
	"strings"
)

type requestNewTarget struct {
	Method string
	Path   string
	Header []*ypb.KVPair
}

func HandleJS(isHttps bool, req []byte, code string, cb ...func(bool, []byte)) {
	prog := js2ssa.ParseSSA(code, nil)
	js := ssaapi.NewProgram(prog)

	getOriginReq := func() []byte {
		var result = make([]byte, len(req))
		copy(result, req)
		return result
	}

	execCallback := func(t *requestNewTarget) {
		https, newReq, err := NewHTTPRequest(isHttps, getOriginReq(), nil, t.Path)
		if err != nil {
			log.Errorf("new http request failed: %v with: %v", err, spew.Sdump(t))
			return
		}
		newReq = lowhttp.ReplaceHTTPPacketMethod(newReq, t.Method)
		for _, header := range t.Header {
			newReq = lowhttp.ReplaceHTTPPacketHeader(newReq, header.Key, header.Value)
		}

		if len(cb) > 0 {
			for _, c := range cb {
				c(https, newReq)
			}
		} else {
			if https {
				log.Infof(" TLS Conn for %v", strconv.Quote(string(newReq)))
			} else {
				log.Infof("PlainConn For %v", strconv.Quote(string(newReq)))
			}
		}
	}

	js.Ref("XMLHttpRequest").Filter(func(value *ssaapi.Value) bool {
		return value.IsCalled()
	}).ForEach(func(value *ssaapi.Value) {
		target := &requestNewTarget{Method: "GET", Path: ""}
		value.GetCalledBy().ShowWithSource(false).Flat(func(value *ssaapi.Value) ssaapi.Values {
			return value.GetCallReturns()
		}).ShowWithSource(false).Filter(func(value *ssaapi.Value) bool {
			if !value.IsField() {
				return false
			}
			switch value.GetFieldName().GetConstValue() {
			case "open":
				value.GetFieldValues().ForEach(func(value *ssaapi.Value) {
					if value.IsCall() {
						args := value.GetCallArgs()
						if method := args.Get(0); method != nil {
							methodStr := utils.InterfaceToString(method.GetConstValue())
							if ret := strings.ToUpper(methodStr); utils.IsCommonHTTPRequestMethod(ret) {
								target.Method = ret
							}
						}
						if path := args.Get(1); path != nil {
							targetPath := utils.InterfaceToString(path.GetConstValue())
							target.Path = targetPath
						}
					}
				})
			case "setRequestHeader":
				value.GetFieldValues().ForEach(func(value *ssaapi.Value) {
					if !value.IsCall() {
						return
					}
					args := value.GetCallArgs()
					if len(args) > 1 {
						key := utils.InterfaceToString(args.Get(0).GetConstValue())
						val := utils.InterfaceToString(args.Get(1).GetConstValue())
						target.Header = append(target.Header, &ypb.KVPair{
							Key: key, Value: val,
						})
					}
				})
			}
			return false
		})
		execCallback(target)
	})

	// handle fetch
	js.Ref("fetch").GetUsers().Filter(func(value *ssaapi.Value) bool {
		return value.IsCall() || value.IsField()
	}).ShowWithSource(false).ForEach(func(value *ssaapi.Value) {
		switch {
		case value.IsCall():
			target := &requestNewTarget{Method: "GET"}
			args := value.GetCallArgs()
			targetUrl := utils.InterfaceToString(args.Get(0).GetConstValue())
			if targetUrl != "" {
				target.Path = targetUrl
			}
			if opt := args.Get(1); opt.IsMake() {
				// args.GetMakeSliceArgs()
				for _, field := range opt.GetMakeObjectFields() {
					switch field.GetFieldName().GetConstValue() {
					case "method":
						target.Method = utils.InterfaceToString(field.GetLatestFieldValue().GetConstValue())
						if ret := strings.ToUpper(target.Method); !utils.IsCommonHTTPRequestMethod(ret) {
							target.Method = "GET"
						}
					case "headers":
						for _, header := range field.GetLatestFieldValue().GetMakeObjectFields() {
							key := utils.InterfaceToString(header.GetFieldName().GetConstValue())
							val := utils.InterfaceToString(header.GetLatestFieldValue().GetConstValue())
							if strings.HasPrefix(key, "'") || strings.HasPrefix(key, "\"") {
								keyRes, _ := yakunquote.Unquote(key)
								if keyRes == "" {
									key = strings.Trim(key, `"'`)
								} else {
									key = keyRes
								}
							}
							target.Header = append(target.Header, &ypb.KVPair{Key: key, Value: val})
						}
					}
				}
			}
			execCallback(target)
		}
	})
}
