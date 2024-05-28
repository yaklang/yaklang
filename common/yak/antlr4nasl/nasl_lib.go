package antlr4nasl

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"github.com/yaklang/yaklang/common/utils/pingutil"
	utils2 "github.com/yaklang/yaklang/common/yak/antlr4nasl/lib"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"math/rand"
	"net"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/vm"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

var empty = yakvm.NewValue("empty", nil, "empty")

type IpPacket struct {
	Data   string
	Ip_hl  uint8
	Ip_v   uint8
	Ip_tos uint8
	Ip_len uint16
	Ip_id  uint16
	Ip_off uint16
	Ip_ttl uint8
	Ip_p   uint8
	Ip_sum uint16
	Ip_src string
	Ip_dst string
}
type NaslBuildInMethodParam struct {
	mapParams  map[string]*yakvm.Value
	listParams []*yakvm.Value
}

func NewNaslBuildInMethodParam() *NaslBuildInMethodParam {
	return &NaslBuildInMethodParam{mapParams: make(map[string]*yakvm.Value)}
}

var (
	notMatchedArgumentTypeError = utils.Error("Argument error in the function %s()")
	securityLogger              = log.GetLogger("security")
	commonLogger                = log.GetLogger("common")
	errorLogger                 = log.GetLogger("error")
	vendor_version              = ""
)

func genNotMatchedArgumentTypeError(name string) error {
	return utils.Errorf("Argument error in the function %s()", name)
}
func (n *NaslBuildInMethodParam) getParamByNumber(index int, defaultValue ...interface{}) *yakvm.Value {
	if index < len(n.listParams) {
		return n.listParams[index]
	} else {
		if len(defaultValue) != 0 {
			return yakvm.NewAutoValue(defaultValue[0])
		}
		return empty
	}
}
func (n *NaslBuildInMethodParam) getParamByName(name string, defaultValue ...interface{}) *yakvm.Value {
	if v, ok := n.mapParams[name]; ok {
		return v
	} else {
		if len(defaultValue) != 0 {
			return yakvm.NewAutoValue(defaultValue[0])
		}
		return yakvm.GetUndefined()
	}
}
func forEachParams(params *NaslBuildInMethodParam, handle func(value *yakvm.Value)) {
	var item *yakvm.Value
	for i := 0; ; i++ {
		item = params.getParamByNumber(i)
		if item == empty {
			break
		}
		handle(item)
	}
}

type NaslBuildInMethod func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error)

var naslLib map[string]NaslBuildInMethod

