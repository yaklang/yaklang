package stream_parser

import (
	"bytes"
	"context"
	"fmt"
	"hash/crc32"
	"reflect"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

type YakNode struct {
	origin               *base.Node
	Process              func() any
	Result               func() any
	Name                 string
	SetCfg               func(k string, v any)
	GetCfg               func(k string) any
	AppendNode           func(d *YakNode)
	ForEachChild         func(f func(child *YakNode))
	GetParent            func() *YakNode
	GetSubNode           func(name string) *YakNode
	GetRemainingSpace    func() uint64
	CalcNodeResultLength func() uint64
	NewElement           func() *YakNode

	SetChildren       func([]*YakNode)
	GetChildren       func() []*YakNode
	Length            func(uint ...string) uint64
	SetMaxLength      func(l uint64, uint ...string)
	GetMaxLength      func(uint ...string) uint64
	HasMaxLength      func() bool
	NewSubNode        func(datas ...any) *YakNode
	NewUnknownNode    func(name ...string) *YakNode
	NewEmptyNode      func(name ...string) *YakNode
	ProcessSubNode    func(name string) any
	TryProcessSubNode func(name string) (any, map[string]any)
	ProcessByType     func(datas ...any) any
	TryProcessByType  func(datas ...string) (any, map[string]any)
	AddInfo           func(key string, v any)
	GetInfo           func(key string) any
}

func ConvertToYakNode(node *base.Node, operator func(node *base.Node) (func(bool), error)) *YakNode {
	getRootNode := func(key string) *YakNode { // 需要处理mapData
		rootMap := node.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)
		if v, ok := rootMap[key]; ok {
			return ConvertToYakNode(v, operator)
		}
		panic("not found root node " + key)
	}
	yakNode := &YakNode{}
	yakNode.origin = node
	yakNode.AddInfo = func(key string, v any) {
		if node.Cfg.Has("additionInfo") {
			additionInfo := node.Cfg.GetItem("additionInfo").(map[string]any)
			additionInfo[key] = v
		} else {
			node.Cfg.SetItem("additionInfo", map[string]any{
				key: v,
			})
		}
	}
	yakNode.GetInfo = func(key string) any {
		if node.Cfg.Has("additionInfo") {
			additionInfo := node.Cfg.GetItem("additionInfo").(map[string]any)
			return additionInfo[key]
		}
		return nil
	}
	yakNode.ProcessSubNode = func(name string) any {
		return yakNode.GetSubNode(name).Process()
	}
	yakNode.TryProcessSubNode = func(name string) (result any, response map[string]any) {
		typeNode := yakNode.GetSubNode(name)
		response = map[string]any{
			"OK":       false,
			"Message":  "",
			"Save":     func() {},
			"Recovery": func() {},
		}
		defer func() {
			if e := recover(); e != nil {
				response["Message"] = fmt.Sprintf("%v", e)
			}
		}()

		//copyNode := node.origin.Copy()
		//copyYakNode := ConvertToYakNode(copyNode, operator)
		yakNode.AppendNode(typeNode)
		copyNode := yakNode.origin.Children[len(yakNode.origin.Children)-1]
		copyYakNode := ConvertToYakNode(copyNode, operator)
		//err := appendNode(copyNode, yakNode.origin)
		//if err != nil {
		//	response["Message"] = err.Error()
		//	return
		//}
		deferFun, err := operator(copyNode)
		response["Save"] = func() {
			deferFun(false)
		}
		response["GetNode"] = func() any {
			return copyYakNode
		}
		response["Recovery"] = func() {
			deferFun(true)
			yakNode.origin.Children = yakNode.origin.Children[:len(yakNode.origin.Children)-1]
		}
		if err != nil {
			response["Message"] = err.Error()
			return nil, response
		}

		result = copyYakNode.Result()
		response["Result"] = result
		response["OK"] = true
		return result, response
	}
	yakNode.GetMaxLength = func(uints ...string) uint64 {
		n := getMulti(yakNode.origin, uints...)
		l, err := getNodeLength(yakNode.origin)
		if err != nil {
			panic(err)
		}
		return l / n
	}
	// A stream with no declared end is different from a bounded empty value.
	// Rules must not allocate the sentinel returned by GetMaxLength as padding.
	yakNode.HasMaxLength = func() bool {
		_, bounded, err := parseLengthByLengthConfig(yakNode.origin)
		if err != nil {
			panic(err)
		}
		return bounded
	}
	yakNode.NewUnknownNode = func(datas ...string) *YakNode {
		name := utils.InterfaceToString(utils.GetLastElement(datas))
		unknownNode := ConvertToYakNode(&base.Node{
			Name:   "Unknown",
			Origin: "raw",
			Cfg:    base.NewConfig(yakNode.origin.Cfg),
			Ctx:    yakNode.origin.Ctx,
		}, operator)
		err := appendNode(node, unknownNode.origin)
		if err != nil {
			panic(err)
		}
		if name != "" {
			utils.GetLastElement(node.Children).Name = name
		}
		return ConvertToYakNode(utils.GetLastElement(node.Children), operator)
	}
	yakNode.NewEmptyNode = func(datas ...string) *YakNode {
		name := utils.InterfaceToString(utils.GetLastElement(datas))
		unknownNode := ConvertToYakNode(&base.Node{
			Name:   "Empty",
			Origin: "raw",
			Cfg:    base.NewConfig(yakNode.origin.Cfg),
			Ctx:    yakNode.origin.Ctx,
		}, operator)
		err := appendNode(node, unknownNode.origin)
		if err != nil {
			panic(err)
		}
		if name != "" {
			utils.GetLastElement(node.Children).Name = name
		}
		return ConvertToYakNode(utils.GetLastElement(node.Children), operator)
	}
	yakNode.SetMaxLength = func(l uint64, uints ...string) {
		n := getMulti(yakNode.origin, uints...)
		node.Cfg.SetItem(CfgLength, l*uint64(n))
	}
	yakNode.ProcessByType = func(datas ...any) any {
		var typeName, nodeName string
		switch len(datas) {
		case 1:
			typeName = utils.InterfaceToString(datas[0])
			nodeName = typeName
		case 2:
			typeName = utils.InterfaceToString(datas[0])
			nodeName = utils.InterfaceToString(datas[1])
		default:
			panic("invalid args")
		}
		typeNode := getRootNode(typeName)
		yakNode.AppendNode(typeNode)
		target := utils.GetLastElement(yakNode.origin.Children)
		target.Name = nodeName
		return ConvertToYakNode(target, operator).Process()
	}
	yakNode.SetChildren = func(nodes []*YakNode) {
		yakNode.origin.Children = nil
		for _, node := range nodes {
			yakNode.origin.Children = append(yakNode.origin.Children, node.origin)
		}
	}
	yakNode.GetChildren = func() []*YakNode {
		res := []*YakNode{}
		for _, node := range yakNode.origin.Children {
			res = append(res, ConvertToYakNode(node, operator))
		}
		return res
	}
	yakNode.GetCfg = func(k string) any {
		return node.Cfg.GetItem(k)
	}
	yakNode.Result = func() any {
		res, err := node.Result()
		if err != nil {
			panic(err)
		}
		return res
	}
	yakNode.NewElement = func() *YakNode {
		element, err := ListNodeNewElement(node)
		if err != nil {
			panic(err)
		}
		return ConvertToYakNode(element, operator)
	}
	yakNode.TryProcessByType = func(datas ...string) (result any, response map[string]any) {
		var typeName, nodeName string
		switch len(datas) {
		case 1:
			typeName = utils.InterfaceToString(datas[0])
			nodeName = typeName
		case 2:
			typeName = utils.InterfaceToString(datas[0])
			nodeName = utils.InterfaceToString(datas[1])
		default:
			panic("invalid args")
		}
		typeNode := getRootNode(typeName)
		response = map[string]any{
			"OK":       false,
			"Message":  "",
			"Save":     func() {},
			"Recovery": func() {},
		}
		defer func() {
			if e := recover(); e != nil {
				response["Message"] = fmt.Sprintf("%v", e)
			}
		}()

		//copyNode := node.origin.Copy()
		//copyYakNode := ConvertToYakNode(copyNode, operator)
		yakNode.AppendNode(typeNode)
		copyNode := yakNode.origin.Children[len(yakNode.origin.Children)-1]
		if nodeName != "" {
			copyNode.Name = nodeName
		}
		copyYakNode := ConvertToYakNode(copyNode, operator)
		//err := appendNode(copyNode, yakNode.origin)
		//if err != nil {
		//	response["Message"] = err.Error()
		//	return
		//}
		deferFun, err := operator(copyNode)
		response["Save"] = func() {
			deferFun(false)
		}
		response["GetNode"] = func() any {
			return copyYakNode
		}
		response["Recovery"] = func() {
			deferFun(true)
			yakNode.origin.Children = yakNode.origin.Children[:len(yakNode.origin.Children)-1]
		}
		if err != nil {
			response["Message"] = err.Error()
			return nil, response
		}

		result = copyYakNode.Result()
		response["Result"] = result
		response["OK"] = true
		return result, response
	}
	yakNode.Process = func() any {
		//defer func() {
		//	if e := recover(); e != nil {
		//		utils.PrintCurrentGoroutineRuntimeStack()
		//	}
		//}()
		deferFun, err := operator(node)
		if err != nil {
			if deferFun != nil {
				deferFun(true)
			}
			panic(err)
		}
		deferFun(false)
		return yakNode.Result()
	}
	yakNode.ForEachChild = func(f func(child *YakNode)) {
		for _, child := range node.Children {
			f(ConvertToYakNode(child, operator))
		}
	}
	yakNode.GetParent = func() *YakNode {
		if node.Cfg.Has(CfgParent) {
			parent := node.Cfg.GetItem(CfgParent).(*base.Node)
			return ConvertToYakNode(parent, operator)
		}
		return nil
	}
	yakNode.GetSubNode = func(name string) *YakNode {
		for _, child := range node.Children {
			if child.Name == name {
				return ConvertToYakNode(child, operator)
			}
		}
		panic(spew.Sprintf("node %s not found", name))
	}
	yakNode.Name = node.Name
	yakNode.SetCfg = func(k string, v any) {
		node.Cfg.SetItem(k, v)
	}
	yakNode.NewSubNode = func(datas ...any) *YakNode {
		var typeName, nodeName string
		switch len(datas) {
		case 1:
			typeName = utils.InterfaceToString(datas[0])
			nodeName = typeName
		case 2:
			typeName = utils.InterfaceToString(datas[0])
			nodeName = utils.InterfaceToString(datas[1])
		default:
			panic("invalid args")
		}
		typeNode := getRootNode(typeName)
		err := appendNode(node, typeNode.origin)
		if err != nil {
			panic(err)
		}
		utils.GetLastElement(node.Children).Name = nodeName
		return ConvertToYakNode(utils.GetLastElement(node.Children), operator)
	}
	yakNode.AppendNode = func(d *YakNode) {
		err := appendNode(node, d.origin)
		if err != nil {
			panic(err)
		}
	}
	yakNode.GetRemainingSpace = func() uint64 {
		res, err := getNodeLength(yakNode.origin)
		if err != nil {
			panic(err)
		}
		return res / getMulti(node)
	}
	yakNode.Length = func(uints ...string) uint64 {
		n := getMulti(yakNode.origin, uints...)
		return CalcNodeConsumedLength(yakNode.origin) / uint64(n)
	}
	return yakNode
}

