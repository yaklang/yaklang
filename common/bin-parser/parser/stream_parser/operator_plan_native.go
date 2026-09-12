package stream_parser

import (
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"strconv"
	"strings"
)

// Unbound Go functions, never node-bound library closures. Only these audited
// parser helpers may be lowered; arbitrary callables remain Yak programs.
var planNativeCalls = map[string]any{
	"parseMemcachedFields": parseMemcachedFields, "parseCassandraFields": parseCassandraFields,
	"parseTNSFields": parseTNSFields, "parseTDSFields": parseTDSFields,
	"parseLDAPFields": parseLDAPFields, "parseSMB3TransformFields": parseSMB3TransformFields,
	"parsePostgreSQLFields": parsePostgreSQLFields, "parseMySQLFields": parseMySQLFields,
	"parseIMAPFields": parseIMAPFields, "parsePOP3Fields": parsePOP3Fields,
	"parseSMTPFields": parseSMTPFields, "parseMQTTFields": parseMQTTFields,
	"parseKerberosFields": parseKerberosFields, "parseHTTP2Fields": parseHTTP2Fields,
	"parseSSHPlaintextPacket":        parseSSHPlaintextPacket,
	"parseTLSChangeCipherSpecRecord": parseTLSChangeCipherSpecRecord,
	"parseTLS12CertificateAuth":      parseTLS12CertificateAuth, "parseTLS12KeyExchange": parseTLS12KeyExchange,
	"parseTLS12ControlHandshake": parseTLS12ControlHandshake, "parseTLSCertificateHandshake": parseTLSCertificateHandshake,
	"parseZlibJSONRecord": parseZlibJSONRecord, "parseKCP": parseKCP,
	"parseSMTPReply": parseSMTPReply, "parseGQUIC35": parseGQUIC35,
	"parseEtcdVersionRecord": parseEtcdVersionRecord, "parseS3SignatureV4Request": parseS3SignatureV4Request,
	"parseKubernetesAPIRequest": parseKubernetesAPIRequest, "parseWinRMRecord": parseWinRMRecord,
	"parseGSSAPIToken": parseGSSAPIToken, "parseSMB3Negotiate": parseSMB3Negotiate,
	"parseRMIRecord": parseRMIRecord, "parseXTPMessage": parseXTPMessage,
	"parseH225Message": parseH225Message, "parseMegacoMessage": parseMegacoMessage,
	"parseZigbeeFrame": parseZigbeeFrame, "parseLATMessage": parseLATMessage,
	"parseGIOPMessage": parseGIOPMessage, "parseSteamDiscovery": parseSteamDiscovery,
}

func compileNativeCallPlan(tokens string) *operatorPlan {
	const suffix = " if err != nil { panic ( err ) }"
	if !strings.HasPrefix(tokens, "err = ") || !strings.HasSuffix(tokens, suffix) {
		return nil
	}
	expression := strings.TrimSuffix(strings.TrimPrefix(tokens, "err = "), suffix)
	name, args, ok := strings.Cut(expression, " ( ")
	if !ok || !strings.HasSuffix(args, ")") {
		return nil
	}
	args = strings.TrimSpace(strings.TrimSuffix(args, ")"))
	var call func(*base.Node, func(*base.Node) (func(bool), error)) error
	switch f := planNativeCalls[name].(type) {
	case func(*base.Node, func(*base.Node) (func(bool), error)) error:
		if args != "" {
			return nil
		}
		call = f
	case func(*base.Node, func(*base.Node) (func(bool), error), string) error:
		value, err := strconv.Unquote(args)
		if err != nil {
			return nil
		}
		call = func(n *base.Node, process func(*base.Node) (func(bool), error)) error { return f(n, process, value) }
	case func(*base.Node, func(*base.Node) (func(bool), error), bool) error:
		if args != "true" && args != "false" {
			return nil
		}
		value := args == "true"
		call = func(n *base.Node, process func(*base.Node) (func(bool), error)) error { return f(n, process, value) }
	case func(*base.Node, func(*base.Node) (func(bool), error), int) error:
		value, err := strconv.Atoi(args)
		if err != nil {
			return nil
		}
		call = func(n *base.Node, process func(*base.Node) (func(bool), error)) error { return f(n, process, value) }
	case func(*base.Node, func(*base.Node) (func(bool), error), uint8) error:
		value, err := strconv.ParseUint(args, 0, 8)
		if err != nil {
			return nil
		}
		call = func(n *base.Node, process func(*base.Node) (func(bool), error)) error {
			return f(n, process, uint8(value))
		}
	case func(*base.Node, func(*base.Node) (func(bool), error), bool, bool) error:
		parts := strings.Split(args, " , ")
		if len(parts) != 2 {
			return nil
		}
		for _, v := range parts {
			if v != "true" && v != "false" {
				return nil
			}
		}
		a, b := parts[0] == "true", parts[1] == "true"
		call = func(n *base.Node, process func(*base.Node) (func(bool), error)) error { return f(n, process, a, b) }
	default:
		return nil
	}
	return &operatorPlan{kind: "native-call", run: func(e *planExecution) bool {
		e.at(expression)
		err := call(e.this.origin, e.this.operator)
		if err != nil {
			e.at("panic(err)")
			panic(err)
		}
		return true
	}}
}