func init() {
	naslLib = map[string]NaslBuildInMethod{
		//"sleep": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
		//	n := params.getParamByNumber(0, 0)
		//	time.Sleep(time.Duration(n) * time.Second)
		//},
		"script_name": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			ctx.scriptObj.ScriptName = params.getParamByNumber(0).AsString()
			return nil, nil
		},
		"script_version": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			ctx.scriptObj.Version = params.getParamByNumber(0).AsString()
			return nil, nil
		},
		"script_timeout": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			t := params.getParamByNumber(0, -65535).Int()
			if t == -65535 {
				panic(utils.Errorf("invalid timeout argument: %d", t))
			}
			ctx.scriptObj.Timeout = t
			return nil, nil
		},
		"script_copyright": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			ctx.scriptObj.Copyright = params.getParamByNumber(0).AsString()
			return nil, nil
		},
		"script_category": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			ctx.scriptObj.Category = params.getParamByNumber(0).AsString()
			return nil, nil
		},
		"script_family": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			ctx.scriptObj.Family = params.getParamByNumber(0).AsString()
			return nil, nil
		},
		"script_dependencies": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			deps := params.getParamByNumber(0)
			for i := 1; deps != nil && !deps.IsUndefined(); i++ {
				if !utils.StringArrayContains(ctx.scriptObj.Dependencies, deps.AsString()) {
					ctx.scriptObj.Dependencies = append(ctx.scriptObj.Dependencies, deps.AsString())
				}
				deps = params.getParamByNumber(i)
			}
			return nil, nil
		},
		"script_require_keys": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			var item *yakvm.Value
			for i := 0; item != nil && !item.IsUndefined(); i++ {
				item = params.getParamByNumber(0)
				ctx.scriptObj.RequireKeys = append(ctx.scriptObj.RequireKeys, item.AsString())
			}
			return nil, nil
		},
		"script_mandatory_keys": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			re := params.getParamByNumber(0).AsString()
			splits := strings.Split(re, "=")
			var item *yakvm.Value
			for i := 0; item != nil && !item.IsUndefined(); i++ {
				item = params.getParamByNumber(0)
				if len(splits) > 0 && item.AsString() == splits[0] {
					ctx.scriptObj.MandatoryKeys = append(ctx.scriptObj.MandatoryKeys, re)
					re = ""
				} else {
					ctx.scriptObj.MandatoryKeys = append(ctx.scriptObj.MandatoryKeys, item.AsString())
				}
			}
			if re != "" {
				ctx.scriptObj.MandatoryKeys = append(ctx.scriptObj.MandatoryKeys, re)
			}
			return nil, nil
		},
		"script_require_ports": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			var item *yakvm.Value
			for i := 0; item != nil && !item.IsUndefined(); i++ {
				item = params.getParamByNumber(0)
				ctx.scriptObj.RequirePorts = append(ctx.scriptObj.RequirePorts, item.AsString())
			}
			return nil, nil
		},
		"script_require_udp_ports": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			var item *yakvm.Value
			for i := 0; item != nil && !item.IsUndefined(); i++ {
				item = params.getParamByNumber(0)
				ctx.scriptObj.RequireUdpPorts = append(ctx.scriptObj.RequireUdpPorts, item.AsString())
			}
			return nil, nil
		},
		"script_exclude_keys": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			var item *yakvm.Value
			for i := 0; item != nil && !item.IsUndefined(); i++ {
				item = params.getParamByNumber(0)
				ctx.scriptObj.ExcludeKeys = append(ctx.scriptObj.ExcludeKeys, item.AsString())
			}
			return nil, nil
		},
		"script_add_preference": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			addPreference := func(s1, s2, s3 string) {
				preferences := map[string]interface{}{}
				preferences["name"] = s1
				preferences["type"] = s2
				preferences["value"] = s3
				ctx.scriptObj.Preferences[s1] = preferences
			}
			name := params.getParamByName("name")
			type_ := params.getParamByName("type")
			value := params.getParamByName("value")
			if name.IsUndefined() || type_.IsUndefined() || value.IsUndefined() {
				panic(genNotMatchedArgumentTypeError("script_add_preference"))
			}
			if strings.Contains(strings.ToLower(name.AsString()), "home directory") {
				println()
			}
			addPreference(name.AsString(), type_.AsString(), value.AsString())
			return nil, nil
		},
		"script_get_preference": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			pref := params.getParamByNumber(0)
			if pref.IsUndefined() {
				return nil, genNotMatchedArgumentTypeError("script_get_preference")
			}
			if v, ok := ctx.scriptObj.Preferences[pref.AsString()]; ok {
				return v.(map[string]interface{})["value"], nil
			}
			return nil, nil
		},
		"script_get_preference_file_content": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			ref := params.getParamByNumber(0, "").String()
			if ref == "" {
				return nil, errors.New("Argument error in the function script_get_preference()\nFunction usage is : pref = script_get_preference_file_content(<name>)")
			}
			if v, ok := ctx.scriptObj.Preferences[ref]; ok {
				if v1, ok := v.(map[string]interface{}); ok {
					if v1["type"] == "file" {
						return v1["value"], nil
					}
				} else {
					return nil, utils.Errorf("BUG: Invalid script preferences value type")
				}
			}
			return nil, nil
		},
		// 新版加的函数，只有一个脚本使用
		"script_get_preference_file_location": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `script_get_preference_file_location` is not implement"))
			return nil, nil
		},
		"script_oid": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			ctx.scriptObj.OID = params.getParamByNumber(0).AsString()
			return nil, nil
		},
		"script_cve_id": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			forEachParams(params, func(value *yakvm.Value) {
				ctx.scriptObj.CVE = append(ctx.scriptObj.CVE, value.AsString())
			})
			return nil, nil
		},
		"script_bugtraq_id": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			forEachParams(params, func(value *yakvm.Value) {
				ctx.scriptObj.BugtraqId = append(ctx.scriptObj.BugtraqId, value.Int())
			})
			return nil, nil
		},
		"script_xref": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			name := params.getParamByName("name")
			value := params.getParamByName("value")
			if name.IsUndefined() || value.IsUndefined() {
				return nil, utils.Errorf("<name> or <value> is empty")
			}
			ctx.scriptObj.Xrefs[name.String()] = value.String()
			return nil, nil
		},
		"script_tag": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			name := params.getParamByName("name")
			value := params.getParamByName("value")
			if name.IsUndefined() || value.IsUndefined() {
				return nil, utils.Errorf("<name> or <value> is empty")
			}
			ctx.scriptObj.Tags[name.String()] = value.String()
			return nil, nil
		},
		"vendor_version": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return vendor_version, nil
		},
		"get_preference": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			name := params.getParamByNumber(0)
			if name.IsUndefined() {
				return nil, utils.Error("<name> is empty")
			}
			preference := ctx.scriptObj.Preferences[name.String()]
			return preference, nil
		},
		"safe_checks": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			if v, ok := GlobalPrefs["safe_checks"]; ok {
				return v == "yes", nil
			}
			return false, nil
		},
		"get_script_oid": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return ctx.scriptObj.OID, nil
		},
		"replace_kb_item": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			name := params.getParamByName("name")
			value := params.getParamByName("value")
			if name.IsUndefined() || value.IsUndefined() {
				return nil, utils.Errorf("<name> or <value> is empty")
			}
			return nil, ctx.Kbs.SetKB(name.String(), value.Value)
		},
		"set_kb_item": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			name := params.getParamByName("name")
			value := params.getParamByName("value")
			if name.IsUndefined() || value.IsUndefined() {
				return nil, utils.Errorf("<name> or <value> is empty")
			}
			return nil, ctx.Kbs.SetKB(name.String(), value.Value)
		},
		"get_kb_item": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			name := params.getParamByNumber(0)
			return ctx.Kbs.GetKB(name.String()), nil
		},
		// 返回如果pattern包含*，则返回map，否则返回list
		"get_kb_list": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			name := params.getParamByNumber(0).String()
			if strings.Contains(name, "*") {
				return ctx.Kbs.GetKBByPattern(name), nil
			} else {
				res, _ := vm.NewNaslArray(nil)
				if v := ctx.Kbs.GetKB(name); v != nil {
					res.AddEleToList(0, v)
				}
				return res, nil
			}
		},
		"security_message": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			protocol := params.getParamByName("protocol")
			if protocol.IsUndefined() {
				protocol = params.getParamByName("proto")
			}
			port := params.getParamByName("port", -1)
			data := params.getParamByName("data").AsString()
			if data == "" {
				data = "Success"
			}
			securityLogger.Info(data, port.Int(), protocol.String())
			return nil, nil
		},
		"log_message": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			data := params.getParamByName("data").Value
			naslLogger.Info(data)
			return nil, nil
		},
		"error_message": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			protocol := params.getParamByName("protocol")
			if protocol.IsUndefined() {
				protocol = params.getParamByName("proto")
			}
			port := params.getParamByName("port", -1)
			data := params.getParamByName("data").AsString()
			if data == "" {
				data = "Success"
			}
			naslLogger.Info(data, port.Int(), protocol.String())
			return nil, nil
		},
		"open_sock_tcp": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			timeout := params.getParamByName("timeout", ctx.scriptObj.Timeout*2).Int()
			if timeout <= 0 {
				timeout = 5000
			}
			transport := params.getParamByName("transport", -1).Int()
			if !params.getParamByName("priority").IsUndefined() {
				return nil, utils.Errorf("priority is not support")
			}
			if params.getParamByName("bufsz", -1).Int() != -1 {
				return nil, utils.Errorf("bufsz is not support")
			}
			port := params.getParamByNumber(0, 0).Int()
			if port == 0 {
				return nil, utils.Errorf("port is empty")
			}
			var conn net.Conn

			conn, err := netx.DialTCPTimeout(time.Duration(timeout)*time.Second, utils.HostPort(ctx.Host, port), ctx.Proxies...)
			if err != nil {
				return nil, err
			}
			if transport >= 0 {
				conn = utils.NewDefaultTLSClient(conn)
			}
			return conn, nil
		},
		"open_sock_udp": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			port := params.getParamByNumber(0, 0).Int()
			if port == 0 {
				return nil, utils.Errorf("port is empty")
			}
			conn, err := net.Dial("udp", fmt.Sprintf("%s:%d", ctx.Host, port))
			if err != nil {
				return nil, err
			}
			return conn, nil
		},
		"open_priv_sock_tcp": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `open_priv_sock_tcp` is not implement"))
			return nil, nil
		},
		"open_priv_sock_udp": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `open_priv_sock_udp` is not implement"))
			return nil, nil
		},
		// 需要把net.Conn封装一下，携带error信息
		"socket_get_error": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_get_error` is not implement"))
			return nil, nil
		},
		"recv": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			//length := params.getParamByName("length", -1).Int()
			//min := params.getParamByName("min", -1).Int()
			iconn := params.getParamByName("socket", nil).Value
			timeout := params.getParamByName("timeout", 2).Int()
			max := params.getParamByName("length", -1).Int()
			min := params.getParamByName("min", -1).Int()
			if max == -1 {
				panic("max is empty")
				return nil, utils.Errorf("length is empty")
			}
			if iconn == nil {
				return nil, utils.Errorf("socket is empty")
			}
			//min := params.getParamByName("min", -1).Int()
			conn := iconn.(net.Conn)
			if err := conn.SetReadDeadline(time.Now().Add(time.Second * time.Duration(timeout))); err != nil {
				return "", err
			}
			byt := make([]byte, 1)
			timeoutFlag := 0
			var buf bytes.Buffer
			for {
				n, err := conn.Read(byt)
				if err != nil {
					if err, ok := err.(net.Error); ok && err.Timeout() {
						timeoutFlag++
						if timeoutFlag == 1 && buf.Len() >= min {
							break
						}
						if timeoutFlag > 3 {
							break
						}
						continue
					}
					break
				}
				if n == 0 {
					break
				}
				buf.Write(byt[:n])
				if buf.Len() >= max {
					break
				}
			}
			//log.Infof("recv_line:%v: %s", reflect.ValueOf(iconn).Pointer(), buf.String())
			return buf.String(), nil
		},
		"recv_line": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			iconn := params.getParamByName("socket", nil).Value
			length := params.getParamByName("length", -1).Int()
			if length == -1 {
				length = 4096
			}
			timeout := params.getParamByName("timeout", 5).Int()
			conn := iconn.(net.Conn)
			if err := conn.SetReadDeadline(time.Now().Add(time.Second * time.Duration(timeout))); err != nil {
				return "", err
			}
			byt := make([]byte, 1)
			var buf bytes.Buffer
			flag := 0
			for {
				n, err := conn.Read(byt)
				if err != nil {
					break
				}
				if n == 0 {
					break
				}
				buf.Write(byt[:n])
				if byt[0] == '\r' {
					flag = 1
					continue
				}
				if flag == 1 && byt[0] == '\n' {
					break
				}
				if buf.Len() >= length {
					break
				}
			}
			//log.Infof("recv_line:%v: %s", reflect.ValueOf(iconn).Pointer(), buf.String())
			return buf.String(), nil
		},
		"send": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			iconn := params.getParamByName("socket", nil).Value
			data := params.getParamByName("data").AsString()
			option := params.getParamByName("option", 0)
			length := params.getParamByName("length", 0)
			if ctx.Debug {
				naslLogger.Infof("send data: %s", data)
			}
			data_length := len(data)
			_ = option
			_ = length
			_ = data_length
			if conn, ok := iconn.(net.Conn); ok {
				n, err := conn.Write([]byte(data))
				if err != nil {
					naslLogger.Error(err)
					return 0, nil
				}
				return n, nil
			} else {
				panic(notMatchedArgumentTypeError)
				return nil, notMatchedArgumentTypeError
			}
		},
		"socket_negotiate_ssl": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_negotiate_ssl` is not implement"))
			return nil, nil
		},
		"socket_get_cert": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_get_cert` is not implement"))
			return nil, nil
		},
		"socket_get_ssl_version": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_get_ssl_version` is not implement"))
			return nil, nil
		},
		"socket_get_ssl_ciphersuite": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_get_ssl_ciphersuite` is not implement"))
			return nil, nil
		},
		"socket_get_ssl_session_id": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_get_ssl_session_id` is not implement"))
			return nil, nil
		},
		"socket_cert_verify": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_cert_verify` is not implement"))
			return nil, nil
		},
		"close": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			iconn := params.getParamByNumber(0, nil).Value
			if iconn == nil {
				return nil, nil
			}
			conn := iconn.(net.Conn)
			return conn.Close(), nil
		},
		"join_multicast_group": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `join_multicast_group` is not implement"))
			return nil, nil
		},
		"leave_multicast_group": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `leave_multicast_group` is not implement"))
			return nil, nil
		},
		"get_source_port": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_source_port` is not implement"))
			return nil, nil
		},
		"get_sock_info": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_sock_info` is not implement"))
			return nil, nil
		},
		"cgibin": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			if v, ok := ctx.scriptObj.Preferences["cgi_path"]; ok {
				return v, nil
			} else {
				return "/cgi-bin:/scripts", nil
			}
		},
		"http_open_socket": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			port := params.getParamByNumber(0, -1).Int()
			timeout := params.getParamByName("timeout", ctx.scriptObj.RecvTimeout).Int()
			adderss := fmt.Sprintf("%s:%d", ctx.Host, port)
			n := -1
			if timeout == 0 {
				timeout = 5000
			}
			if v, err := naslLibCall("get_kb_item", ctx, nil, []any{fmt.Sprintf("Transports/TCP/%d", port)}); err != nil {
				if v1, ok := v.(int); ok {
					n = v1
				}
			}
			if n == -1 {
				if netx.IsTLSService(adderss, ctx.Proxies...) {
					n = utils2.OPENVAS_ENCAPS_SSLv2
				} else {
					n = utils2.OPENVAS_ENCAPS_IP
				}
			}
			conn, err := netx.DialTCPTimeout(time.Duration(timeout)*time.Second, utils.HostPort(ctx.Host, port), ctx.Proxies...)
			if err != nil {
				return nil, err
			}
			if n > utils2.OPENVAS_ENCAPS_IP {
				tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionSSL30, ServerName: ctx.Host})
				if err := tlsConn.HandshakeContext(context.Background()); err != nil {
					return nil, err
				} else {
					conn = tlsConn
				}
			}
			if _, err := naslLibCall("set_kb_item", ctx, map[string]interface{}{
				"name":  fmt.Sprintf("Transports/TCP/%d", port),
				"value": int(n),
			}, nil); err != nil {
				return nil, err
			}
			return conn, nil
		},
		"http_head": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			getReq := lowhttp.UrlToGetRequestPacket(fmt.Sprintf("http://%s:%d%s", ctx.Host, params.getParamByName("port", -1).Int(), params.getParamByName("item").String()), nil, false)
			freq, err := mutate.NewFuzzHTTPRequest(getReq)
			if err != nil {
				return nil, err
			}
			results, err := freq.FuzzMethod("HEAD").Results()
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, errors.New("http_head fuzz error")
			}
			return utils.HttpDumpWithBody(results[0], true)
		},
		"http_get": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			res := lowhttp.UrlToGetRequestPacket(fmt.Sprintf("http://%s:%d%s", ctx.Host, params.getParamByName("port", -1).Int(), params.getParamByName("item").String()), nil, false)
			return res, nil
		},
		"http_post": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			getReq := lowhttp.UrlToGetRequestPacket(fmt.Sprintf("http://%s:%d%s", ctx.Host, params.getParamByName("port", -1).Int(), params.getParamByName("item").String()), nil, false)
			freq, err := mutate.NewFuzzHTTPRequest(getReq)
			if err != nil {
				return nil, err
			}
			results, err := freq.FuzzMethod("POST").Results()
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, errors.New("http_head fuzz error")
			}
			return utils.HttpDumpWithBody(results[0], true)
		},
		"http_delete": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			getReq := lowhttp.UrlToGetRequestPacket(fmt.Sprintf("http://%s:%d%s", ctx.Host, params.getParamByName("port", -1).Int(), params.getParamByName("item").String()), nil, false)
			freq, err := mutate.NewFuzzHTTPRequest(getReq)
			if err != nil {
				return nil, err
			}
			results, err := freq.FuzzMethod("DELETE").Results()
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, errors.New("http_head fuzz error")
			}
			return utils.HttpDumpWithBody(results[0], true)
			return nil, nil
		},
		"http_put": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			getReq := lowhttp.UrlToGetRequestPacket(fmt.Sprintf("http://%s:%d%s", ctx.Host, params.getParamByName("port", -1).Int(), params.getParamByName("item").String()), nil, false)
			freq, err := mutate.NewFuzzHTTPRequest(getReq)
			if err != nil {
				return nil, err
			}
			results, err := freq.FuzzMethod("PUT").Results()
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, errors.New("http_head fuzz error")
			}
			return utils.HttpDumpWithBody(results[0], true)
			return nil, nil
		},
		"http_close_socket": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			connV := params.getParamByNumber(0, nil)
			conn := connV.Value
			if v, ok := conn.(net.Conn); ok {
				return nil, v.Close()
			} else {
				panic(notMatchedArgumentTypeError)
				return nil, notMatchedArgumentTypeError
			}
		},
		"add_host_name": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			hostname := params.getParamByName("hostname", "").String()
			source := params.getParamByName("source", "").String()
			if source == "" {
				source = "NASL"
			}
			ctx.scriptObj.Vhosts = append(ctx.scriptObj.Vhosts, &NaslVhost{
				Hostname: hostname,
				Source:   source,
			})
			ctx.Kbs.AddKB("internal/vhosts", strings.ToLower(hostname))
			ctx.Kbs.AddKB(fmt.Sprintf("internal/source/%s", strings.ToLower(hostname)), source)
			return nil, nil
		},
		"get_host_name": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return ctx.Host, nil
		},
		"get_host_names": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return vm.NewNaslArray([]interface{}{ctx.Host})
		},
		"get_host_name_source": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return netx.LookupFirst(params.getParamByName("hostname", "").String()), nil
		},
		"resolve_host_name": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `resolve_host_name` is not implement"))
			return nil, nil
		},
		"get_host_ip": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			//_, _, sIp, err := netutil.Route(time.Duration(ctx.scriptObj.Timeout*2)*time.Second, utils.ExtractHost("8.8.8.8"))
			//if err != nil {
			//	return nil, err
			//}
			//return sIp.String(), nil
			return ctx.Host, nil
		},
		"same_host": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `same_host` is not implement"))
			return nil, nil
		},
		"TARGET_IS_IPV6": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return utils.IsIPv6(ctx.Host), nil
		},
		"get_host_open_port": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			naslLibCall("get_kb_item", ctx, nil, []interface{}{"Ports/tcp/*"})
			return nil, nil
		},
		"get_port_state": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			port := params.getParamByNumber(0).Int()
			if v, ok := ctx.Kbs.data[fmt.Sprintf("Ports/tcp/%d", port)]; ok {
				if v2, ok := v.(int); ok {
					return v2, nil
				}
			}
			return false, nil
		},
		"get_tcp_port_state": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			port := params.getParamByNumber(0, 0).Int()
			isOpen := ctx.Kbs.GetKB(fmt.Sprintf("Ports/%d", port))
			if v, ok := isOpen.(int); ok && v == 1 {
				return true, nil
			}
			isOpen = ctx.Kbs.GetKB(fmt.Sprintf("Ports/tcp/%d", port))
			if v, ok := isOpen.(int); ok && v == 1 {
				return true, nil
			}
			return false, nil
		},
		"get_udp_port_state": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			port := params.getParamByNumber(0, 0).Int()
			isOpen := ctx.Kbs.GetKB(fmt.Sprintf("Ports/udp/%d", port))
			if v, ok := isOpen.(int); ok && v == 1 {
				return true, nil
			}
			return false, nil
		},
		"scanner_add_port": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			port := params.getParamByName("port", -1).Int()
			proto := params.getParamByName("proto", "tcp").String()
			if port > 0 {
				ctx.Kbs.SetKB(fmt.Sprintf("Ports/%s/%d", proto, port), 1)
			}
			return nil, nil
		},
		"scanner_status": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			/* Kept for backward compatibility. */
			return nil, nil
		},
		"scanner_get_port": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `scanner_get_port` is not implement"))
			return nil, nil
		},
		"islocalhost": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return utils.IsLoopback(ctx.Host), nil
		},
		"is_public_addr": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return !utils.IsPrivateIP(net.ParseIP(ctx.Host)), nil
		},

		"islocalnet": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return utils.IsPrivateIP(net.ParseIP(ctx.Host)), nil
		},
		"get_port_transport": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			port := params.getParamByNumber(0, -1).Int()
			if port > 0 {
				k := fmt.Sprintf("Transports/TCP/%d", port)
				v, err := naslLibCall("get_kb_item", ctx, nil, []interface{}{k})
				if err != nil {
					return nil, err
				}
				if v1, ok := v.(int); ok {
					if params.getParamByName("asstring").IntBool() {
						return utils2.GetEncapsName(v1), nil
					} else {
						return v1, nil
					}
				}
			}
			return -1, nil
		},
		"this_host": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return utils.GetLocalIPAddress(), nil
		},
		"this_host_name": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return os.Hostname()
		},
		"string": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			s := ""
			forEachParams(params, func(value *yakvm.Value) {
				s += value.String()
			})
			return s, nil
		},
		"raw_string": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			hexs := []byte{}
			forEachParams(params, func(value *yakvm.Value) {
				hexs = append(hexs, byte(value.Int()))
			})
			return string(hexs), nil
		},
		"strcat": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			s := ""
			forEachParams(params, func(value *yakvm.Value) {
				if value.Value == nil {
					return
				}
				s += value.String()
			})
			return s, nil
		},
		"display": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			s := ""
			forEachParams(params, func(value *yakvm.Value) {
				s += value.String()
			})
			commonLogger.Info(s)
			return nil, nil
		},
		"ord": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return int(params.getParamByNumber(0, 0).String()[0]), nil
		},
		"hex": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `hex` is not implement"))
			return nil, nil
		},
		"hexstr": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return codec.EncodeToHex(params.getParamByNumber(0).Value), nil
		},
		"strstr": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			a := params.getParamByNumber(0, "").String()
			b := params.getParamByNumber(1, "").String()
			index := strings.Index(a, b)
			if index == -1 {
				return nil, nil
			}
			return a[index:], nil
		},
		"ereg": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			pattern := params.getParamByName("pattern").String()
			s := params.getParamByName("string").String()
			matched, err := regexp.MatchString(pattern, s)
			if err != nil {
				return false, err
			}
			return matched, nil
		},
		"ereg_replace": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			icase := params.getParamByName("icase", false).IntBool()
			pattern := params.getParamByName("pattern").String()
			s := params.getParamByName("string").String()
			replace := params.getParamByName("replace").String()
			if s == "<nil>" {
				println()
			}
			if icase {
				pattern = "(?i)" + pattern
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return s, err
			}
			newReplace := ""
			for i := 0; i < len(replace); i++ {
				ch := replace[i]
				if ch == '\\' {
					if i+1 >= len(replace) {
						newReplace += string(ch)
						continue
					}
					nextCh := replace[i+1]
					if nextCh >= '0' && nextCh <= '9' {
						newReplace += "$" + string(nextCh)
						i++
						continue
					}
				}
				newReplace += string(ch)
			}
			return re.ReplaceAllString(s, newReplace), nil
		},
		"egrep": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) { // 返回值应该是匹配内容
			pattern := params.getParamByName("pattern").String()
			s := params.getParamByName("string").String()
			icase := params.getParamByName("icase").IntBool()
			if icase {
				pattern = "(?i)" + pattern
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return "", err
			}
			var res interface{}
			if r := re.FindString(s); r != "" {
				res = r
			} else {
				res = nil
			}
			return res, nil
		},
		"eregmatch": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			pattern := params.getParamByName("pattern").String()
			s := params.getParamByName("string").String()
			icase := params.getParamByName("icase").IntBool()
			if icase {
				pattern = "(?i)" + pattern
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return []string{}, err
			}
			return re.FindStringSubmatch(s), nil
		},
		"match": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			pattern := params.getParamByName("pattern").String()
			s := params.getParamByName("string").String()
			icase := params.getParamByName("icase").IntBool()
			if icase {
				pattern = "(?i)" + pattern
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return false, err
			}
			return re.MatchString(s), nil
		},
		"substr": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			str := params.getParamByNumber(0, "").String()
			start := params.getParamByNumber(1, -1).Int()
			end := params.getParamByNumber(2, -1).Int()
			if start < 0 && end < 0 {
				return nil, utils.Errorf("invalid scope")
			}
			if start < 0 {
				return str[:end], nil
			}
			if end < 0 {
				return str[start:], nil
			}
			if start <= end {
				return str[start:end], nil
			} else {
				return nil, utils.Errorf("end must less than start")
			}
		},
		"insstr": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `insstr` is not implement"))
			return nil, nil
		},
		"tolower": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return strings.ToLower(params.getParamByNumber(0).String()), nil
		},
		"toupper": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return strings.ToUpper(params.getParamByNumber(0).String()), nil
		},
		"crap": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			data := params.getParamByName("data").String()
			length := params.getParamByName("length", -1).Int()
			length2 := params.getParamByNumber(0, -1).Int()
			if length == -1 {
				length = length2
			}
			if length == -1 {
				return nil, errors.New("crap length is invalid")
			}
			for i := 0; i < length; i++ {
				data += data
			}
			return data, nil
		},
		"strlen": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return len(params.getParamByNumber(0).String()), nil
		},
		"split": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			str := params.getParamByNumber(0, "").String()
			sep := params.getParamByName("sep", "").String()
			keep := params.getParamByName("keep").IntBool()
			res := strings.Split(str, sep)
			if keep {
				newRes := make([]string, 0)
				for i, v := range res {
					if v != "" {
						if i == len(res)-1 {
							v = v + sep
						}
						newRes = append(newRes, v)
					}
				}
				res = newRes
			}
			return res, nil
		},
		"chomp": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			s := params.getParamByNumber(0, "").AsString()
			return strings.TrimSpace(s), nil
		},
		"int": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			v, err := strconv.Atoi(params.getParamByNumber(0, "0").String())
			if err != nil {
				panic(err)
			}
			return v, err
		},
		"stridx": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			s := params.getParamByNumber(0).String()
			subs := params.getParamByNumber(1).String()
			start := params.getParamByNumber(2)
			if start.IsInt() {
				s = s[start.Int():]
			}
			return strings.Index(s, subs), nil
		},
		"str_replace": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			a := params.getParamByName("string").String()
			b := params.getParamByName("find").String()
			r := params.getParamByName("replace").String()
			count := params.getParamByName("count", 0).Int()
			if count == 0 {
				return strings.Replace(a, b, r, -1), nil
			} else {
				return strings.Replace(a, b, r, count), nil
			}
		},
		"make_list": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			res := vm.NewEmptyNaslArray()
			i := 0
			forEachParams(params, func(value *yakvm.Value) {
				defer func() { i++ }()
				if value.Value == nil {
					naslLogger.Errorf("nasl_make_list: undefined variable #%d skipped\n", i)
					return
				}
				// 列表的每一个元素添加到新的列表中
				switch ret := value.Value.(type) {
				case *vm.NaslArray: // array类型
					for _, v := range ret.Num_elt {
						if res.AddEleToList(i, v) != nil {
							i++
						}
					}
					for _, v := range ret.Hash_elt {
						if res.AddEleToList(i, v) != nil {
							i++
						}
					}
				default: // int, string, data类型
					if res.AddEleToList(i, value.Value) != nil {
						i++
					}
				}
			})
			return res, nil
		},
		"make_list_unique": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			res, err := naslLibCall("make_list", ctx, nil, nil)
			if err != nil {
				return nil, err
			}

			list := res.(*vm.NaslArray).Num_elt
			set := utils.NewSet(list)
			newArray, err := vm.NewNaslArray(set.List())
			if err != nil {
				return nil, err
			}
			return newArray, nil
		},
		"make_array": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			array := vm.NewEmptyNaslArray()
			iskey := false
			var v interface{}
			v1 := 0
			forEachParams(params, func(value *yakvm.Value) {
				defer func() { v1++ }()
				if !iskey {
					v = value.Value
				} else {
					v2 := value.Value
					switch ret := v2.(type) {
					case []byte, string, int, *vm.NaslArray:
						switch ret2 := v.(type) {
						case int:
							array.AddEleToList(ret2, ret)
						case string, []byte:
							array.AddEleToArray(utils.InterfaceToString(ret2), ret)
						}
					default:
						err := utils.Errorf("make_array: bad value type %s for arg #%d\n", reflect.TypeOf(v2).Kind(), v1)
						naslLogger.Error(err)
					}
				}
				iskey = !iskey
			})
			return array, nil
		},
		"keys": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			array := vm.NewEmptyNaslArray()
			i := 0
			p := params.getParamByNumber(0, nil)
			if p == nil || p.Value == nil {
				return array, nil
			}
			if v, ok := p.Value.(*vm.NaslArray); ok {
				if len(v.Num_elt) > 0 {
					for k := range v.Num_elt {
						array.AddEleToList(i, k)
						i++
					}
				}
				if len(v.Hash_elt) > 0 {
					for k := range v.Hash_elt {
						array.AddEleToList(i, k)
						i++
					}
				}
				return array, nil
			} else {
				return nil, utils.Errorf("keys: bad value type %s for arg #%d\n", reflect.TypeOf(p.Value).Kind(), 0)
			}
		},
		"max_index": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			i := params.getParamByNumber(0, nil).Value
			if i == nil {
				return -1, nil
			}
			switch ret := i.(type) {
			case *vm.NaslArray:
				return ret.GetMaxIdx(), nil
			default:
				return nil, utils.Errorf("max_index: bad value type %s for arg #%d\n", reflect.TypeOf(i).Kind(), 0)
			}
		},
		"sort": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			i := params.getParamByNumber(0, nil).Value
			if i == nil {
				return nil, nil
			}
			switch ret := i.(type) {
			case *vm.NaslArray:
				newArray := ret.Copy()
				sort.Sort(vm.SortableArrayByString(newArray.Num_elt))
				return newArray, nil
			}
			return i, nil
		},
		"unixtime": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return time.Now().Unix(), nil
		},
		"gettimeofday": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `gettimeofday` is not implement"))
			return nil, nil
		},
		"localtime": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `localtime` is not implement"))
			return nil, nil
		},
		"mktime": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `mktime` is not implement"))
			return nil, nil
		},
		"open_sock_kdc": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `open_sock_kdc` is not implement"))
			return nil, nil
		},
		"telnet_init": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `telnet_init` is not implement"))
			return nil, nil
		},
		"ftp_log_in": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `ftp_log_in` is not implement"))
			return nil, nil
		},
		"ftp_get_pasv_port": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `ftp_get_pasv_port` is not implement"))
			return nil, nil
		},
		"start_denial": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `start_denial` is not implement"))
			return nil, nil
		},
		"end_denial": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `end_denial` is not implement"))
			return nil, nil
		},
		"dump_ctxt": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_ctxt` is not implement"))
			return nil, nil
		},
		"typeof": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			v := params.getParamByNumber(0, "").Value
			typeName := reflect.TypeOf(v).Kind().String()
			switch typeName {
			case "slice":
				return "array", nil
			}
			return typeName, nil
		},
		"rand": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return rand.Int(), nil
		},
		"usleep": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			t := params.getParamByNumber(0, 0).Int()
			time.Sleep(time.Duration(t) * time.Microsecond)
			return nil, nil
		},
		"sleep": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			t := params.getParamByNumber(0, 0).Int()
			time.Sleep(time.Duration(t) * time.Second)
			return nil, nil
		},
		"isnull": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return params.getParamByNumber(0).IsUndefined(), nil
		},
		"defined_func": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			_, ok := naslLib[params.getParamByNumber(0).String()]
			return ok, nil
		},
		"forge_ip_packet": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			data := params.getParamByName("data").String()
			ip_hl := params.getParamByName("ip_hl", 5).Int()
			ip_v := params.getParamByName("ip_v", 4).Int()
			ip_tos := params.getParamByName("ip_tos", 0).Int()
			ip_id := params.getParamByName("ip_id", rand.Int()).Int()
			ip_off := params.getParamByName("ip_off", 0).Int()
			ip_ttl := params.getParamByName("ip_ttl", 64).Int()
			ip_p := params.getParamByName("ip_p", 0).Int()
			ip_sum := params.getParamByName("ip_sum", 0).Int()
			ip_src := params.getParamByName("ip_src").String()
			ip_dst := params.getParamByName("ip_dst").String()
			ip_len := params.getParamByName("ip_len", 0).Int()
			ipPacket := &IpPacket{
				Data:   data,
				Ip_hl:  uint8(ip_hl),
				Ip_v:   uint8(ip_v),
				Ip_tos: uint8(ip_tos),
				Ip_id:  uint16(ip_id),
				Ip_off: uint16(ip_off),
				Ip_ttl: uint8(ip_ttl),
				Ip_p:   uint8(ip_p),
				Ip_sum: uint16(ip_sum),
				Ip_src: ip_src,
				Ip_dst: ip_dst,
				Ip_len: uint16(ip_len),
			}
			return ipPacket, nil
		},
		"forge_ipv6_packet": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_ipv6_packet` is not implement"))
			return nil, nil
		},
		"get_ip_element": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_ip_element` is not implement"))
			return nil, nil
		},
		"get_ipv6_element": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_ipv6_element` is not implement"))
			return nil, nil
		},
		"set_ip_elements": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `set_ip_elements` is not implement"))
			return nil, nil
		},
		"set_ipv6_elements": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `set_ipv6_elements` is not implement"))
			return nil, nil
		},
		"insert_ip_options": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `insert_ip_options` is not implement"))
			return nil, nil
		},
		"insert_ipv6_options": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `insert_ipv6_options` is not implement"))
			return nil, nil
		},
		"dump_ip_packet": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_ip_packet` is not implement"))
			return nil, nil
		},
		"dump_ipv6_packet": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_ipv6_packet` is not implement"))
			return nil, nil
		},
		"forge_tcp_packet": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_tcp_packet` is not implement"))
			return nil, nil
		},
		"forge_tcp_v6_packet": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_tcp_v6_packet` is not implement"))
			return nil, nil
		},
		"get_tcp_element": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_tcp_element` is not implement"))
			return nil, nil
		},
		"get_tcp_v6_element": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_tcp_v6_element` is not implement"))
			return nil, nil
		},
		"set_tcp_elements": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `set_tcp_elements` is not implement"))
			return nil, nil
		},
		"set_tcp_v6_elements": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `set_tcp_v6_elements` is not implement"))
			return nil, nil
		},
		"dump_tcp_packet": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_tcp_packet` is not implement"))
			return nil, nil
		},
		"dump_tcp_v6_packet": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_tcp_v6_packet` is not implement"))
			return nil, nil
		},
		"tcp_ping": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `tcp_ping` is not implement"))
			return nil, nil
		},
		"tcp_v6_ping": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `tcp_v6_ping` is not implement"))
			return nil, nil
		},
		"forge_udp_packet": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_udp_packet` is not implement"))
			return nil, nil
		},
		"forge_udp_v6_packet": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_udp_v6_packet` is not implement"))
			return nil, nil
		},
		"get_udp_element": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_udp_element` is not implement"))
			return nil, nil
		},
		"get_udp_v6_element": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_udp_v6_element` is not implement"))
			return nil, nil
		},
		"set_udp_elements": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `set_udp_elements` is not implement"))
			return nil, nil
		},
		"set_udp_v6_elements": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `set_udp_v6_elements` is not implement"))
			return nil, nil
		},
		"dump_udp_packet": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_udp_packet` is not implement"))
			return nil, nil
		},
		"dump_udp_v6_packet": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_udp_v6_packet` is not implement"))
			return nil, nil
		},
		"forge_icmp_packet": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_icmp_packet` is not implement"))
			return nil, nil
		},
		"forge_icmp_v6_packet": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_icmp_v6_packet` is not implement"))
			return nil, nil
		},
		"get_icmp_element": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_icmp_element` is not implement"))
			return nil, nil
		},
		"get_icmp_v6_element": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_icmp_v6_element` is not implement"))
			return nil, nil
		},
		"forge_igmp_packet": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_igmp_packet` is not implement"))
			return nil, nil
		},
		"forge_igmp_v6_packet": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_igmp_v6_packet` is not implement"))
			return nil, nil
		},
		"send_packet": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `send_packet` is not implement"))
			return nil, nil
		},
		"send_v6packet": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `send_v6packet` is not implement"))
			return nil, nil
		},
		"pcap_next": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `pcap_next` is not implement"))
			return nil, nil
		},
		"send_capture": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `send_capture` is not implement"))
			return nil, nil
		},
		"MD2": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `MD2` is not implement"))
			return nil, nil
		},
		"MD4": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `MD4` is not implement"))
			return nil, nil
		},
		"MD5": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `MD5` is not implement"))
			return nil, nil
		},
		"SHA1": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `SHA1` is not implement"))
			return nil, nil
		},
		"SHA256": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `SHA256` is not implement"))
			return nil, nil
		},
		"RIPEMD160": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `RIPEMD160` is not implement"))
			return nil, nil
		},
		"HMAC_MD2": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_MD2` is not implement"))
			return nil, nil
		},
		"HMAC_MD5": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_MD5` is not implement"))
			return nil, nil
		},
		"HMAC_SHA1": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_SHA1` is not implement"))
			return nil, nil
		},
		"HMAC_SHA256": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_SHA256` is not implement"))
			return nil, nil
		},
		"HMAC_SHA384": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_SHA384` is not implement"))
			return nil, nil
		},
		"HMAC_SHA512": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_SHA512` is not implement"))
			return nil, nil
		},
		"HMAC_RIPEMD160": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_RIPEMD160` is not implement"))
			return nil, nil
		},
		"prf_sha256": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `prf_sha256` is not implement"))
			return nil, nil
		},
		"prf_sha384": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `prf_sha384` is not implement"))
			return nil, nil
		},
		"tls1_prf": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `tls1_prf` is not implement"))
			return nil, nil
		},
		"ntlmv2_response": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `ntlmv2_response` is not implement"))
			return nil, nil
		},
		"ntlm2_response": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `ntlm2_response` is not implement"))
			return nil, nil
		},
		"ntlm_response": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `ntlm_response` is not implement"))
			return nil, nil
		},
		"key_exchange": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `key_exchange` is not implement"))
			return nil, nil
		},
		"NTLMv1_HASH": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `NTLMv1_HASH` is not implement"))
			return nil, nil
		},
		"NTLMv2_HASH": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `NTLMv2_HASH` is not implement"))
			return nil, nil
		},
		"nt_owf_gen": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `nt_owf_gen` is not implement"))
			return nil, nil
		},
		"lm_owf_gen": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `lm_owf_gen` is not implement"))
			return nil, nil
		},
		"ntv2_owf_gen": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `ntv2_owf_gen` is not implement"))
			return nil, nil
		},
		"insert_hexzeros": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `insert_hexzeros` is not implement"))
			return nil, nil
		},
		"dec2str": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dec2str` is not implement"))
			return nil, nil
		},
		"get_signature": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_signature` is not implement"))
			return nil, nil
		},
		"get_smb2_signature": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_smb2_signature` is not implement"))
			return nil, nil
		},
		"dh_generate_key": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dh_generate_key` is not implement"))
			return nil, nil
		},
		"bn_random": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `bn_random` is not implement"))
			return nil, nil
		},
		"bn_cmp": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `bn_cmp` is not implement"))
			return nil, nil
		},
		"dh_compute_key": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dh_compute_key` is not implement"))
			return nil, nil
		},
		"rsa_public_encrypt": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `rsa_public_encrypt` is not implement"))
			return nil, nil
		},
		"rsa_private_decrypt": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `rsa_private_decrypt` is not implement"))
			return nil, nil
		},
		"rsa_public_decrypt": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `rsa_public_decrypt` is not implement"))
			return nil, nil
		},
		"bf_cbc_encrypt": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `bf_cbc_encrypt` is not implement"))
			return nil, nil
		},
		"bf_cbc_decrypt": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `bf_cbc_decrypt` is not implement"))
			return nil, nil
		},
		"rc4_encrypt": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `rc4_encrypt` is not implement"))
			return nil, nil
		},
		"aes128_cbc_encrypt": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `aes128_cbc_encrypt` is not implement"))
			return nil, nil
		},
		"aes256_cbc_encrypt": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `aes256_cbc_encrypt` is not implement"))
			return nil, nil
		},
		"aes128_ctr_encrypt": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `aes128_ctr_encrypt` is not implement"))
			return nil, nil
		},
		"aes256_ctr_encrypt": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `aes256_ctr_encrypt` is not implement"))
			return nil, nil
		},
		"aes128_gcm_encrypt": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `aes128_gcm_encrypt` is not implement"))
			return nil, nil
		},
		"aes256_gcm_encrypt": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `aes256_gcm_encrypt` is not implement"))
			return nil, nil
		},
		"des_ede_cbc_encrypt": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `des_ede_cbc_encrypt` is not implement"))
			return nil, nil
		},
		"dsa_do_verify": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dsa_do_verify` is not implement"))
			return nil, nil
		},
		"pem_to_rsa": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `pem_to_rsa` is not implement"))
			return nil, nil
		},
		"pem_to_dsa": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `pem_to_dsa` is not implement"))
			return nil, nil
		},
		"rsa_sign": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `rsa_sign` is not implement"))
			return nil, nil
		},
		"dsa_do_sign": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dsa_do_sign` is not implement"))
			return nil, nil
		},
		"gunzip": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `gunzip` is not implement"))
			return nil, nil
		},
		"gzip": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `gzip` is not implement"))
			return nil, nil
		},
		"DES": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `DES` is not implement"))
			return nil, nil
		},
		//源码里没找到
		"pop3_get_banner": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			port := params.getParamByName("port", -1).Int()
			if port == -1 {
				return nil, fmt.Errorf("port is invalid")
			}
			conn, err := netx.DialTCPTimeout(time.Duration(5)*time.Second, utils.HostPort(ctx.Host, port), ctx.Proxies...)
			if err != nil {
				return nil, fmt.Errorf("connect pop3 server error：%s", err)
			}
			defer conn.Close()
			// 读取服务器响应
			buffer := make([]byte, 1024)
			n, err := conn.Read(buffer)
			if err != nil {
				return nil, fmt.Errorf("read pop3 server error：%s", err)
			}
			// 打印服务器响应
			response := string(buffer[:n])
			return response, nil
		},
		"http_cgi_dirs": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			cgiPath, ok := GlobalPrefs["cgi_path"]
			if ok {
				return []string{cgiPath}, nil
			}
			return []string{}, nil
		},

		"new_preference": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			name := params.getParamByName("name").AsString()
			typ := params.getParamByName("typ").AsString()
			value := params.getParamByName("value").AsString()
			ctx.scriptObj.Preferences[name] = map[string]string{"type": typ, "value": value}
			return nil, nil
		},
		"dump": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			v := make([]interface{}, 0)
			forEachParams(params, func(value *yakvm.Value) {
				v = append(v, value.Value)
			})
			spew.Dump(v...)
			return nil, nil
		},
		"wmi_versioninfo": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return nil, nil
		},
		"smb_versioninfo": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return nil, nil
		},
		"register_host_detail": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			name := params.getParamByName("name", "").String()
			value := params.getParamByName("value", "").Value
			naslLibCall("set_kb_item", ctx, map[string]interface{}{"name": "HostDetails", "value": name}, nil)
			naslLibCall("set_kb_item", ctx, map[string]interface{}{"name": "HostDetails/NVT", "value": ctx.scriptObj.OID}, nil)
			naslLibCall("set_kb_item", ctx, map[string]interface{}{"name": fmt.Sprintf("HostDetails/NVT/%s/%s", ctx.scriptObj.OID, name), "value": value}, nil)
			return nil, nil
		},
		"this_host_mac": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			target := params.getParamByNumber(0, "")
			iface, _, _, err := netutil.Route(5*time.Second, target.String())
			if err != nil {
				return nil, err
			}
			return iface.HardwareAddr.String(), nil
		},
		"pingHost": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			result := pingutil.PingAuto(ctx.Host, "", time.Second*5, ctx.Proxies...)
			return result.Ok, nil
		},
		"call_yak_method": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			var methodName string
			var args []interface{}
			first := true
			forEachParams(params, func(value *yakvm.Value) {
				if first {
					methodName = value.String()
					first = false
				} else {
					args = append(args, value.Value)
				}
			})
			yakEngine := yaklang.New()
			yakEngine.SetVar("params", args)
			code := fmt.Sprintf("result = %s(params...)", methodName)
			err := yakEngine.SafeEval(context.Background(), code)
			if err != nil {
				return nil, utils.Errorf("call yak method `%s` error: %v", methodName, err)
			}
			val, ok := yakEngine.GetVar("result")
			if !ok {
				return nil, nil
			}
			return val, nil
		},
		"plugin_run_find_service": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			//nasl_builitin_find_service.c 写了具体的指纹识别逻辑，和nmap的指纹不同，这里需要转换下
			register_service := func(port int, service string) {
				naslLibCall("set_kb_item", ctx, map[string]interface{}{"name": fmt.Sprintf("Services/%s", service), "value": port}, nil)
			}
			iinfos := ctx.Kbs.GetKB("Host/port_infos")
			if iinfos != nil {
				infos := iinfos.([]*fp.MatchResult)
				for _, info := range infos {
					if !info.IsOpen() {
						continue
					}
					var ServiceName string
					switch info.Fingerprint.ServiceName {
					case "http", "https":
						ServiceName = "www"
					default:
						ServiceName = info.Fingerprint.ServiceName
					}
					register_service(info.Port, ServiceName)
				}
			}
			return nil, nil
		},
		"is_array": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			p := params.getParamByNumber(0)
			if p == nil || p.Value == nil {
				return false, nil
			}
			_, ok := p.Value.(*vm.NaslArray)
			return ok, nil
		},
		"ssh_connect": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			p := params.getParamByNumber(0)
			if p == nil || p.Value == nil {
				return false, nil
			}
			_, ok := p.Value.(*vm.NaslArray)
			return ok, nil
		},
		"http_get_remote_headers": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			port := params.getParamByNumber(0).Int()
			host := ctx.Host
			if port != 80 {
				host = fmt.Sprintf("%s:%d", host, port)
			}
			url := fmt.Sprintf("http://%s/", host)
			resp, err := http.Head(url)
			if err != nil {
				return nil, err
			}
			header, err := utils.HttpDumpWithBody(resp, false)
			if err != nil {
				return nil, err
			}
			return header, nil
		},
		"service_get_ports": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			idefault_port_list := params.getParamByName("default_port_list", "").Value
			default_port_array, ok := idefault_port_list.(*vm.NaslArray)
			if !ok {
				return nil, utils.Errorf("service_get_ports: default_port_list is not array")
			}
			default_port_list := []int{}
			for i := 0; i < default_port_array.GetMaxIdx(); i++ {
				iport := default_port_array.GetElementByNum(i)
				port, ok := iport.(int)
				if !ok {
					continue
				}
				default_port_list = append(default_port_list, port)
			}
			nodefault := params.getParamByName("nodefault", 0).IntBool()
			service := params.getParamByName("proto", "").String()
			ipproto := params.getParamByName("ipproto", "tcp").String()
			var port = -1
			if ipproto == "tcp" {
				p := ctx.Kbs.GetKB(fmt.Sprintf("Services/%s", service))
				if p != nil {
					if p1, ok := p.(int); ok {
						port = p1
					}
				}
			} else {
				p := ctx.Kbs.GetKB(fmt.Sprintf("Services/%s/%s", ipproto, service))
				if p != nil {
					if p1, ok := p.(int); ok {
						port = p1
					}
				}
			}
			if port != -1 {
				return []int{port}, nil
			}
			if nodefault {
				return default_port_list, nil
			} else {
				return -1, nil
			}
		},
		"service_get_port": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			res, err := naslLibCall("service_get_ports", ctx, map[string]interface{}{
				"proto":             params.getParamByName("proto", "").Value,
				"ipproto":           params.getParamByName("ipproto", "").Value,
				"default_port_list": []int{params.getParamByName("default", "").Int()},
				"nodefault":         params.getParamByName("nodefault", 0).IntBool(),
			}, nil)
			if err != nil {
				return nil, err
			}
			ports, ok := res.([]int)
			if !ok {
				return -1, nil
			}
			if len(ports) == 0 {
				return -1, nil
			}
			return ports[0], nil
		},
		"unknownservice_get_port": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return naslLibCall("service_get_port", ctx, map[string]interface{}{
				"proto":     "unknown",
				"ipproto":   params.getParamByName("ipproto", "").Value,
				"default":   params.getParamByName("default", 0).Int(),
				"nodefault": params.getParamByName("nodefault", 0).IntBool(),
			}, nil)
		},
		"unknownservice_get_ports": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return naslLibCall("service_get_ports", ctx, map[string]interface{}{
				"proto":             "unknown",
				"ipproto":           params.getParamByName("ipproto", "").Value,
				"default_port_list": params.getParamByName("default_port_list", "").Value,
				"nodefault":         params.getParamByName("nodefault", 0).IntBool(),
			}, nil)
		},
		"report_vuln_url": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return naslLibCall("http_report_vuln_url", ctx, map[string]interface{}{
				"port":     params.getParamByName("port", 0).Value,
				"url":      params.getParamByName("url", "").Value,
				"url_only": params.getParamByName("url_only", false).Value,
			}, nil)
		},
		//http_report_vuln_url(port: port, url: url1, url_only: TRUE);
		"http_report_vuln_url": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			port := params.getParamByName("port", "").Int()
			url := params.getParamByName("url", "").String()
			url_only := params.getParamByName("url_only", false).IntBool()
			if url_only {
				return fmt.Sprintf("%v%v", utils.HostPort(ctx.Host, port), url), nil
			} else {
				return fmt.Sprintf("detect vul on: %v%v", utils.HostPort(ctx.Host, port), url), nil
			}
		},
		//build_detection_report(app: "OpenMairie Open Foncier", version: version,
		//install: install, cpe: cpe, concluded: vers[0],
		//concludedUrl: concUrl),
		"build_detection_report": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			scriptObj := ctx.scriptObj
			app := params.getParamByName("app", "").String()
			version := params.getParamByName("version", "").String()
			install := params.getParamByName("install", "").String()
			cpe := params.getParamByName("cpe", "").String()
			concluded := params.getParamByName("concluded", "").String()
			riskType := ""
			if v, ok := utils2.ActToChinese[scriptObj.Category]; ok {
				riskType = v
			} else {
				riskType = scriptObj.Category
			}
			source := "[NaslScript] " + scriptObj.ScriptName
			concludedUrl := params.getParamByName("concludedUrl", "").String()
			solution := utils.MapGetString(scriptObj.Tags, "solution")
			summary := utils.MapGetString(scriptObj.Tags, "summary")
			cve := strings.Join(scriptObj.CVE, ", ")
			//xrefStr := ""
			//for k, v := range ctx.scriptObj.Xrefs {
			//	xrefStr += fmt.Sprintf("\n Reference: %s(%s)", v, k)
			//}
			title := fmt.Sprintf("检测目标存在 [%s] 应用，版本号为 [%s]", app, version)
			return fmt.Sprintf(`{"title":"%s","riskType":"%s","source":"%s","concluded":"%s","concludedUrl":"%s","solution":"%s","summary":"%s","cve":"%s","cpe":"%s","install":"%s"}`, title, riskType, source, concluded, concludedUrl, solution, summary, cve, cpe, install), nil
		},
		"ftp_get_banner": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			port := params.getParamByName("port", -1).Int()
			if port == -1 {
				return nil, fmt.Errorf("port is not set")
			}
			banner, err := GetPortBannerByCache(ctx, port)
			if err != nil {
				return nil, err
			}
			return banner, nil
		},
		"telnet_get_banner": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			port := params.getParamByName("port", -1).Int()
			if port == -1 {
				return nil, fmt.Errorf("port is not set")
			}
			banner, err := GetPortBannerByCache(ctx, port)
			if err != nil {
				return nil, err
			}
			return banner, nil
		},
		"smtp_get_banner": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			port := params.getParamByName("port", -1).Int()
			if port == -1 {
				return nil, fmt.Errorf("port is not set")
			}
			banner, err := GetPortBannerByCache(ctx, port)
			if err != nil {
				return nil, err
			}
			return banner, nil
		},
		"imap_get_banner": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			port := params.getParamByName("port", -1).Int()
			if port == -1 {
				return nil, fmt.Errorf("port is not set")
			}
			banner, err := GetPortBannerByCache(ctx, port)
			if err != nil {
				return nil, err
			}
			return banner, nil
		},
		"http_can_host_php": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			ok := false
			rsp, _ := http.Get(fmt.Sprintf("http://%s:%d/index.php", ctx.Host, params.getParamByName("port", 80).Int()))
			if rsp != nil && rsp.StatusCode == 200 {
				ok = true
			}
			if !ok {
				rsp, _ := http.Get(fmt.Sprintf("https://%s:%d/index.php", ctx.Host, params.getParamByName("port", 443).Int()))
				if rsp != nil && rsp.StatusCode == 200 {
					ok = true
				}
			}
			return ok, nil
		},
		"http_can_host_asp": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			ok := false
			rsp, _ := http.Get(fmt.Sprintf("http://%s:%d/index.asp", ctx.Host, params.getParamByName("port", 80).Int()))
			if rsp != nil && rsp.StatusCode == 200 {
				ok = true
			}
			if !ok {
				rsp, _ := http.Get(fmt.Sprintf("https://%s:%d/index.asp", ctx.Host, params.getParamByName("port", 443).Int()))
				if rsp != nil && rsp.StatusCode == 200 {
					ok = true
				}
			}
			return ok, nil
		},
		"http_extract_body_from_response": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			res := params.getParamByName("data", "").String()
			_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket([]byte(res))
			return body, nil
		},
		"os_host_runs": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			if params.getParamByNumber(0, "").String() == runtime.GOOS {
				return true, nil
			}
			return false, nil
		},
		"wmi_file_is_file_search_disabled": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return true, nil
		},
		"snmp_get_port": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return nil, nil
		},
		//需要把ssh相关插件重写
		"ssh_session_id_from_sock": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			panic("not implement")
		},
		"ssh_get_port": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			return nil, nil
		},
		"in_array": func(ctx *ExecContext, params *NaslBuildInMethodParam) (interface{}, error) {
			search := params.getParamByName("search", "").String()
			iArray := params.getParamByName("array", nil).Value
			var array *vm.NaslArray
			if v, ok := iArray.(*vm.NaslArray); ok {
				array = v
			} else {
				panic("param array is not an array")
			}
			_, ok := array.Hash_elt[search]
			return ok, nil
		},
	}
}
func GetExtLib(ctx *ExecContext) map[string]func(params *NaslBuildInMethodParam) interface{} {
	lib := make(map[string]func(params *NaslBuildInMethodParam) interface{})
	for name, method := range naslLib {
		name := name
		method := method
		lib[name] = func(params *NaslBuildInMethodParam) interface{} {
			var res interface{}
			var err error
			timeStart := time.Now()
			if ctx.MethodHook == nil {
				res, err = method(ctx, params)
			} else {
				if v, ok := ctx.MethodHook[name]; ok {
					res, err = v(method, ctx, params)
				} else {
					res, err = method(ctx, params)
				}
			}
			paramstr := ""
			for _, v := range params.listParams {
				paramstr += fmt.Sprintf("%v,", v)
			}
			for k, v := range params.mapParams {
				paramstr += fmt.Sprintf("%s=%v,", k, v)
			}

			if err != nil {
				naslLogger.Errorf("call build in function `%s(%v)` error in script `%v`: %v", name, paramstr, ctx.scriptObj.OriginFileName, err)
				return res
			}
			du := time.Now().Sub(timeStart).Seconds()
			if ctx.Debug && du > 3 {
				naslLogger.Infof("call build in function `%s` cost: %f", name, du)
			}
			if res == nil {
				return res
			}
			switch ret := res.(type) {
			case []byte:
				return string(ret)
			default:
				if reflect.TypeOf(res).Kind() == reflect.Slice || reflect.TypeOf(res).Kind() == reflect.Array {
					if reflect.ValueOf(res).Len() == 0 {
						return nil
					}
				}
				array, err := vm.NewNaslArray(res)
				if err == nil {
					return array
				} else {
					return res
				}
			}
		}
	}
	return lib
}

func GetNaslLibKeys() map[string]interface{} {
	res := make(map[string]interface{})
	for k, _ := range naslLib {
		res[k] = struct {
		}{}
	}
	return res
}

func GetPortBannerByCache(ctx *ExecContext, port int) (string, error) {
	iport_infos, err := naslLibCall("get_kb_item", ctx, map[string]interface{}{"key": "Services/ftp"}, nil)
	if err != nil {
		return "", err
	}
	port_infos := iport_infos.([]*fp.MatchResult)
	for _, port_info := range port_infos {
		if port_info.Port == port {
			return port_info.Fingerprint.Banner, nil
		}
	}
	return "", nil
}

func naslLibCall(name string, ctx *ExecContext, mapParam map[string]interface{}, sliceParam []interface{}) (any, error) {
	if v, ok := naslLib[name]; ok {
		params := NewNaslBuildInMethodParam()
		for _, i1 := range sliceParam {
			params.listParams = append(params.listParams, yakvm.NewAutoValue(i1))
		}
		for k, v := range mapParam {
			params.mapParams[k] = yakvm.NewAutoValue(v)
		}
		return v(ctx, params)
	}
	return nil, fmt.Errorf("not found function: %s", name)
}
