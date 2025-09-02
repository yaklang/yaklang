package script_core

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
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

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"github.com/yaklang/yaklang/common/utils/pingutil"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/executor"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/executor/nasl_type"
	utils2 "github.com/yaklang/yaklang/common/yak/antlr4nasl/lib"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

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

type NaslBuildInMethod func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error)

var naslLib map[string]NaslBuildInMethod

func init() {
	naslLib = map[string]NaslBuildInMethod{
		//"sleep": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
		//	n := params.GetParamByNumber(0, 0)
		//	time.Sleep(time.Duration(n) * time.Second)
		//},
		"script_id": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			ctx.ScriptObj.ScriptID = params.GetParamByNumber(0).Int64()
			return nil, nil
		},
		"script_set_attribute": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			ctx.ScriptObj.ScriptAttributes[params.GetParamByNumber(0).AsString()] = params.GetParamByNumber(1).AsString()
			return nil, nil
		},
		"script_end_attributes": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return nil, nil
		},
		"script_summary": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			ctx.ScriptObj.Summary = params.GetParamByNumber(0).AsString()
			return nil, nil
		},
		"script_name": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			ctx.ScriptObj.ScriptName = params.GetParamByNumber(0).AsString()
			return nil, nil
		},
		"script_version": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			ctx.ScriptObj.Version = params.GetParamByNumber(0).AsString()
			return nil, nil
		},
		"script_timeout": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			t := params.GetParamByNumber(0, -65535).Int()
			if t == -65535 {
				panic(utils.Errorf("invalid timeout argument: %d", t))
			}
			ctx.ScriptObj.Timeout = t
			return nil, nil
		},
		"script_copyright": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			ctx.ScriptObj.Copyright = params.GetParamByNumber(0).AsString()
			return nil, nil
		},
		"script_category": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			ctx.ScriptObj.Category = params.GetParamByNumber(0).AsString()
			return nil, nil
		},
		"script_family": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			ctx.ScriptObj.Family = params.GetParamByNumber(0).AsString()
			return nil, nil
		},
		"script_dependencies": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			deps := params.GetParamByNumber(0)
			for i := 1; deps != nil && !deps.IsUndefined(); i++ {
				if !utils.StringArrayContains(ctx.ScriptObj.Dependencies, deps.AsString()) {
					ctx.ScriptObj.Dependencies = append(ctx.ScriptObj.Dependencies, deps.AsString())
				}
				deps = params.GetParamByNumber(i)
			}
			return nil, nil
		},
		"script_require_keys": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			var item *yakvm.Value
			for i := 0; item != nil && !item.IsUndefined(); i++ {
				item = params.GetParamByNumber(0)
				ctx.ScriptObj.RequireKeys = append(ctx.ScriptObj.RequireKeys, item.AsString())
			}
			return nil, nil
		},
		"script_mandatory_keys": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			key := params.GetParamByNumber(0).AsString()
			if key == "" {
				return nil, errors.New("Lack of key argument")
			}
			reKey := params.GetParamByName("re").AsString()
			splits := strings.Split(reKey, "=")
			var reName string
			if len(splits) == 2 {
				reName = splits[0]
			}

			for i := 1; key != "-"; i++ {
				if key == reName {
					ctx.ScriptObj.MandatoryKeys = append(ctx.ScriptObj.MandatoryKeys, reKey)
					reKey = "-"
					continue
				}
				ctx.ScriptObj.MandatoryKeys = append(ctx.ScriptObj.MandatoryKeys, key)
				key = params.GetParamByNumber(i).AsString()
			}
			if reKey != "-" {
				ctx.ScriptObj.MandatoryKeys = append(ctx.ScriptObj.MandatoryKeys, reKey)
			}
			return nil, nil
		},
		"script_require_ports": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			var item *yakvm.Value
			for i := 0; item != nil && !item.IsUndefined(); i++ {
				item = params.GetParamByNumber(0)
				if item.IsInt() {
					ctx.ScriptObj.RequirePorts = append(ctx.ScriptObj.RequirePorts, item.AsString())
				}
			}
			return nil, nil
		},
		"script_require_udp_ports": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			var item *yakvm.Value
			for i := 0; item != nil && !item.IsUndefined(); i++ {
				item = params.GetParamByNumber(0)
				ctx.ScriptObj.RequireUdpPorts = append(ctx.ScriptObj.RequireUdpPorts, item.AsString())
			}
			return nil, nil
		},
		"script_exclude_keys": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			var item *yakvm.Value
			for i := 0; item != nil && !item.IsUndefined(); i++ {
				item = params.GetParamByNumber(0)
				ctx.ScriptObj.ExcludeKeys = append(ctx.ScriptObj.ExcludeKeys, item.AsString())
			}
			return nil, nil
		},
		"script_add_preference": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			addPreference := func(s1, s2, s3 string) {
				preferences := map[string]interface{}{}
				preferences["name"] = s1
				preferences["type"] = s2
				preferences["value"] = s3
				ctx.ScriptObj.Preferences[s1] = preferences
			}
			name := params.GetParamByName("name")
			type_ := params.GetParamByName("type")
			value := params.GetParamByName("value")
			if name.IsUndefined() || type_.IsUndefined() || value.IsUndefined() {
				panic(genNotMatchedArgumentTypeError("script_add_preference"))
			}
			addPreference(name.AsString(), type_.AsString(), value.AsString())
			return nil, nil
		},
		"script_get_preference": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			pref := params.GetParamByNumber(0)
			if pref.IsUndefined() {
				return nil, genNotMatchedArgumentTypeError("script_get_preference")
			}
			if v, ok := ctx.ScriptObj.Preferences[pref.AsString()]; ok {
				return v.(map[string]interface{})["value"], nil
			}
			return nil, nil
		},
		"script_get_preference_file_content": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			ref := params.GetParamByNumber(0, "").String()
			if ref == "" {
				return nil, errors.New("Argument error in the function script_get_preference()\nFunction usage is : pref = script_get_preference_file_content(<name>)")
			}
			if v, ok := ctx.ScriptObj.Preferences[ref]; ok {
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
		"script_get_preference_file_location": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `script_get_preference_file_location` is not implement"))
			return nil, nil
		},
		"script_oid": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			ctx.ScriptObj.OID = params.GetParamByNumber(0).AsString()
			return nil, nil
		},
		"script_cve_id": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			executor.ForEachParams(params, func(value *yakvm.Value) {
				ctx.ScriptObj.CVE = append(ctx.ScriptObj.CVE, value.AsString())
			})
			return nil, nil
		},
		"script_bugtraq_id": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			executor.ForEachParams(params, func(value *yakvm.Value) {
				ctx.ScriptObj.BugtraqId = append(ctx.ScriptObj.BugtraqId, value.Int())
			})
			return nil, nil
		},
		"script_xref": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			name := params.GetParamByName("name")
			value := params.GetParamByName("value")
			if name.IsUndefined() || value.IsUndefined() {
				return nil, utils.Errorf("<name> or <value> is empty")
			}
			ctx.ScriptObj.Xrefs[name.String()] = value.String()
			return nil, nil
		},
		"script_tag": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			name := params.GetParamByName("name")
			value := params.GetParamByName("value")
			if name.IsUndefined() || value.IsUndefined() {
				return nil, utils.Errorf("<name> or <value> is empty")
			}
			ctx.ScriptObj.Tags[name.String()] = value.String()
			return nil, nil
		},
		"vendor_version": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return vendor_version, nil
		},
		"get_preference": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			name := params.GetParamByNumber(0)
			if name.IsUndefined() {
				return nil, utils.Error("<name> is empty")
			}
			preference := ctx.ScriptObj.Preferences[name.String()]
			return preference, nil
		},
		"safe_checks": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			if v, ok := GlobalPrefs["safe_checks"]; ok {
				return v == "yes", nil
			}
			return false, nil
		},
		"get_script_oid": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return ctx.ScriptObj.OID, nil
		},
		"replace_kb_item": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			name := params.GetParamByName("name")
			value := params.GetParamByName("value")
			if name.IsUndefined() || value.IsUndefined() {
				return nil, utils.Errorf("<name> or <value> is empty")
			}
			return nil, ctx.Kbs.SetKB(name.String(), value.Value)
		},
		"set_kb_item": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			name := params.GetParamByName("name")
			value := params.GetParamByName("value")
			if name.IsUndefined() || value.IsUndefined() {
				return nil, utils.Errorf("<name> or <value> is empty")
			}
			return nil, ctx.Kbs.SetKB(name.String(), value.Value)
		},
		"get_kb_item": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			name := params.GetParamByNumber(0)
			return ctx.Kbs.GetKB(name.String()), nil
		},
		// 返回如果pattern包含*，则返回map，否则返回list
		"get_kb_list": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			name := params.GetParamByNumber(0).String()
			if strings.Contains(name, "*") {
				return ctx.Kbs.GetKBByPattern(name), nil
			} else {
				res, _ := nasl_type.NewNaslArray(nil)
				if v := ctx.Kbs.GetKB(name); v != nil {
					res.AddEleToList(0, v)
				}
				return res, nil
			}
		},
		"security_message": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			protocol := params.GetParamByName("protocol")
			if protocol.IsUndefined() {
				protocol = params.GetParamByName("proto")
			}
			port := params.GetParamByName("port", -1)
			data := params.GetParamByName("data").AsString()
			if data == "" {
				data = "Success"
			}
			securityLogger.Info(data, port.Int(), protocol.String())
			return nil, nil
		},
		"log_message": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			data := params.GetParamByName("data").Value
			naslLogger.Info(data)
			return nil, nil
		},
		"error_message": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			protocol := params.GetParamByName("protocol")
			if protocol.IsUndefined() {
				protocol = params.GetParamByName("proto")
			}
			port := params.GetParamByName("port", -1)
			data := params.GetParamByName("data").AsString()
			if data == "" {
				data = "Success"
			}
			naslLogger.Info(data, port.Int(), protocol.String())
			return nil, nil
		},
		"open_sock_tcp": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			timeout := params.GetParamByName("timeout", ctx.ScriptObj.Timeout*2).Int()
			if timeout <= 0 {
				timeout = 5000
			}
			transport := params.GetParamByName("transport", -1).Int()
			if !params.GetParamByName("priority").IsUndefined() {
				return nil, utils.Errorf("priority is not support")
			}
			if params.GetParamByName("bufsz", -1).Int() != -1 {
				return nil, utils.Errorf("bufsz is not support")
			}
			port := params.GetParamByNumber(0, 0).Int()
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
		"open_sock_udp": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			port := params.GetParamByNumber(0, 0).Int()
			if port == 0 {
				return nil, utils.Errorf("port is empty")
			}
			conn, err := net.Dial("udp", fmt.Sprintf("%s:%d", ctx.Host, port))
			if err != nil {
				return nil, err
			}
			return conn, nil
		},
		"open_priv_sock_tcp": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `open_priv_sock_tcp` is not implement"))
			return nil, nil
		},
		"open_priv_sock_udp": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `open_priv_sock_udp` is not implement"))
			return nil, nil
		},
		// 需要把net.Conn封装一下，携带error信息
		"socket_get_error": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_get_error` is not implement"))
			return nil, nil
		},
		"recv": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			//length := params.getParamByName("length", -1).Int()
			//min := params.getParamByName("min", -1).Int()
			iconn := params.GetParamByName("socket", nil).Value
			timeout := params.GetParamByName("timeout", 2).Int()
			max := params.GetParamByName("length", -1).Int()
			min := params.GetParamByName("min", -1).Int()
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
		"recv_line": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			iconn := params.GetParamByName("socket", nil).Value
			length := params.GetParamByName("length", -1).Int()
			if length == -1 {
				length = 4096
			}
			timeout := params.GetParamByName("timeout", 5).Int()
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
		"send": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			iconn := params.GetParamByName("socket", nil).Value
			data := params.GetParamByName("data").AsString()
			option := params.GetParamByName("option", 0)
			length := params.GetParamByName("length", 0)
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
		"socket_negotiate_ssl": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_negotiate_ssl` is not implement"))
			return nil, nil
		},
		"socket_get_cert": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_get_cert` is not implement"))
			return nil, nil
		},
		"socket_get_ssl_version": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_get_ssl_version` is not implement"))
			return nil, nil
		},
		"socket_get_ssl_ciphersuite": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_get_ssl_ciphersuite` is not implement"))
			return nil, nil
		},
		"socket_get_ssl_session_id": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_get_ssl_session_id` is not implement"))
			return nil, nil
		},
		"socket_cert_verify": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_cert_verify` is not implement"))
			return nil, nil
		},
		"close": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			iconn := params.GetParamByNumber(0, nil).Value
			if iconn == nil {
				return nil, nil
			}
			conn := iconn.(net.Conn)
			return conn.Close(), nil
		},
		"join_multicast_group": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `join_multicast_group` is not implement"))
			return nil, nil
		},
		"leave_multicast_group": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `leave_multicast_group` is not implement"))
			return nil, nil
		},
		"get_source_port": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_source_port` is not implement"))
			return nil, nil
		},
		"get_sock_info": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_sock_info` is not implement"))
			return nil, nil
		},
		"cgibin": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			if v, ok := ctx.ScriptObj.Preferences["cgi_path"]; ok {
				return v, nil
			} else {
				return "/cgi-bin:/scripts", nil
			}
		},
		"http_open_socket": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			port := params.GetParamByNumber(0, -1).Int()
			timeout := params.GetParamByName("timeout", ctx.ScriptObj.RecvTimeout).Int()
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
		"http_head": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			getReq := lowhttp.UrlToGetRequestPacket(fmt.Sprintf("http://%s:%d%s", ctx.Host, params.GetParamByName("port", -1).Int(), params.GetParamByName("item").String()), nil, false)
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
		"http_get": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			res := lowhttp.UrlToGetRequestPacket(fmt.Sprintf("http://%s:%d%s", ctx.Host, params.GetParamByName("port", -1).Int(), params.GetParamByName("item").String()), nil, false)
			return res, nil
		},
		"http_post": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			getReq := lowhttp.UrlToGetRequestPacket(fmt.Sprintf("http://%s:%d%s", ctx.Host, params.GetParamByName("port", -1).Int(), params.GetParamByName("item").String()), nil, false)
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
		"http_delete": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			getReq := lowhttp.UrlToGetRequestPacket(fmt.Sprintf("http://%s:%d%s", ctx.Host, params.GetParamByName("port", -1).Int(), params.GetParamByName("item").String()), nil, false)
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
		"http_put": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			getReq := lowhttp.UrlToGetRequestPacket(fmt.Sprintf("http://%s:%d%s", ctx.Host, params.GetParamByName("port", -1).Int(), params.GetParamByName("item").String()), nil, false)
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
		"http_close_socket": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			connV := params.GetParamByNumber(0, nil)
			conn := connV.Value
			if v, ok := conn.(net.Conn); ok {
				return nil, v.Close()
			} else {
				panic(notMatchedArgumentTypeError)
				return nil, notMatchedArgumentTypeError
			}
		},
		"add_host_name": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			hostname := params.GetParamByName("hostname", "").String()
			source := params.GetParamByName("source", "").String()
			if source == "" {
				source = "NASL"
			}
			ctx.ScriptObj.Vhosts = append(ctx.ScriptObj.Vhosts, &NaslVhost{
				Hostname: hostname,
				Source:   source,
			})
			ctx.Kbs.AddKB("internal/vhosts", strings.ToLower(hostname))
			ctx.Kbs.AddKB(fmt.Sprintf("internal/source/%s", strings.ToLower(hostname)), source)
			return nil, nil
		},
		"get_host_name": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return ctx.Host, nil
		},
		"get_host_names": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return nasl_type.NewNaslArray([]interface{}{ctx.Host})
		},
		"get_host_name_source": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return netx.LookupFirst(params.GetParamByName("hostname", "").String()), nil
		},
		"resolve_host_name": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `resolve_host_name` is not implement"))
			return nil, nil
		},
		"get_host_ip": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			//_, _, sIp, err := netutil.Route(time.Duration(ctx.scriptObj.Timeout*2)*time.Second, utils.ExtractHost("8.8.8.8"))
			//if err != nil {
			//	return nil, err
			//}
			//return sIp.String(), nil
			return ctx.Host, nil
		},
		"same_host": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `same_host` is not implement"))
			return nil, nil
		},
		"TARGET_IS_IPV6": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return utils.IsIPv6(ctx.Host), nil
		},
		"get_host_open_port": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			naslLibCall("get_kb_item", ctx, nil, []interface{}{"Ports/tcp/*"})
			return nil, nil
		},
		"get_port_state": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			port := params.GetParamByNumber(0).Int()
			if v, ok := ctx.Kbs.data[fmt.Sprintf("Ports/tcp/%d", port)]; ok {
				if v2, ok := v.(int); ok {
					return v2, nil
				}
			}
			return false, nil
		},
		"get_tcp_port_state": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			port := params.GetParamByNumber(0, 0).Int()
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
		"get_udp_port_state": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			port := params.GetParamByNumber(0, 0).Int()
			isOpen := ctx.Kbs.GetKB(fmt.Sprintf("Ports/udp/%d", port))
			if v, ok := isOpen.(int); ok && v == 1 {
				return true, nil
			}
			return false, nil
		},
		"scanner_add_port": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			port := params.GetParamByName("port", -1).Int()
			proto := params.GetParamByName("proto", "tcp").String()
			if port > 0 {
				ctx.Kbs.SetKB(fmt.Sprintf("Ports/%s/%d", proto, port), 1)
			}
			return nil, nil
		},
		"scanner_status": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			/* Kept for backward compatibility. */
			return nil, nil
		},
		"scanner_get_port": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `scanner_get_port` is not implement"))
			return nil, nil
		},
		"islocalhost": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return utils.IsLoopback(ctx.Host), nil
		},
		"is_public_addr": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return !utils.IsPrivateIP(net.ParseIP(ctx.Host)), nil
		},

		"islocalnet": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return utils.IsPrivateIP(net.ParseIP(ctx.Host)), nil
		},
		"get_port_transport": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			port := params.GetParamByNumber(0, -1).Int()
			if port > 0 {
				k := fmt.Sprintf("Transports/TCP/%d", port)
				v, err := naslLibCall("get_kb_item", ctx, nil, []interface{}{k})
				if err != nil {
					return nil, err
				}
				if v1, ok := v.(int); ok {
					if params.GetParamByName("asstring").IntBool() {
						return utils2.GetEncapsName(v1), nil
					} else {
						return v1, nil
					}
				}
			}
			return -1, nil
		},
		"this_host": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return utils.GetLocalIPAddress(), nil
		},
		"this_host_name": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return os.Hostname()
		},
		"string": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			s := ""
			executor.ForEachParams(params, func(value *yakvm.Value) {
				s += value.String()
			})
			return s, nil
		},
		"raw_string": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			hexs := []byte{}
			executor.ForEachParams(params, func(value *yakvm.Value) {
				hexs = append(hexs, byte(value.Int()))
			})
			return string(hexs), nil
		},
		"strcat": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			s := ""
			executor.ForEachParams(params, func(value *yakvm.Value) {
				if value.Value == nil {
					return
				}
				s += value.String()
			})
			return s, nil
		},
		"display": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			s := ""
			executor.ForEachParams(params, func(value *yakvm.Value) {
				s += value.String()
			})
			commonLogger.Info(s)
			return nil, nil
		},
		"ord": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return int(params.GetParamByNumber(0, 0).String()[0]), nil
		},
		"hex": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `hex` is not implement"))
			return nil, nil
		},
		"hexstr": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return codec.EncodeToHex(params.GetParamByNumber(0).Value), nil
		},
		"strstr": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			a := params.GetParamByNumber(0, "").String()
			b := params.GetParamByNumber(1, "").String()
			index := strings.Index(a, b)
			if index == -1 {
				return nil, nil
			}
			return a[index:], nil
		},
		"ereg": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			pattern := params.GetParamByName("pattern").String()
			s := params.GetParamByName("string").String()
			matched, err := regexp.MatchString(pattern, s)
			if err != nil {
				return false, err
			}
			return matched, nil
		},
		"ereg_replace": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			icase := params.GetParamByName("icase", false).IntBool()
			pattern := params.GetParamByName("pattern").String()
			s := params.GetParamByName("string").String()
			replace := params.GetParamByName("replace").String()
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
		"egrep": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) { // 返回值应该是匹配内容
			pattern := params.GetParamByName("pattern").String()
			s := params.GetParamByName("string").String()
			icase := params.GetParamByName("icase").IntBool()
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
		"eregmatch": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			pattern := params.GetParamByName("pattern").String()
			s := params.GetParamByName("string").String()
			icase := params.GetParamByName("icase").IntBool()
			if icase {
				pattern = "(?i)" + pattern
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return []string{}, err
			}
			return re.FindStringSubmatch(s), nil
		},
		"match": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			pattern := params.GetParamByName("pattern").String()
			s := params.GetParamByName("string").String()
			icase := params.GetParamByName("icase").IntBool()
			if icase {
				pattern = "(?i)" + pattern
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return false, err
			}
			return re.MatchString(s), nil
		},
		"substr": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			str := params.GetParamByNumber(0, "").String()
			start := params.GetParamByNumber(1, -1).Int()
			end := params.GetParamByNumber(2, -1).Int()
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
		"insstr": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `insstr` is not implement"))
			return nil, nil
		},
		"tolower": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return strings.ToLower(params.GetParamByNumber(0).String()), nil
		},
		"toupper": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return strings.ToUpper(params.GetParamByNumber(0).String()), nil
		},
		"crap": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			data := params.GetParamByName("data").String()
			length := params.GetParamByName("length", -1).Int()
			length2 := params.GetParamByNumber(0, -1).Int()
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
		"strlen": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return len(params.GetParamByNumber(0).String()), nil
		},
		"split": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			str := params.GetParamByNumber(0, "").String()
			sep := params.GetParamByName("sep", "").String()
			keep := params.GetParamByName("keep").IntBool()
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
		"chomp": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			s := params.GetParamByNumber(0, "").AsString()
			return strings.TrimSpace(s), nil
		},
		"int": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			v, err := strconv.Atoi(params.GetParamByNumber(0, "0").String())
			if err != nil {
				panic(err)
			}
			return v, err
		},
		"stridx": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			s := params.GetParamByNumber(0).String()
			subs := params.GetParamByNumber(1).String()
			start := params.GetParamByNumber(2)
			if start.IsInt() {
				s = s[start.Int():]
			}
			return strings.Index(s, subs), nil
		},
		"str_replace": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			a := params.GetParamByName("string").String()
			b := params.GetParamByName("find").String()
			r := params.GetParamByName("replace").String()
			count := params.GetParamByName("count", 0).Int()
			if count == 0 {
				return strings.Replace(a, b, r, -1), nil
			} else {
				return strings.Replace(a, b, r, count), nil
			}
		},
		"make_list": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			res := nasl_type.NewEmptyNaslArray()
			i := 0
			executor.ForEachParams(params, func(value *yakvm.Value) {
				defer func() { i++ }()
				if value.Value == nil {
					naslLogger.Errorf("nasl_make_list: undefined variable #%d skipped\n", i)
					return
				}
				// 列表的每一个元素添加到新的列表中
				switch ret := value.Value.(type) {
				case *nasl_type.NaslArray: // array类型
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
		"make_list_unique": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			res, err := naslLibCall("make_list", ctx, nil, nil)
			if err != nil {
				return nil, err
			}

			list := res.(*nasl_type.NaslArray).Num_elt
			set := utils.NewSet(list)
			newArray, err := nasl_type.NewNaslArray(set.List())
			if err != nil {
				return nil, err
			}
			return newArray, nil
		},
		"make_array": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			array := nasl_type.NewEmptyNaslArray()
			iskey := false
			var v interface{}
			v1 := 0
			executor.ForEachParams(params, func(value *yakvm.Value) {
				defer func() { v1++ }()
				if !iskey {
					v = value.Value
				} else {
					v2 := value.Value
					switch ret := v2.(type) {
					case []byte, string, int, *nasl_type.NaslArray:
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
		"keys": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			array := nasl_type.NewEmptyNaslArray()
			i := 0
			p := params.GetParamByNumber(0, nil)
			if p == nil || p.Value == nil {
				return array, nil
			}
			if v, ok := p.Value.(*nasl_type.NaslArray); ok {
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
		"max_index": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			i := params.GetParamByNumber(0, nil).Value
			if i == nil {
				return -1, nil
			}
			switch ret := i.(type) {
			case *nasl_type.NaslArray:
				return ret.GetMaxIdx(), nil
			default:
				return nil, utils.Errorf("max_index: bad value type %s for arg #%d\n", reflect.TypeOf(i).Kind(), 0)
			}
		},
		"sort": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			i := params.GetParamByNumber(0, nil).Value
			if i == nil {
				return nil, nil
			}
			switch ret := i.(type) {
			case *nasl_type.NaslArray:
				newArray := ret.Copy()
				sort.Sort(nasl_type.SortableArrayByString(newArray.Num_elt))
				return newArray, nil
			}
			return i, nil
		},
		"unixtime": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return time.Now().Unix(), nil
		},
		"gettimeofday": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `gettimeofday` is not implement"))
			return nil, nil
		},
		"localtime": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `localtime` is not implement"))
			return nil, nil
		},
		"mktime": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `mktime` is not implement"))
			return nil, nil
		},
		"open_sock_kdc": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `open_sock_kdc` is not implement"))
			return nil, nil
		},
		"telnet_init": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `telnet_init` is not implement"))
			return nil, nil
		},
		"ftp_log_in": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `ftp_log_in` is not implement"))
			return nil, nil
		},
		"ftp_get_pasv_port": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `ftp_get_pasv_port` is not implement"))
			return nil, nil
		},
		"start_denial": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `start_denial` is not implement"))
			return nil, nil
		},
		"end_denial": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `end_denial` is not implement"))
			return nil, nil
		},
		"dump_ctxt": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_ctxt` is not implement"))
			return nil, nil
		},
		"typeof": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			v := params.GetParamByNumber(0, "").Value
			typeName := reflect.TypeOf(v).Kind().String()
			switch typeName {
			case "slice":
				return "array", nil
			}
			return typeName, nil
		},
		"rand": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return rand.Int(), nil
		},
		"usleep": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			t := params.GetParamByNumber(0, 0).Int()
			time.Sleep(time.Duration(t) * time.Microsecond)
			return nil, nil
		},
		"sleep": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			t := params.GetParamByNumber(0, 0).Int()
			time.Sleep(time.Duration(t) * time.Second)
			return nil, nil
		},
		"isnull": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return params.GetParamByNumber(0).IsUndefined(), nil
		},
		"defined_func": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			_, ok := naslLib[params.GetParamByNumber(0).String()]
			return ok, nil
		},
		"forge_ip_packet": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			data := params.GetParamByName("data").String()
			ip_hl := params.GetParamByName("ip_hl", 5).Int()
			ip_v := params.GetParamByName("ip_v", 4).Int()
			ip_tos := params.GetParamByName("ip_tos", 0).Int()
			ip_id := params.GetParamByName("ip_id", rand.Int()).Int()
			ip_off := params.GetParamByName("ip_off", 0).Int()
			ip_ttl := params.GetParamByName("ip_ttl", 64).Int()
			ip_p := params.GetParamByName("ip_p", 0).Int()
			ip_sum := params.GetParamByName("ip_sum", 0).Int()
			ip_src := params.GetParamByName("ip_src").String()
			ip_dst := params.GetParamByName("ip_dst").String()
			ip_len := params.GetParamByName("ip_len", 0).Int()
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
		"forge_ipv6_packet": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_ipv6_packet` is not implement"))
			return nil, nil
		},
		"get_ip_element": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_ip_element` is not implement"))
			return nil, nil
		},
		"get_ipv6_element": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_ipv6_element` is not implement"))
			return nil, nil
		},
		"set_ip_elements": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `set_ip_elements` is not implement"))
			return nil, nil
		},
		"set_ipv6_elements": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `set_ipv6_elements` is not implement"))
			return nil, nil
		},
		"insert_ip_options": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `insert_ip_options` is not implement"))
			return nil, nil
		},
		"insert_ipv6_options": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `insert_ipv6_options` is not implement"))
			return nil, nil
		},
		"dump_ip_packet": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_ip_packet` is not implement"))
			return nil, nil
		},
		"dump_ipv6_packet": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_ipv6_packet` is not implement"))
			return nil, nil
		},
		"forge_tcp_packet": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_tcp_packet` is not implement"))
			return nil, nil
		},
		"forge_tcp_v6_packet": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_tcp_v6_packet` is not implement"))
			return nil, nil
		},
		"get_tcp_element": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_tcp_element` is not implement"))
			return nil, nil
		},
		"get_tcp_v6_element": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_tcp_v6_element` is not implement"))
			return nil, nil
		},
		"set_tcp_elements": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `set_tcp_elements` is not implement"))
			return nil, nil
		},
		"set_tcp_v6_elements": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `set_tcp_v6_elements` is not implement"))
			return nil, nil
		},
		"dump_tcp_packet": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_tcp_packet` is not implement"))
			return nil, nil
		},
		"dump_tcp_v6_packet": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_tcp_v6_packet` is not implement"))
			return nil, nil
		},
		"tcp_ping": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `tcp_ping` is not implement"))
			return nil, nil
		},
		"tcp_v6_ping": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `tcp_v6_ping` is not implement"))
			return nil, nil
		},
		"forge_udp_packet": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_udp_packet` is not implement"))
			return nil, nil
		},
		"forge_udp_v6_packet": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_udp_v6_packet` is not implement"))
			return nil, nil
		},
		"get_udp_element": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_udp_element` is not implement"))
			return nil, nil
		},
		"get_udp_v6_element": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_udp_v6_element` is not implement"))
			return nil, nil
		},
		"set_udp_elements": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `set_udp_elements` is not implement"))
			return nil, nil
		},
		"set_udp_v6_elements": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `set_udp_v6_elements` is not implement"))
			return nil, nil
		},
		"dump_udp_packet": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_udp_packet` is not implement"))
			return nil, nil
		},
		"dump_udp_v6_packet": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_udp_v6_packet` is not implement"))
			return nil, nil
		},
		"forge_icmp_packet": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_icmp_packet` is not implement"))
			return nil, nil
		},
		"forge_icmp_v6_packet": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_icmp_v6_packet` is not implement"))
			return nil, nil
		},
		"get_icmp_element": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_icmp_element` is not implement"))
			return nil, nil
		},
		"get_icmp_v6_element": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_icmp_v6_element` is not implement"))
			return nil, nil
		},
		"forge_igmp_packet": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_igmp_packet` is not implement"))
			return nil, nil
		},
		"forge_igmp_v6_packet": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_igmp_v6_packet` is not implement"))
			return nil, nil
		},
		"send_packet": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `send_packet` is not implement"))
			return nil, nil
		},
		"send_v6packet": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `send_v6packet` is not implement"))
			return nil, nil
		},
		"pcap_next": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `pcap_next` is not implement"))
			return nil, nil
		},
		"send_capture": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `send_capture` is not implement"))
			return nil, nil
		},
		"MD2": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `MD2` is not implement"))
			return nil, nil
		},
		"MD4": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `MD4` is not implement"))
			return nil, nil
		},
		"MD5": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `MD5` is not implement"))
			return nil, nil
		},
		"SHA1": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `SHA1` is not implement"))
			return nil, nil
		},
		"SHA256": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `SHA256` is not implement"))
			return nil, nil
		},
		"RIPEMD160": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `RIPEMD160` is not implement"))
			return nil, nil
		},
		"HMAC_MD2": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_MD2` is not implement"))
			return nil, nil
		},
		"HMAC_MD5": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_MD5` is not implement"))
			return nil, nil
		},
		"HMAC_SHA1": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_SHA1` is not implement"))
			return nil, nil
		},
		"HMAC_SHA256": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_SHA256` is not implement"))
			return nil, nil
		},
		"HMAC_SHA384": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_SHA384` is not implement"))
			return nil, nil
		},
		"HMAC_SHA512": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_SHA512` is not implement"))
			return nil, nil
		},
		"HMAC_RIPEMD160": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_RIPEMD160` is not implement"))
			return nil, nil
		},
		"prf_sha256": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `prf_sha256` is not implement"))
			return nil, nil
		},
		"prf_sha384": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `prf_sha384` is not implement"))
			return nil, nil
		},
		"tls1_prf": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `tls1_prf` is not implement"))
			return nil, nil
		},
		"ntlmv2_response": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `ntlmv2_response` is not implement"))
			return nil, nil
		},
		"ntlm2_response": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `ntlm2_response` is not implement"))
			return nil, nil
		},
		"ntlm_response": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `ntlm_response` is not implement"))
			return nil, nil
		},
		"key_exchange": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `key_exchange` is not implement"))
			return nil, nil
		},
		"NTLMv1_HASH": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `NTLMv1_HASH` is not implement"))
			return nil, nil
		},
		"NTLMv2_HASH": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `NTLMv2_HASH` is not implement"))
			return nil, nil
		},
		"nt_owf_gen": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `nt_owf_gen` is not implement"))
			return nil, nil
		},
		"lm_owf_gen": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `lm_owf_gen` is not implement"))
			return nil, nil
		},
		"ntv2_owf_gen": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `ntv2_owf_gen` is not implement"))
			return nil, nil
		},
		"insert_hexzeros": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `insert_hexzeros` is not implement"))
			return nil, nil
		},
		"dec2str": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dec2str` is not implement"))
			return nil, nil
		},
		"get_signature": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_signature` is not implement"))
			return nil, nil
		},
		"get_smb2_signature": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_smb2_signature` is not implement"))
			return nil, nil
		},
		"dh_generate_key": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dh_generate_key` is not implement"))
			return nil, nil
		},
		"bn_random": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `bn_random` is not implement"))
			return nil, nil
		},
		"bn_cmp": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `bn_cmp` is not implement"))
			return nil, nil
		},
		"dh_compute_key": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dh_compute_key` is not implement"))
			return nil, nil
		},
		"rsa_public_encrypt": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `rsa_public_encrypt` is not implement"))
			return nil, nil
		},
		"rsa_private_decrypt": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `rsa_private_decrypt` is not implement"))
			return nil, nil
		},
		"rsa_public_decrypt": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `rsa_public_decrypt` is not implement"))
			return nil, nil
		},
		"bf_cbc_encrypt": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `bf_cbc_encrypt` is not implement"))
			return nil, nil
		},
		"bf_cbc_decrypt": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `bf_cbc_decrypt` is not implement"))
			return nil, nil
		},
		"rc4_encrypt": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `rc4_encrypt` is not implement"))
			return nil, nil
		},
		"aes128_cbc_encrypt": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `aes128_cbc_encrypt` is not implement"))
			return nil, nil
		},
		"aes256_cbc_encrypt": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `aes256_cbc_encrypt` is not implement"))
			return nil, nil
		},
		"aes128_ctr_encrypt": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `aes128_ctr_encrypt` is not implement"))
			return nil, nil
		},
		"aes256_ctr_encrypt": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `aes256_ctr_encrypt` is not implement"))
			return nil, nil
		},
		"aes128_gcm_encrypt": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `aes128_gcm_encrypt` is not implement"))
			return nil, nil
		},
		"aes256_gcm_encrypt": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `aes256_gcm_encrypt` is not implement"))
			return nil, nil
		},
		"des_ede_cbc_encrypt": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `des_ede_cbc_encrypt` is not implement"))
			return nil, nil
		},
		"dsa_do_verify": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dsa_do_verify` is not implement"))
			return nil, nil
		},
		"pem_to_rsa": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `pem_to_rsa` is not implement"))
			return nil, nil
		},
		"pem_to_dsa": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `pem_to_dsa` is not implement"))
			return nil, nil
		},
		"rsa_sign": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `rsa_sign` is not implement"))
			return nil, nil
		},
		"dsa_do_sign": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dsa_do_sign` is not implement"))
			return nil, nil
		},
		"gunzip": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `gunzip` is not implement"))
			return nil, nil
		},
		"gzip": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `gzip` is not implement"))
			return nil, nil
		},
		"DES": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `DES` is not implement"))
			return nil, nil
		},
		//源码里没找到
		"pop3_get_banner": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			port := params.GetParamByName("port", -1).Int()
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
		"http_cgi_dirs": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			cgiPath, ok := GlobalPrefs["cgi_path"]
			if ok {
				return []string{cgiPath}, nil
			}
			return []string{}, nil
		},

		"new_preference": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			name := params.GetParamByName("name").AsString()
			typ := params.GetParamByName("typ").AsString()
			value := params.GetParamByName("value").AsString()
			ctx.ScriptObj.Preferences[name] = map[string]string{"type": typ, "value": value}
			return nil, nil
		},
		"dump": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			v := make([]interface{}, 0)
			executor.ForEachParams(params, func(value *yakvm.Value) {
				v = append(v, value.Value)
			})
			spew.Dump(v...)
			return nil, nil
		},
		"wmi_versioninfo": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return nil, nil
		},
		"smb_versioninfo": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return nil, nil
		},
		"register_host_detail": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			name := params.GetParamByName("name", "").String()
			value := params.GetParamByName("value", "").Value
			naslLibCall("set_kb_item", ctx, map[string]interface{}{"name": "HostDetails", "value": name}, nil)
			naslLibCall("set_kb_item", ctx, map[string]interface{}{"name": "HostDetails/NVT", "value": ctx.ScriptObj.OID}, nil)
			naslLibCall("set_kb_item", ctx, map[string]interface{}{"name": fmt.Sprintf("HostDetails/NVT/%s/%s", ctx.ScriptObj.OID, name), "value": value}, nil)
			return nil, nil
		},
		"host_mac": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			target := ctx.Host
			iface, _, _, err := netutil.Route(5*time.Second, target)
			if err != nil {
				return nil, err
			}
			return iface.HardwareAddr.String(), nil
		},
		"pingHost": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			result := pingutil.PingAuto(ctx.Host, pingutil.WithDefaultTcpPort(""), pingutil.WithTimeout(5*time.Second), pingutil.WithProxies(ctx.Proxies...))
			return result.Ok, nil
		},
		"call_yak_method": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			var methodName string
			var args []interface{}
			first := true
			executor.ForEachParams(params, func(value *yakvm.Value) {
				if first {
					methodName = value.String()
					first = false
				} else {
					args = append(args, value.Value)
				}
			})
			if YakScriptEngineGetter == nil {
				return nil, utils.Errorf("yak script engine getter is not set")
			}
			yakEngine := YakScriptEngineGetter()
			yakEngine.SetVars(map[string]any{
				"params": args,
			})
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
		"plugin_run_find_service": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
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
		"is_array": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			p := params.GetParamByNumber(0)
			if p == nil || p.Value == nil {
				return false, nil
			}
			_, ok := p.Value.(*nasl_type.NaslArray)
			return ok, nil
		},
		"ssh_connect": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			p := params.GetParamByNumber(0)
			if p == nil || p.Value == nil {
				return false, nil
			}
			_, ok := p.Value.(*nasl_type.NaslArray)
			return ok, nil
		},
		"http_get_remote_headers": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			port := params.GetParamByNumber(0).Int()
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
		"service_get_ports": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			idefault_port_list := params.GetParamByName("default_port_list", "").Value
			default_port_array, ok := idefault_port_list.(*nasl_type.NaslArray)
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
			nodefault := params.GetParamByName("nodefault", 0).IntBool()
			service := params.GetParamByName("proto", "").String()
			ipproto := params.GetParamByName("ipproto", "tcp").String()
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
		"service_get_port": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			res, err := naslLibCall("service_get_ports", ctx, map[string]interface{}{
				"proto":             params.GetParamByName("proto", "").Value,
				"ipproto":           params.GetParamByName("ipproto", "").Value,
				"default_port_list": []int{params.GetParamByName("default", "").Int()},
				"nodefault":         params.GetParamByName("nodefault", 0).IntBool(),
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
		"unknownservice_get_port": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return naslLibCall("service_get_port", ctx, map[string]interface{}{
				"proto":     "unknown",
				"ipproto":   params.GetParamByName("ipproto", "").Value,
				"default":   params.GetParamByName("default", 0).Int(),
				"nodefault": params.GetParamByName("nodefault", 0).IntBool(),
			}, nil)
		},
		"unknownservice_get_ports": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return naslLibCall("service_get_ports", ctx, map[string]interface{}{
				"proto":             "unknown",
				"ipproto":           params.GetParamByName("ipproto", "").Value,
				"default_port_list": params.GetParamByName("default_port_list", "").Value,
				"nodefault":         params.GetParamByName("nodefault", 0).IntBool(),
			}, nil)
		},
		"report_vuln_url": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return naslLibCall("http_report_vuln_url", ctx, map[string]interface{}{
				"port":     params.GetParamByName("port", 0).Value,
				"url":      params.GetParamByName("url", "").Value,
				"url_only": params.GetParamByName("url_only", false).Value,
			}, nil)
		},
		//http_report_vuln_url(port: port, url: url1, url_only: TRUE);
		"http_report_vuln_url": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			port := params.GetParamByName("port", "").Int()
			url := params.GetParamByName("url", "").String()
			url_only := params.GetParamByName("url_only", false).IntBool()
			if url_only {
				return fmt.Sprintf("%v%v", utils.HostPort(ctx.Host, port), url), nil
			} else {
				return fmt.Sprintf("detect vul on: %v%v", utils.HostPort(ctx.Host, port), url), nil
			}
		},
		//build_detection_report(app: "OpenMairie Open Foncier", version: version,
		//install: install, cpe: cpe, concluded: vers[0],
		//concludedUrl: concUrl),
		"build_detection_report": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			scriptObj := ctx.ScriptObj
			app := params.GetParamByName("app", "").String()
			version := params.GetParamByName("version", "").String()
			install := params.GetParamByName("install", "").String()
			cpe := params.GetParamByName("cpe", "").String()
			concluded := params.GetParamByName("concluded", "").String()
			riskType := ""
			if v, ok := utils2.ActToChinese[scriptObj.Category]; ok {
				riskType = v
			} else {
				riskType = scriptObj.Category
			}
			source := "[NaslScript] " + scriptObj.ScriptName
			concludedUrl := params.GetParamByName("concludedUrl", "").String()
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
		"ftp_get_banner": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			port := params.GetParamByName("port", -1).Int()
			if port == -1 {
				return nil, fmt.Errorf("port is not set")
			}
			banner, err := GetPortBannerByCache(ctx, port)
			if err != nil {
				return nil, err
			}
			return banner, nil
		},
		"telnet_get_banner": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			port := params.GetParamByName("port", -1).Int()
			if port == -1 {
				return nil, fmt.Errorf("port is not set")
			}
			banner, err := GetPortBannerByCache(ctx, port)
			if err != nil {
				return nil, err
			}
			return banner, nil
		},
		"smtp_get_banner": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			port := params.GetParamByName("port", -1).Int()
			if port == -1 {
				return nil, fmt.Errorf("port is not set")
			}
			banner, err := GetPortBannerByCache(ctx, port)
			if err != nil {
				return nil, err
			}
			return banner, nil
		},
		"imap_get_banner": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			port := params.GetParamByName("port", -1).Int()
			if port == -1 {
				return nil, fmt.Errorf("port is not set")
			}
			banner, err := GetPortBannerByCache(ctx, port)
			if err != nil {
				return nil, err
			}
			return banner, nil
		},
		"http_can_host_php": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			ok := false
			rsp, _ := http.Get(fmt.Sprintf("http://%s:%d/index.php", ctx.Host, params.GetParamByName("port", 80).Int()))
			if rsp != nil && rsp.StatusCode == 200 {
				ok = true
			}
			if !ok {
				rsp, _ := http.Get(fmt.Sprintf("https://%s:%d/index.php", ctx.Host, params.GetParamByName("port", 443).Int()))
				if rsp != nil && rsp.StatusCode == 200 {
					ok = true
				}
			}
			return ok, nil
		},
		"http_can_host_asp": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			ok := false
			rsp, _ := http.Get(fmt.Sprintf("http://%s:%d/index.asp", ctx.Host, params.GetParamByName("port", 80).Int()))
			if rsp != nil && rsp.StatusCode == 200 {
				ok = true
			}
			if !ok {
				rsp, _ := http.Get(fmt.Sprintf("https://%s:%d/index.asp", ctx.Host, params.GetParamByName("port", 443).Int()))
				if rsp != nil && rsp.StatusCode == 200 {
					ok = true
				}
			}
			return ok, nil
		},
		"http_extract_body_from_response": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			res := params.GetParamByName("data", "").String()
			_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket([]byte(res))
			return body, nil
		},
		"os_host_runs": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			if params.GetParamByNumber(0, "").String() == runtime.GOOS {
				return true, nil
			}
			return false, nil
		},
		"wmi_file_is_file_search_disabled": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return true, nil
		},
		"snmp_get_port": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return nil, nil
		},
		//需要把ssh相关插件重写
		"ssh_session_id_from_sock": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			panic("not implement")
		},
		"ssh_get_port": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			return nil, nil
		},
		"in_array": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			search := params.GetParamByName("search", "").String()
			iArray := params.GetParamByName("array", nil).Value
			var array *nasl_type.NaslArray
			if v, ok := iArray.(*nasl_type.NaslArray); ok {
				array = v
			} else {
				panic("param array is not an array")
			}
			_, ok := array.Hash_elt[search]
			return ok, nil
		},
		"exit": func(ctx *ExecContext, params *executor.NaslBuildInMethodParam) (interface{}, error) {
			code := params.GetParamByNumber(0).Int()
			msg := params.GetParamByNumber(1, "").String()
			panic(yakvm.NewVMPanic(&yakvm.VMPanicSignal{Info: code, AdditionalInfo: map[string]string{"code": strconv.Itoa(code), "msg": msg}}))
			return nil, nil
		},
	}
}
func GetExtLib(ctx *ExecContext) map[string]func(params *executor.NaslBuildInMethodParam) interface{} {
	lib := make(map[string]func(params *executor.NaslBuildInMethodParam) interface{})
	for name, method := range naslLib {
		name := name
		method := method
		lib[name] = func(params *executor.NaslBuildInMethodParam) interface{} {
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
			for _, v := range params.ListParams {
				paramstr += fmt.Sprintf("%v,", v)
			}
			for k, v := range params.MapParams {
				paramstr += fmt.Sprintf("%s=%v,", k, v)
			}

			if err != nil {
				naslLogger.Errorf("call build in function `%s(%v)` error in script `%v`: %v", name, paramstr, ctx.ScriptObj.OriginFileName, err)
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
				array, err := nasl_type.NewNaslArray(res)
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
		params := executor.NewNaslBuildInMethodParam()
		for _, i1 := range sliceParam {
			params.ListParams = append(params.ListParams, yakvm.NewAutoValue(i1))
		}
		for k, v := range mapParam {
			params.MapParams[k] = yakvm.NewAutoValue(v)
		}
		return v(ctx, params)
	}
	return nil, fmt.Errorf("not found function: %s", name)
}
