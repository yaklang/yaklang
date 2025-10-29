package crawler

import (
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type requestNewTarget struct {
	Method string
	Path   string
	Header []*ypb.KVPair
}

func HandleJSGetNewRequest(isHttps bool, req []byte, code string, cb ...func(bool, []byte)) {
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

	handleJS(code, execCallback)
}

func handleJS(code string, callback func(*requestNewTarget)) {
	js, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.JS))
	if err != nil {
		log.Error("parse js failed")
		return
	}

	js.Ref("XMLHttpRequest").GetUsers().Filter(func(value *ssaapi.Value) bool {
		return value.IsCall()
	}).ForEach(func(value *ssaapi.Value) {
		target := &requestNewTarget{Method: "GET", Path: ""}
		value.GetAllMember().Filter(func(value *ssaapi.Value) bool {
			// log.Infof("v: %s, key: %s, key-const: %v", value.String(), value.GetKey().String(), value.GetKey().GetConstValue())
			switch value.GetKey().GetConstValue() {
			case "open":
				log.Infof("open: %v", value.StringWithRange())
				value.GetUsers().Filter(func(v *ssaapi.Value) bool {
					return v.IsCall()
				}).ForEach(func(value *ssaapi.Value) {
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
				})
			case "setRequestHeader":
				log.Infof("set request header: %v", value.StringWithRange())
				value.GetUsers().Filter(func(v *ssaapi.Value) bool {
					return v.IsCall()
				}).ForEach(func(value *ssaapi.Value) {
					args := value.GetCallArgs()
					if len(args) < 2 {
						return
					}
					key := utils.InterfaceToString(args.Get(0).GetConstValue())
					val := utils.InterfaceToString(args.Get(1).GetConstValue())
					target.Header = append(target.Header, &ypb.KVPair{
						Key: key, Value: val,
					})
				})
			}
			return false
		})
		log.Infof("Found param from XMLHttpRequest with API EndPoint: %v", target.Path)
		callback(target)
	})

	// handle fetch
	js.Ref("fetch").GetUsers().Filter(func(value *ssaapi.Value) bool {
		return value.IsCall() || value.IsMember()
	}).ForEach(func(value *ssaapi.Value) {
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
				for _, field := range opt.GetAllMember() {
					switch field.GetKey().GetConstValue() {
					case "method":
						target.Method = utils.InterfaceToString(field.GetConstValue())
						if ret := strings.ToUpper(target.Method); !utils.IsCommonHTTPRequestMethod(ret) {
							target.Method = "GET"
						}
					case "headers":
						for _, header := range field.GetAllMember() {
							key := utils.InterfaceToString(header.GetKey().GetConstValue())
							val := utils.InterfaceToString(header.GetConstValue())
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
			log.Infof("Found param from fetch with API EndPoint: %v", target.Path)
			callback(target)
		}
	})
}