type operatorInvocation struct {
	bridgeResult *bridgeCallResult
	node         *base.Node
	operator     func(*base.Node) (func(bool), error)
	modes        []string
}

// The ordinary evaluator owns an immutable invocation. A pooled bridge worker
// owns a private invocation whose binding is changed only under an exclusive
// lease. Such programs cannot access this, save callbacks, eval or start tasks.
func (invocation *operatorInvocation) library() map[string]interface{} {
	var this *YakNode
	if invocation.node != nil {
		this = ConvertToYakNode(invocation.node, invocation.operator)
	}
	return map[string]interface{}{
		"parseMemcachedFields": func(profile string) error {
			if invocation.bridgeResult != nil {
				return invocation.bridgeResult.deliver()
			}
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("memcached-fields: structured generation is unsupported")
			}
			return parseMemcachedFields(invocation.node, invocation.operator, profile)
		},
		"parseCassandraFields": func(profile string) error {
			if invocation.bridgeResult != nil {
				return invocation.bridgeResult.deliver()
			}
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("cassandra-fields: structured generation is unsupported")
			}
			return parseCassandraFields(invocation.node, invocation.operator, profile)
		},
		"parseTNSFields": func(profile string) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("tns-fields: structured generation is unsupported")
			}
			return parseTNSFields(invocation.node, invocation.operator, profile)
		},
		"parseTDSFields": func(profile string) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("tds-fields: structured generation is unsupported")
			}
			return parseTDSFields(invocation.node, invocation.operator, profile)
		},
		"parseSMB3TransformFields": func() error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("smb3-transform: structured generation is unsupported")
			}
			return parseSMB3TransformFields(invocation.node, invocation.operator)
		},
		"parseLDAPFields": func(profile string) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("ldap-fields: structured generation is unsupported")
			}
			return parseLDAPFields(invocation.node, invocation.operator, profile)
		},
		"parsePostgreSQLFields": func(profile string) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("postgresql-fields: structured generation is unsupported")
			}
			return parsePostgreSQLFields(invocation.node, invocation.operator, profile)
		},
		"parseMySQLFields": func(profile string) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("mysql-fields: structured generation is unsupported")
			}
			return parseMySQLFields(invocation.node, invocation.operator, profile)
		},
		"parseIMAPFields": func(profile string) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("imap-fields: structured generation is unsupported")
			}
			return parseIMAPFields(invocation.node, invocation.operator, profile)
		},
		"parsePOP3Fields": func(profile string) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("pop3-fields: structured generation is not supported")
			}
			return parsePOP3Fields(invocation.node, invocation.operator, profile)
		},
		"parseSMTPFields": func(data bool) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("smtp-fields: structured generation is not supported")
			}
			return parseSMTPFields(invocation.node, invocation.operator, data)
		},
		"parseMQTTFields": func(level int) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("mqtt-fields: structured generation is not supported")
			}
			return parseMQTTFields(invocation.node, invocation.operator, level)
		},
		"parseKerberosFields": func(tcp bool) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("kerberos-fields: structured generation is not supported")
			}
			return parseKerberosFields(invocation.node, invocation.operator, tcp)
		},
		"parseHTTP2Fields": func(mode string) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("http2-fields: structured generation is not supported")
			}
			return parseHTTP2Fields(invocation.node, invocation.operator, mode)
		},
		"parseSSHPlaintextPacket": func(profile string) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("ssh-plaintext: structured generation is not supported")
			}
			return parseSSHPlaintextPacket(invocation.node, invocation.operator, profile)
		},
		"parseX509CertificateDERPublicKey": func() error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("x509-public-key: structured generation is not supported")
			}
			return parseCertificateFieldTree(invocation.node, invocation.operator, decodeX509CertificateDERPublicKey, "x509-public-key")
		},
		"parseX509CertificateDERExtensions": func() error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("x509-extensions: structured generation is not supported")
			}
			return parseCertificateFieldTree(invocation.node, invocation.operator, decodeX509CertificateDERExtensions, "x509-extensions")
		},
		"parseX509CertificateDER": func() error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("x509-der: structured generation is not supported")
			}
			return parseCertificateFieldTree(invocation.node, invocation.operator, decodeX509CertificateDER, "x509-der")
		},
		"parseTLSChangeCipherSpecRecord": func() error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("tls-ccs: structured generation is not supported")
			}
			return parseTLSChangeCipherSpecRecord(invocation.node, invocation.operator)
		},
		"parseTLS12CertificateAuth": func(messageType uint8) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("tls-certificate-auth: structured generation is not supported")
			}
			return parseTLS12CertificateAuth(invocation.node, invocation.operator, messageType)
		},
		"parseTLS12KeyExchange": func(profile string) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("tls12-key-exchange: structured generation is not supported")
			}
			return parseTLS12KeyExchange(invocation.node, invocation.operator, profile)
		},
		"parseTLS12ControlHandshake": func(messageType uint8) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("tls-control-handshake: structured generation is not supported")
			}
			return parseTLS12ControlHandshake(invocation.node, invocation.operator, messageType)
		},
		"parseTLSCertificateHandshake": func() error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("tls-certificate: structured generation is not supported")
			}
			return parseTLSCertificateHandshake(invocation.node, invocation.operator)
		},
		"parseTLSServerHello": func(record bool) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("tls-server-hello: structured generation is not supported")
			}
			return parseTLSServerHello(invocation.node, record, invocation.operator)
		},
		"parseZlibJSONRecord": func() error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("zlib-json: structured generation is not supported")
			}
			return parseZlibJSONRecord(invocation.node, invocation.operator)
		},
		"parseKCP": func(singleSegment bool) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("kcp: structured generation is not supported")
			}
			return parseKCP(invocation.node, invocation.operator, singleSegment)
		},
		"parseSMTPReply": func() error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("smtp-reply: structured generation is not supported")
			}
			return parseSMTPReply(invocation.node, invocation.operator)
		},
		"parseGQUIC35": func(fromServer bool, clientHello bool) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("gquic35: structured generation is not supported")
			}
			return parseGQUIC35(invocation.node, invocation.operator, fromServer, clientHello)
		},
		"parseBrowserMailslotDatagram": func(strict bool) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("browser-mailslot: structured generation is not supported")
			}
			return parseBrowserMailslotDatagram(invocation.node, strict, invocation.operator)
		},
		"parseHTTP3Stream": func(mode string) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("http3: structured generation is not supported")
			}
			return parseHTTP3Stream(invocation.node, mode, invocation.operator)
		},
		"parseT38SDPAdvertisement": func(h248 bool) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("t38-sdp: structured generation is not supported")
			}
			return parseT38SDPAdvertisement(invocation.node, invocation.operator, h248)
		},
		"parseDoQStream": func(mode string, response bool) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("doq: structured generation is not supported")
			}
			return parseDoQStream(invocation.node, mode, response, invocation.operator)
		},
		"parseEtcdVersionRecord": func() error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("etcd: structured generation is not supported")
			}
			return parseEtcdVersionRecord(invocation.node, invocation.operator)
		},
		"parseS3SignatureV4Request": func() error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("s3: structured generation is not supported")
			}
			return parseS3SignatureV4Request(invocation.node, invocation.operator)
		},
		"parseKubernetesAPIRequest": func() error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("kubernetes: structured generation is not supported")
			}
			return parseKubernetesAPIRequest(invocation.node, invocation.operator)
		},
		"parseWinRMRecord": func() error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("winrm: structured generation is not supported")
			}
			return parseWinRMRecord(invocation.node, invocation.operator)
		},
		"parseGSSAPIToken": func() error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("gssapi: structured generation is not supported")
			}
			return parseGSSAPIToken(invocation.node, invocation.operator)
		},
		"inspectGSSAPIBase64":             inspectGSSAPIBase64,
		"inspectACMEJWS":                  inspectACMEJWS,
		"decodeRedfishServiceRootTarget":  decodeRedfishServiceRootTarget,
		"decodeDockerContainerListTarget": decodeDockerContainerListTarget,
		"parseSMB3Negotiate": func() error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("smb3: structured generation is not supported")
			}
			return parseSMB3Negotiate(invocation.node, invocation.operator)
		},
		"parseRMIRecord": func(mode string) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("rmi: structured generation is not supported")
			}
			return parseRMIRecord(invocation.node, invocation.operator, mode)
		},
		"parseXTPMessage": func() error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("xtp: structured generation is not supported")
			}
			return parseXTPMessage(invocation.node, invocation.operator)
		},
		"parseH225Message": func(mode string) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("h225: structured generation is not supported")
			}
			return parseH225Message(invocation.node, invocation.operator, mode)
		},
		"parseMegacoMessage": func() error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("megaco: structured generation is not supported")
			}
			return parseMegacoMessage(invocation.node, invocation.operator)
		},
		"parseZigbeeFrame": func(hasFCS bool) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("zigbee: structured generation is not supported")
			}
			return parseZigbeeFrame(invocation.node, invocation.operator, hasFCS)
		},
		"parseLATMessage": func() error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("lat: structured generation is not supported")
			}
			return parseLATMessage(invocation.node, invocation.operator)
		},
		"parseGIOPMessage": func(exact bool) error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("giop: structured generation is not supported")
			}
			return parseGIOPMessage(invocation.node, invocation.operator, exact)
		},
		"parseSteamDiscovery": func() error {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode {
				return fmt.Errorf("steam discovery: structured generation is not supported")
			}
			return parseSteamDiscovery(invocation.node, invocation.operator)
		},
		"tryParseWSMP": func() (bool, error) {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode || invocation.node.Ctx.GetBool("wsmpLegacy") {
				return false, nil
			}
			return parseWSMP(invocation.node, invocation.operator)
		},
		"tryParseNATTPayloads": func() (bool, error) {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode || invocation.node.Ctx.GetBool("nattPayloadsLegacy") {
				return false, nil
			}
			return parseNATTPayloads(invocation.node, invocation.operator)
		},
		"tryParseNHRPClients": func() (bool, error) {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode || invocation.node.Ctx.GetBool("nhrpClientsLegacy") {
				return false, nil
			}
			return parseNHRPClients(invocation.node, invocation.operator)
		},
		"tryParseDICOMUserInformation": func() (bool, error) {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode || invocation.node.Ctx.GetBool("dicomUserLegacy") {
				return false, nil
			}
			return parseDICOMUserInformation(invocation.node, invocation.operator)
		},
		"tryParseDICOMPDVList": func() (bool, error) {
			if len(invocation.modes) == 0 || invocation.modes[0] != ParserMode || invocation.node.Ctx.GetBool("dicomPDVLegacy") {
				return false, nil
			}
			return parseDICOMPDVList(invocation.node, invocation.operator)
		},
		"this":                      this,
		"decodeJSONText":            decodeJSONText,
		"decodeXMLRPCText":          decodeXMLRPCText,
		"decodeWSDiscoveryText":     decodeWSDiscoveryText,
		"decodeXMPPText":            decodeXMPPText,
		"decodeHTTPServiceURL":      decodeHTTPServiceURL,
		"decodeC37118Frame":         decodeC37118Frame,
		"validateDCCPDataChecksums": validateDCCPDataChecksums,
		"fcoeCRC32IEEE":             crc32.ChecksumIEEE,
		"decodePrometheusText":      decodePrometheusText,
		"decodeEtherSBusDatagram":   decodeEtherSBusDatagram,
		"decodeIPCompPayload":       decodeIPCompPayload,
		"len": func(i interface{}) int {
			return reflect.ValueOf(i).Len()
		},
		"getNodeResult": func(key string) any {
			targetNode := getNodeByPath(invocation.node, key)
			if targetNode == nil {
				panic("node not found")
			}
			res, err := targetNode.Result()
			if err != nil {
				panic(err)
			}
			return res
		},
		"setCfg": func(key string, value any) {
			targetNode, key := getNodeAttrByPath(invocation.node, key)
			if targetNode == nil {
				panic("node not found")
			}
			targetNode.Cfg.SetItem(key, value)
		},
		"getCfg": func(key string) any {
			targetNode, key := getNodeAttrByPath(invocation.node, key)
			if targetNode == nil {
				panic("node not found")
			}
			return targetNode.Cfg.GetItem(key)
		},
		"deleteCfg": func(key string) {
			targetNode, key := getNodeAttrByPath(invocation.node, key)
			if targetNode == nil {
				panic("node not found")
			}
			targetNode.Cfg.DeleteItem(key)
		},
		"setCtx": func(key string, value any) {
			invocation.node.Ctx.SetItem(key, value)
		},
		"getCtx": func(key string) any {
			return invocation.node.Ctx.GetItem(key)
		},
		"hasCtx": func(key string) bool {
			return invocation.node.Ctx.Has(key)
		},
		"deleteCtx": func(key string) {
			invocation.node.Ctx.DeleteItem(key)
		},
		"getRootNode": func(key string) any { // 需要处理mapData
			rootMap := invocation.node.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)
			if v, ok := rootMap[key]; ok {
				return ConvertToYakNode(v, invocation.operator)
			}
			panic("not found root node " + key)
		},
		"getNode": func(key string) any {
			n := getNodeByPath(invocation.node, key)
			return ConvertToYakNode(n, invocation.operator)
		},
		"getCurrentPosition": func() int {
			buf := invocation.node.Ctx.GetItem("buffer").(*bytes.Buffer)
			return len(buf.Bytes())
		},
		"dump":  spew.Dump,
		"debug": log.Debugf,
	}
}

