package antlr4nasl

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"net"
	"os"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/lowhttp"
	utils2 "yaklang/common/yak/antlr4nasl/lib"
	"yaklang/common/yak/antlr4yak/yakvm"
	"path"
	"regexp"
	"strings"
	"time"
)

var empty = yakvm.NewValue("empty", nil, "empty")

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

type NaslBuildInMethod func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error)

var NaslLib = make(map[string]func(engine *Engine, params *NaslBuildInMethodParam) interface{})

func init() {
	naslLib := map[string]NaslBuildInMethod{
		//"sleep": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
		//	n := params.getParamByNumber(0, 0)
		//	time.Sleep(time.Duration(n) * time.Second)
		//},
		"script_name": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			engine.scriptObj.ScriptName = params.getParamByNumber(0).AsString()
			return nil, nil
		},
		"script_version": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			engine.scriptObj.Version = params.getParamByNumber(0).AsString()
			return nil, nil
		},
		"script_timeout": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			t := params.getParamByNumber(0, -65535).Int()
			if t == -65535 {
				panic(utils.Errorf("invalid timeout argument: %d", t))
			}
			engine.scriptObj.Timeout = t
			return nil, nil
		},
		"script_copyright": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			engine.scriptObj.Copyright = params.getParamByNumber(0).AsString()
			return nil, nil
		},
		"script_category": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			engine.scriptObj.Category = params.getParamByNumber(0).AsString()
			return nil, nil
		},
		"script_family": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			engine.scriptObj.Family = params.getParamByNumber(0).AsString()
			return nil, nil
		},
		"script_dependencies": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			var deps *yakvm.Value
			for i := 0; deps != nil && !deps.IsUndefined(); i++ {
				deps = params.getParamByNumber(0)
				engine.scriptObj.Dependencies = append(engine.scriptObj.Dependencies, deps.AsString())
			}
			return nil, nil
		},
		"script_require_keys": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			var item *yakvm.Value
			for i := 0; item != nil && !item.IsUndefined(); i++ {
				item = params.getParamByNumber(0)
				engine.scriptObj.RequireKeys = append(engine.scriptObj.RequireKeys, item.AsString())
			}
			return nil, nil
		},
		"script_mandatory_keys": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			re := params.getParamByNumber(0).AsString()
			splits := strings.Split(re, "=")
			var item *yakvm.Value
			for i := 0; item != nil && !item.IsUndefined(); i++ {
				item = params.getParamByNumber(0)
				if len(splits) > 0 && item.AsString() == splits[0] {
					engine.scriptObj.MandatoryKeys = append(engine.scriptObj.MandatoryKeys, re)
					re = ""
				} else {
					engine.scriptObj.MandatoryKeys = append(engine.scriptObj.MandatoryKeys, item.AsString())
				}
			}
			if re != "" {
				engine.scriptObj.MandatoryKeys = append(engine.scriptObj.MandatoryKeys, re)
			}
			return nil, nil
		},
		"script_require_ports": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			var item *yakvm.Value
			for i := 0; item != nil && !item.IsUndefined(); i++ {
				item = params.getParamByNumber(0)
				engine.scriptObj.RequirePorts = append(engine.scriptObj.RequirePorts, item.AsString())
			}
			return nil, nil
		},
		"script_require_udp_ports": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			var item *yakvm.Value
			for i := 0; item != nil && !item.IsUndefined(); i++ {
				item = params.getParamByNumber(0)
				engine.scriptObj.RequireUdpPorts = append(engine.scriptObj.RequireUdpPorts, item.AsString())
			}
			return nil, nil
		},
		"script_exclude_keys": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			var item *yakvm.Value
			for i := 0; item != nil && !item.IsUndefined(); i++ {
				item = params.getParamByNumber(0)
				engine.scriptObj.ExcludeKeys = append(engine.scriptObj.ExcludeKeys, item.AsString())
			}
			return nil, nil
		},
		"script_add_preference": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			addPreference := func(s1, s2, s3 string) {
				engine.scriptObj.Preferences["name"] = s1
				engine.scriptObj.Preferences["type"] = s2
				engine.scriptObj.Preferences["value"] = s3
			}
			name := params.getParamByName("name")
			type_ := params.getParamByName("type")
			value := params.getParamByName("value")
			if name.IsUndefined() || type_.IsUndefined() || value.IsUndefined() {
				panic(genNotMatchedArgumentTypeError("script_add_preference"))
			}
			addPreference(name.AsString(), type_.AsString(), value.AsString())
			return nil, nil
		},
		"script_get_preference": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			pref := params.getParamByNumber(0)
			if pref.IsUndefined() {
				return nil, genNotMatchedArgumentTypeError("script_get_preference")
			}
			if v, ok := engine.scriptObj.Preferences[pref.AsString()]; ok {
				return v, nil
			}
			return nil, nil
		},
		"script_get_preference_file_content": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			//ref := params.getParamByNumber(0).AsString()
			//
			panic(fmt.Sprintf("method `script_get_preference_file_content` is not implement"))
			return nil, nil
		},
		"script_get_preference_file_location": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `script_get_preference_file_location` is not implement"))
			return nil, nil
		},
		"script_oid": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			engine.scriptObj.OID = params.getParamByNumber(0).AsString()
			return nil, nil
		},
		"script_cve_id": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			forEachParams(params, func(value *yakvm.Value) {
				engine.scriptObj.CVE = append(engine.scriptObj.CVE, value.AsString())
			})
			return nil, nil
		},
		"script_bugtraq_id": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			forEachParams(params, func(value *yakvm.Value) {
				engine.scriptObj.BugtraqId = append(engine.scriptObj.BugtraqId, value.Int())
			})
			return nil, nil
		},
		"script_xref": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			name := params.getParamByName("name")
			value := params.getParamByName("value")
			if name.IsUndefined() || value.IsUndefined() {
				return nil, utils.Errorf("<name> or <value> is empty")
			}
			engine.scriptObj.Xrefs[name.String()] = value.String()
			return nil, nil
		},
		"script_tag": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			name := params.getParamByName("name")
			value := params.getParamByName("value")
			if name.IsUndefined() || value.IsUndefined() {
				return nil, utils.Errorf("<name> or <value> is empty")
			}
			engine.scriptObj.Tags[name.String()] = value.String()
			return nil, nil
		},
		"vendor_version": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			return vendor_version, nil
		},
		"get_preference": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			name := params.getParamByNumber(0)
			if name.IsUndefined() {
				return nil, utils.Error("<name> is empty")
			}
			preference := engine.scriptObj.Preferences[name.String()]
			return preference, nil
		},
		"safe_checks": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `safe_checks` is not implement"))
			return nil, nil
		},
		"get_script_oid": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			return engine.scriptObj.OID, nil
		},
		"replace_kb_item": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			name := params.getParamByName("name")
			value := params.getParamByName("value")
			if name.IsUndefined() || value.IsUndefined() {
				return nil, utils.Errorf("<name> or <value> is empty")
			}
			engine.scriptObj.Kbs[name.String()] = value.Value
			return nil, nil
		},
		"set_kb_item": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			name := params.getParamByName("name")
			value := params.getParamByName("value")
			if name.IsUndefined() || value.IsUndefined() {
				return nil, utils.Errorf("<name> or <value> is empty")
			}
			engine.scriptObj.Kbs[name.String()] = value.Value
			return nil, nil
		},
		"get_kb_item": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			name := params.getParamByNumber(0)
			if v, ok := engine.scriptObj.Kbs[name.String()]; ok {
				return v, nil
			}
			return nil, nil
		},
		"get_kb_list": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			name := params.getParamByNumber(0)
			res := map[string]interface{}{}
			s := name.String()
			for k, v := range engine.scriptObj.Kbs {
				if utils.MatchAllOfGlob(k, s) {
					res[k] = v
				}
			}
			return res, nil
		},
		"security_message": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
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
		"log_message": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			protocol := params.getParamByName("protocol")
			if protocol.IsUndefined() {
				protocol = params.getParamByName("proto")
			}
			port := params.getParamByName("port", -1)
			data := params.getParamByName("data").AsString()
			if data == "" {
				data = "Success"
			}
			commonLogger.Info(data, port.Int(), protocol.String())
			return nil, nil
		},
		"error_message": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			protocol := params.getParamByName("protocol")
			if protocol.IsUndefined() {
				protocol = params.getParamByName("proto")
			}
			port := params.getParamByName("port", -1)
			data := params.getParamByName("data").AsString()
			if data == "" {
				data = "Success"
			}
			errorLogger.Info(data, port.Int(), protocol.String())
			return nil, nil
		},
		"open_sock_tcp": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			//timeout := params.getParamByName("timeout", engine.scriptObj.Timeout*2).Int()
			//transport := params.getParamByName("transport", -1).Int()

			//port := params.getParamByName("port", 0).Int()
			//tcpAddr, err := net.ResolveTCPAddr("tcp4", fmt.Sprintf("%s:%d", engine.host, port))
			//if err != nil {
			//	return nil, err
			//}
			//conn, err := net.DialTCP("tcp", nil, tcpAddr)
			//return conn, nil
			panic(fmt.Sprintf("method `open_sock_tcp` is not implement"))
			return nil, nil
		},
		"open_sock_udp": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `open_sock_udp` is not implement"))
			return nil, nil
		},
		"open_priv_sock_tcp": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `open_priv_sock_tcp` is not implement"))
			return nil, nil
		},
		"open_priv_sock_udp": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `open_priv_sock_udp` is not implement"))
			return nil, nil
		},
		"socket_get_error": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_get_error` is not implement"))
			return nil, nil
		},
		"recv": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			//length := params.getParamByName("length", -1).Int()
			//min := params.getParamByName("min", -1).Int()
			iconn := params.getParamByName("socket", nil).Value
			timeout := params.getParamByName("timeout", 2).Int()
			conn := iconn.(net.Conn)
			res := utils.StableReader(conn, time.Second*time.Duration(timeout), 10240)
			return res, nil
		},
		"recv_line": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			iconn := params.getParamByName("socket", nil).Value
			//length := params.getParamByName("length", -1).Int()
			timeout := params.getParamByName("timeout", 5).Int()
			conn := iconn.(net.Conn)
			if err := conn.SetReadDeadline(time.Now().Add(time.Second * time.Duration(timeout))); err != nil {
				return "", err
			}
			byt := make([]byte, 1)
			var buf bytes.Buffer
			for {
				n, err := conn.Read(byt)
				if err != nil {
					break
				}
				if n == 0 {
					break
				}
				if byt[0] == '\n' {
					break
				}
				buf.Write(byt[:n])
			}
			//if len(line) > length {
			//	return line[:length], nil
			//}
			return buf.String(), nil
		},
		"send": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			iconn := params.getParamByName("socket", nil).Value
			data := params.getParamByName("data").AsString()
			option := params.getParamByName("option", 0)
			length := params.getParamByName("length", 0)
			data_length := len(data)
			_ = option
			_ = length
			_ = data_length
			if conn, ok := iconn.(net.Conn); ok {
				n, err := conn.Write([]byte(data))
				if err != nil {
					log.Error(err)
					return 0, nil
				}
				return n, nil
			} else {
				panic(notMatchedArgumentTypeError)
				return nil, notMatchedArgumentTypeError
			}
		},
		"socket_negotiate_ssl": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_negotiate_ssl` is not implement"))
			return nil, nil
		},
		"socket_get_cert": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_get_cert` is not implement"))
			return nil, nil
		},
		"socket_get_ssl_version": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_get_ssl_version` is not implement"))
			return nil, nil
		},
		"socket_get_ssl_ciphersuite": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_get_ssl_ciphersuite` is not implement"))
			return nil, nil
		},
		"socket_get_ssl_session_id": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_get_ssl_session_id` is not implement"))
			return nil, nil
		},
		"socket_cert_verify": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `socket_cert_verify` is not implement"))
			return nil, nil
		},
		"close": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `close` is not implement"))
			return nil, nil
		},
		"join_multicast_group": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `join_multicast_group` is not implement"))
			return nil, nil
		},
		"leave_multicast_group": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `leave_multicast_group` is not implement"))
			return nil, nil
		},
		"get_source_port": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_source_port` is not implement"))
			return nil, nil
		},
		"get_sock_info": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_sock_info` is not implement"))
			return nil, nil
		},
		"cgibin": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `cgibin` is not implement"))
			return nil, nil
		},
		"http_open_socket": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			port := params.getParamByName("port", 0).Int()
			adderss := fmt.Sprintf("%s:%d", engine.host, port)
			var n int

			if utils.IsTLSService(adderss) {
				n = utils2.OPENVAS_ENCAPS_SSLv2
			} else {
				n = utils2.OPENVAS_ENCAPS_IP
			}
			conn, err := net.DialTimeout("tcp", adderss, time.Second*5)
			if err != nil {
				return nil, err
			}
			if n > utils2.OPENVAS_ENCAPS_IP {
				tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionSSL30, ServerName: engine.host})
				if err := tlsConn.HandshakeContext(context.Background()); err != nil {
					return nil, err
				} else {
					conn = tlsConn
				}
			}
			if _, err := engine.CallNativeFunction("set_kb_item", map[string]interface{}{
				"name":  fmt.Sprintf("Transports/TCP/%d", port),
				"value": int(n),
			}, nil); err != nil {
				return nil, err
			}
			return conn, nil
		},
		"http_head": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `http_head` is not implement"))
			return nil, nil
		},
		"http_get": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			res := lowhttp.UrlToGetRequestPacket(fmt.Sprintf("http://%s:%d%s", engine.host, params.getParamByName("port", -1).Int(), params.getParamByName("item").String()), nil, false)
			return res, nil
		},
		"http_post": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `http_post` is not implement"))
			return nil, nil
		},
		"http_delete": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `http_delete` is not implement"))
			return nil, nil
		},
		"http_put": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `http_put` is not implement"))
			return nil, nil
		},
		"http_close_socket": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			connV := params.getParamByNumber(0, nil)
			conn := connV.Value
			if v, ok := conn.(net.Conn); ok {
				return nil, v.Close()
			} else {
				panic(notMatchedArgumentTypeError)
				return nil, notMatchedArgumentTypeError
			}
		},
		"add_host_name": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `add_host_name` is not implement"))
			return nil, nil
		},
		"get_host_name": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			//name, err := os.Hostname()
			//if err != nil {
			//	return "", err
			//}
			return engine.host, nil
		},
		"get_host_names": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_host_names` is not implement"))
			return nil, nil
		},
		"get_host_name_source": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_host_name_source` is not implement"))
			return nil, nil
		},
		"resolve_host_name": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `resolve_host_name` is not implement"))
			return nil, nil
		},
		"get_host_ip": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			//_, _, sIp, err := netutil.Route(time.Duration(engine.scriptObj.Timeout*2)*time.Second, utils.ExtractHost("8.8.8.8"))
			//if err != nil {
			//	return nil, err
			//}
			//return sIp.String(), nil
			return engine.host, nil
		},
		"same_host": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `same_host` is not implement"))
			return nil, nil
		},
		"TARGET_IS_IPV6": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `TARGET_IS_IPV6` is not implement"))
			return nil, nil
		},
		"get_host_open_port": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_host_open_port` is not implement"))
			return nil, nil
		},
		"get_port_state": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			port := params.getParamByNumber(0).Int()
			if v, ok := engine.scriptObj.Kbs["Host/scanned"]; ok {
				if v2, ok := v.([]int); ok {
					return utils.IntArrayContains(v2, port), nil
				}
			}
			return false, nil
		},
		"get_tcp_port_state": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_tcp_port_state` is not implement"))
			return nil, nil
		},
		"get_udp_port_state": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_udp_port_state` is not implement"))
			return nil, nil
		},
		"scanner_add_port": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `scanner_add_port` is not implement"))
			return nil, nil
		},
		"scanner_status": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `scanner_status` is not implement"))
			return nil, nil
		},
		"scanner_get_port": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `scanner_get_port` is not implement"))
			return nil, nil
		},
		"islocalhost": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `islocalhost` is not implement"))
			return nil, nil
		},
		"islocalnet": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `islocalnet` is not implement"))
			return nil, nil
		},
		"get_port_transport": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			port := params.getParamByNumber(0, -1).Int()
			if port > 0 {
				k := fmt.Sprintf("Transports/TCP/%d", port)
				v, err := engine.CallNativeFunction("get_kb_item", nil, []interface{}{k})
				if err != nil {
					return nil, err
				}
				if v1, ok := v.(int); ok {
					if params.getParamByName("asstring").Bool() {
						return utils2.GetEncapsName(v1), nil
					} else {
						return v1, nil
					}
				}
			}
			return -1, nil
		},
		"this_host": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `this_host` is not implement"))
			return nil, nil
		},
		"this_host_name": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `this_host_name` is not implement"))
			return nil, nil
		},
		"string": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			return params.getParamByNumber(0).String(), nil
		},
		"raw_string": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `raw_string` is not implement"))
			return nil, nil
		},
		"strcat": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			s := ""
			forEachParams(params, func(value *yakvm.Value) {
				s += value.String()
			})
			return s, nil
		},
		"display": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			s := ""
			forEachParams(params, func(value *yakvm.Value) {
				s += value.String()
			})
			fmt.Sprintln(s)
			return nil, nil
		},
		"ord": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `ord` is not implement"))
			return nil, nil
		},
		"hex": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `hex` is not implement"))
			return nil, nil
		},
		"hexstr": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `hexstr` is not implement"))
			return nil, nil
		},
		"strstr": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `strstr` is not implement"))
			return nil, nil
		},
		"ereg": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			pattern := params.getParamByName("pattern").String()
			s := params.getParamByName("string").String()
			matched, err := regexp.MatchString(pattern, s)
			if err != nil {
				return false, err
			}
			return matched, nil
		},
		"ereg_replace": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			icase := params.getParamByName("icase", false).Bool()
			pattern := params.getParamByName("pattern").String()
			s := params.getParamByName("string").String()
			replace := params.getParamByName("replace").String()

			if icase {
				pattern = "(?i)" + pattern
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return s, err
			}
			return re.ReplaceAllString(s, replace), nil
		},
		"egrep": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			pattern := params.getParamByName("pattern").String()
			s := params.getParamByName("string").String()
			icase := params.getParamByName("icase").Bool()
			if icase {
				pattern = "(?i)" + pattern
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return "", err
			}
			return re.FindString(s), nil
		},
		"eregmatch": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			pattern := params.getParamByName("pattern").String()
			s := params.getParamByName("string").String()
			icase := params.getParamByName("icase").Bool()
			if icase {
				pattern = "(?i)" + pattern
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return "", err
			}
			return re.FindStringSubmatch(s), nil
		},
		"match": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `match` is not implement"))
			return nil, nil
		},
		"substr": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `substr` is not implement"))
			return nil, nil
		},
		"insstr": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `insstr` is not implement"))
			return nil, nil
		},
		"tolower": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			return strings.ToLower(params.getParamByNumber(0).String()), nil
		},
		"toupper": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			return strings.ToUpper(params.getParamByNumber(0).String()), nil
		},
		"crap": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `crap` is not implement"))
			return nil, nil
		},
		"strlen": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			return len(params.getParamByNumber(0).String()), nil
		},
		"split": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `split` is not implement"))
			return nil, nil
		},
		"chomp": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `chomp` is not implement"))
			return nil, nil
		},
		"int": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `int` is not implement"))
			return nil, nil
		},
		"stridx": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			s := params.getParamByNumber(0).String()
			subs := params.getParamByNumber(1).String()
			start := params.getParamByNumber(2)
			if start.IsInt() {
				s = s[start.Int():]
			}
			return strings.Index(s, subs), nil
		},
		"str_replace": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `str_replace` is not implement"))
			return nil, nil
		},
		"make_list": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			res := make([]interface{}, 0)
			forEachParams(params, func(value *yakvm.Value) {
				res = append(res, value.Value)
			})
			return res, nil
		},
		"make_array": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			array := make(map[interface{}]interface{})
			iskey := false
			var v interface{}
			forEachParams(params, func(value *yakvm.Value) {
				if !iskey {
					v = value.Value
				} else {
					array[value.Value] = v
				}
				iskey = !iskey
			})
			return array, nil
		},
		"keys": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `keys` is not implement"))
			return nil, nil
		},
		"max_index": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `max_index` is not implement"))
			return nil, nil
		},
		"sort": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `sort` is not implement"))
			return nil, nil
		},
		"unixtime": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `unixtime` is not implement"))
			return nil, nil
		},
		"gettimeofday": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `gettimeofday` is not implement"))
			return nil, nil
		},
		"localtime": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `localtime` is not implement"))
			return nil, nil
		},
		"mktime": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `mktime` is not implement"))
			return nil, nil
		},
		"open_sock_kdc": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `open_sock_kdc` is not implement"))
			return nil, nil
		},
		"telnet_init": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `telnet_init` is not implement"))
			return nil, nil
		},
		"ftp_log_in": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `ftp_log_in` is not implement"))
			return nil, nil
		},
		"ftp_get_pasv_port": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `ftp_get_pasv_port` is not implement"))
			return nil, nil
		},
		"start_denial": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `start_denial` is not implement"))
			return nil, nil
		},
		"end_denial": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `end_denial` is not implement"))
			return nil, nil
		},
		"dump_ctxt": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_ctxt` is not implement"))
			return nil, nil
		},
		"typeof": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `typeof` is not implement"))
			return nil, nil
		},
		"rand": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `rand` is not implement"))
			return nil, nil
		},
		"usleep": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `usleep` is not implement"))
			return nil, nil
		},
		"sleep": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `sleep` is not implement"))
			return nil, nil
		},
		"isnull": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			return params.getParamByNumber(0).IsUndefined(), nil
		},
		"defined_func": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			_, ok := NaslLib[params.getParamByNumber(0).String()]
			return ok, nil
		},
		"forge_ip_packet": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_ip_packet` is not implement"))
			return nil, nil
		},
		"forge_ipv6_packet": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_ipv6_packet` is not implement"))
			return nil, nil
		},
		"get_ip_element": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_ip_element` is not implement"))
			return nil, nil
		},
		"get_ipv6_element": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_ipv6_element` is not implement"))
			return nil, nil
		},
		"set_ip_elements": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `set_ip_elements` is not implement"))
			return nil, nil
		},
		"set_ipv6_elements": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `set_ipv6_elements` is not implement"))
			return nil, nil
		},
		"insert_ip_options": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `insert_ip_options` is not implement"))
			return nil, nil
		},
		"insert_ipv6_options": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `insert_ipv6_options` is not implement"))
			return nil, nil
		},
		"dump_ip_packet": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_ip_packet` is not implement"))
			return nil, nil
		},
		"dump_ipv6_packet": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_ipv6_packet` is not implement"))
			return nil, nil
		},
		"forge_tcp_packet": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_tcp_packet` is not implement"))
			return nil, nil
		},
		"forge_tcp_v6_packet": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_tcp_v6_packet` is not implement"))
			return nil, nil
		},
		"get_tcp_element": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_tcp_element` is not implement"))
			return nil, nil
		},
		"get_tcp_v6_element": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_tcp_v6_element` is not implement"))
			return nil, nil
		},
		"set_tcp_elements": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `set_tcp_elements` is not implement"))
			return nil, nil
		},
		"set_tcp_v6_elements": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `set_tcp_v6_elements` is not implement"))
			return nil, nil
		},
		"dump_tcp_packet": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_tcp_packet` is not implement"))
			return nil, nil
		},
		"dump_tcp_v6_packet": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_tcp_v6_packet` is not implement"))
			return nil, nil
		},
		"tcp_ping": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `tcp_ping` is not implement"))
			return nil, nil
		},
		"tcp_v6_ping": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `tcp_v6_ping` is not implement"))
			return nil, nil
		},
		"forge_udp_packet": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_udp_packet` is not implement"))
			return nil, nil
		},
		"forge_udp_v6_packet": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_udp_v6_packet` is not implement"))
			return nil, nil
		},
		"get_udp_element": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_udp_element` is not implement"))
			return nil, nil
		},
		"get_udp_v6_element": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_udp_v6_element` is not implement"))
			return nil, nil
		},
		"set_udp_elements": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `set_udp_elements` is not implement"))
			return nil, nil
		},
		"set_udp_v6_elements": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `set_udp_v6_elements` is not implement"))
			return nil, nil
		},
		"dump_udp_packet": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_udp_packet` is not implement"))
			return nil, nil
		},
		"dump_udp_v6_packet": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dump_udp_v6_packet` is not implement"))
			return nil, nil
		},
		"forge_icmp_packet": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_icmp_packet` is not implement"))
			return nil, nil
		},
		"forge_icmp_v6_packet": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_icmp_v6_packet` is not implement"))
			return nil, nil
		},
		"get_icmp_element": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_icmp_element` is not implement"))
			return nil, nil
		},
		"get_icmp_v6_element": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_icmp_v6_element` is not implement"))
			return nil, nil
		},
		"forge_igmp_packet": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_igmp_packet` is not implement"))
			return nil, nil
		},
		"forge_igmp_v6_packet": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `forge_igmp_v6_packet` is not implement"))
			return nil, nil
		},
		"send_packet": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `send_packet` is not implement"))
			return nil, nil
		},
		"send_v6packet": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `send_v6packet` is not implement"))
			return nil, nil
		},
		"pcap_next": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `pcap_next` is not implement"))
			return nil, nil
		},
		"send_capture": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `send_capture` is not implement"))
			return nil, nil
		},
		"MD2": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `MD2` is not implement"))
			return nil, nil
		},
		"MD4": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `MD4` is not implement"))
			return nil, nil
		},
		"MD5": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `MD5` is not implement"))
			return nil, nil
		},
		"SHA1": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `SHA1` is not implement"))
			return nil, nil
		},
		"SHA256": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `SHA256` is not implement"))
			return nil, nil
		},
		"RIPEMD160": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `RIPEMD160` is not implement"))
			return nil, nil
		},
		"HMAC_MD2": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_MD2` is not implement"))
			return nil, nil
		},
		"HMAC_MD5": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_MD5` is not implement"))
			return nil, nil
		},
		"HMAC_SHA1": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_SHA1` is not implement"))
			return nil, nil
		},
		"HMAC_SHA256": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_SHA256` is not implement"))
			return nil, nil
		},
		"HMAC_SHA384": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_SHA384` is not implement"))
			return nil, nil
		},
		"HMAC_SHA512": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_SHA512` is not implement"))
			return nil, nil
		},
		"HMAC_RIPEMD160": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `HMAC_RIPEMD160` is not implement"))
			return nil, nil
		},
		"prf_sha256": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `prf_sha256` is not implement"))
			return nil, nil
		},
		"prf_sha384": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `prf_sha384` is not implement"))
			return nil, nil
		},
		"tls1_prf": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `tls1_prf` is not implement"))
			return nil, nil
		},
		"ntlmv2_response": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `ntlmv2_response` is not implement"))
			return nil, nil
		},
		"ntlm2_response": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `ntlm2_response` is not implement"))
			return nil, nil
		},
		"ntlm_response": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `ntlm_response` is not implement"))
			return nil, nil
		},
		"key_exchange": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `key_exchange` is not implement"))
			return nil, nil
		},
		"NTLMv1_HASH": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `NTLMv1_HASH` is not implement"))
			return nil, nil
		},
		"NTLMv2_HASH": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `NTLMv2_HASH` is not implement"))
			return nil, nil
		},
		"nt_owf_gen": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `nt_owf_gen` is not implement"))
			return nil, nil
		},
		"lm_owf_gen": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `lm_owf_gen` is not implement"))
			return nil, nil
		},
		"ntv2_owf_gen": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `ntv2_owf_gen` is not implement"))
			return nil, nil
		},
		"insert_hexzeros": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `insert_hexzeros` is not implement"))
			return nil, nil
		},
		"dec2str": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dec2str` is not implement"))
			return nil, nil
		},
		"get_signature": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_signature` is not implement"))
			return nil, nil
		},
		"get_smb2_signature": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `get_smb2_signature` is not implement"))
			return nil, nil
		},
		"dh_generate_key": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dh_generate_key` is not implement"))
			return nil, nil
		},
		"bn_random": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `bn_random` is not implement"))
			return nil, nil
		},
		"bn_cmp": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `bn_cmp` is not implement"))
			return nil, nil
		},
		"dh_compute_key": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dh_compute_key` is not implement"))
			return nil, nil
		},
		"rsa_public_encrypt": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `rsa_public_encrypt` is not implement"))
			return nil, nil
		},
		"rsa_private_decrypt": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `rsa_private_decrypt` is not implement"))
			return nil, nil
		},
		"rsa_public_decrypt": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `rsa_public_decrypt` is not implement"))
			return nil, nil
		},
		"bf_cbc_encrypt": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `bf_cbc_encrypt` is not implement"))
			return nil, nil
		},
		"bf_cbc_decrypt": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `bf_cbc_decrypt` is not implement"))
			return nil, nil
		},
		"rc4_encrypt": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `rc4_encrypt` is not implement"))
			return nil, nil
		},
		"aes128_cbc_encrypt": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `aes128_cbc_encrypt` is not implement"))
			return nil, nil
		},
		"aes256_cbc_encrypt": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `aes256_cbc_encrypt` is not implement"))
			return nil, nil
		},
		"aes128_ctr_encrypt": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `aes128_ctr_encrypt` is not implement"))
			return nil, nil
		},
		"aes256_ctr_encrypt": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `aes256_ctr_encrypt` is not implement"))
			return nil, nil
		},
		"aes128_gcm_encrypt": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `aes128_gcm_encrypt` is not implement"))
			return nil, nil
		},
		"aes256_gcm_encrypt": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `aes256_gcm_encrypt` is not implement"))
			return nil, nil
		},
		"des_ede_cbc_encrypt": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `des_ede_cbc_encrypt` is not implement"))
			return nil, nil
		},
		"dsa_do_verify": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dsa_do_verify` is not implement"))
			return nil, nil
		},
		"pem_to_rsa": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `pem_to_rsa` is not implement"))
			return nil, nil
		},
		"pem_to_dsa": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `pem_to_dsa` is not implement"))
			return nil, nil
		},
		"rsa_sign": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `rsa_sign` is not implement"))
			return nil, nil
		},
		"dsa_do_sign": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `dsa_do_sign` is not implement"))
			return nil, nil
		},
		"gunzip": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `gunzip` is not implement"))
			return nil, nil
		},
		"gzip": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `gzip` is not implement"))
			return nil, nil
		},
		"DES": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			panic(fmt.Sprintf("method `DES` is not implement"))
			return nil, nil
		},

		"include": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			name := params.getParamByNumber(0, "").String()
			//if lib, ok := libs[name]; ok {
			//	vm.ExecYakCode("", lib)
			//}
			fPath := path.Join(engine.naslLibsPath, name)
			recoverPath := engine.compiler.SetSourceCodeFilePath(fPath)
			defer func() { recoverPath() }()
			codes, err := os.ReadFile(fPath)
			if err != nil {
				return nil, err
			}
			err = engine.Eval(string(codes))

			if err != nil {
				return nil, err
			}
			return nil, nil
		},

		"new_preference": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			name := params.getParamByName("name").AsString()
			typ := params.getParamByName("typ").AsString()
			value := params.getParamByName("value").AsString()
			engine.scriptObj.Preferences[name] = map[string]string{"type": typ, "value": value}
			return nil, nil
		},
		"dump": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			v := make([]interface{}, 0)
			forEachParams(params, func(value *yakvm.Value) {
				v = append(v, value.Value)
			})
			spew.Dump(v...)
			return nil, nil
		},
		"assert": func(engine *Engine, params *NaslBuildInMethodParam) (interface{}, error) {
			b := params.getParamByNumber(0).Bool()
			msg := params.getParamByNumber(1).String()
			if !b {
				panic(msg)
			}
			return nil, nil
		},
	}
	for name, method := range naslLib {
		NaslLib[name] = func(name string, m NaslBuildInMethod) func(engine *Engine, params *NaslBuildInMethodParam) interface{} {
			return func(engine *Engine, params *NaslBuildInMethodParam) interface{} {
				//defer func() {
				//	if e := recover(); e != nil {
				//		log.Errorf("call function `%s` panic error: %v", name, e)
				//	}
				//}()
				res, err := m(engine, params)
				if err != nil {
					log.Errorf("call build in function `%s` error: %v", name, err)
					return res
				}
				return res
			}
		}(name, method)
	}
}

func GetNaslLibKeys() map[string]interface{} {
	res := make(map[string]interface{})
	for k, _ := range NaslLib {
		res[k] = struct {
		}{}
	}
	return res
}