func ExecOperator(node *base.Node, code string, operator func(node *base.Node) (func(bool), error), modes ...string) error {
	if handled, err := execOperatorPlan(node, code, operator, modes); handled {
		return err
	}
	if handled, err := execRegisteredNativeBridge(node, code, operator, modes); handled {
		return err
	}
	if reusableBridgeOperator(code) {
		return execBridgeOperator(node, code, operator, modes)
	}
	if handled, err := execPreparedOperator(node, code, operator, modes); handled {
		return err
	}
	return execFreshOperator(node, code, operator, modes)
}

func execFreshOperator(node *base.Node, code string, operator func(node *base.Node) (func(bool), error), modes []string) error {
	invocation := &operatorInvocation{node: node, operator: operator, modes: modes}
	engine := antlr4yak.New()
	// Preserve complete returned diagnostics without the optional Go stack dump.
	engine.GetVM().GetConfig().SetSuppressPanicDebugStack(true)
	engine.ImportLibs(invocation.library())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return evalOperatorProgram(ctx, engine, code)
}
func getMulti(node *base.Node, uints ...string) uint64 {
	var uint string
	if len(uints) > 0 {
		uint = utils.InterfaceToString(utils.GetLastElement(uints))
	}
	if uint == "" {
		value, _ := node.Cfg.LookupItem(CfgUnit)
		uint, _ = value.(string)
	}
	if uint == "" {
		uint = "byte"
	}
	n := 0
	switch uint {
	case "byte":
		n = 8
	case "bit":
		n = 1
	default:
		panic("unknown unit " + uint)
	}
	return uint64(n)
}
func ExecParser(node *base.Node) (res any, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", e)
		}
	}()
	code := node.Cfg.GetString("out")
	history := captureExpressionConfigHistory(node)
	node.Cfg.DeleteItem("out")
	defer func() {
		history.restore(node, "out", code)
	}()
	res, err = node.Result()
	if err != nil {
		return nil, err
	}
	engineLib := map[string]interface{}{
		"dump": func(d any) {
			spew.Dump(d)
		},
		"len": func(i interface{}) int {
			return reflect.ValueOf(i).Len()
		},
	}
	engine := antlr4yak.New()
	engine.ImportLibs(engineLib)
	returnV, err := engine.ExecuteAsExpression(code, nil)
	if err != nil {
		return nil, err
	}
	return returnV, nil
}
func ExecOut(node *base.Node) (res *base.NodeValue, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", e)
		}
	}()
	code := node.Cfg.GetString("out")
	history := captureExpressionConfigHistory(node)
	node.Cfg.DeleteItem("out")
	defer func() {
		history.restore(node, "out", code)
	}()
	res, err = node.Result()
	if err != nil {
		return nil, err
	}
	if !node.Ctx.GetBool("outProgramLegacy") && !node.Ctx.GetBool("outScalarLegacy") {
		if value, handled := evalWSMPScalarOut(code, res); handled {
			return newNodeValue(node, value), nil
		}
		if kind := steamScalarOutKind(code); kind != "" {
			if raw, ok := res.Value.([]byte); ok {
				value, err := decodeSteamScalar(raw, kind)
				if err != nil {
					return nil, err
				}
				return newNodeValue(node, value), nil
			}
		}
		if value, handled := evalScalarOut(code, res); handled {
			return newNodeValue(node, value), nil
		}
	}
	engineLib := map[string]interface{}{
		"name": node.Name,
		"dump": func(d any) {
			spew.Dump(d)
		},
		"len": func(i interface{}) int {
			return reflect.ValueOf(i).Len()
		},
		"decodeSteamScalar": func(data []byte, kind string) any {
			value, err := decodeSteamScalar(data, kind)
			if err != nil {
				panic(err)
			}
			return value
		},
		"data":           res,
		"newStructValue": newStructValueAny,
		"newListValue":   newListNodeValue,
		"newValue":       newValueAny,
		"node":           node,
	}
	engine := antlr4yak.New()
	engine.ImportLibs(engineLib)
	var returnV any
	if node.Ctx.GetBool("outProgramLegacy") {
		// Diagnostic oracle for expression/result differential tests. Imports
		// receive this explicit caller setting through the normal context path.
		returnV, err = engine.ExecuteAsExpression(code, nil)
	} else {
		returnV, err = evalOutProgram(engine, code)
	}
	if err != nil {
		return nil, err
	}
	v, ok := returnV.(*base.NodeValue)
	if !ok {
		return newNodeValue(node, returnV), nil
	}
	return v, nil
}
func ExecInput(node *base.Node) (res any, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", e)
		}
	}()
	code := node.Cfg.GetString("input")
	history := captureExpressionConfigHistory(node)
	node.Cfg.DeleteItem("input")
	defer func() {
		history.restore(node, "input", code)
	}()
	res, err = node.Result()
	if err != nil {
		return nil, err
	}
	engineLib := map[string]interface{}{
		"dump": spew.Dump,
		"data": res,
	}
	engine := antlr4yak.New()
	engine.ImportLibs(engineLib)
	res, err = engine.ExecuteAsExpression(code, nil)
	if err != nil {
		return nil, err
	}
	return res, nil
}
