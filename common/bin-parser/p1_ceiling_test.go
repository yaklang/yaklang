package bin_parser

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/rules"
)

var leftoverRawRe = regexp.MustCompile(`(?m)^\s+(Body|Rest|Payload|Ciphertext|Stub|MsgGlobal|Section|Optional|TPDU|Query|Schema): raw\s*$`)
var namedScalarRe = regexp.MustCompile(`(?m)^\s+[A-Za-z][^:\n]{0,40}:\s*(uint\d+|int\d+|string|raw,\d+|"del:)`)
var typeArmRe = regexp.MustCompile(`(?i)(if |switch ).{0,160}(Type|typ|Command|cmd|Opcode|Kind|Code|Prefix|Status|Tag|Magic|Version|op ==|op ==)`)

func ruleKey(ruleFile string) string {
	k := strings.TrimSuffix(ruleFile, ".yaml")
	return strings.ReplaceAll(k, "/", ".")
}

func failCount(ruleFile string) int {
	key := ruleKey(ruleFile)
	names := map[string]struct{}{}
	for _, c := range p1FailCases {
		if c.rule == key {
			names[c.name] = struct{}{}
		}
	}
	if n := len(names); n > 0 {
		return n
	}
	return extraFailCount[key]
}

// extraFailCount covers P0-owned rules that P1 names reuse (parseMustFail live in p0_*_test.go).
var extraFailCount = map[string]int{
	"application-layer.dhcp":    4,
	"application-layer.mysql":   3,
	"application-layer.redis":   3,
	"application-layer.http":    3,
	"application-layer.dns":     3,
	"application-layer.snmp":    3,
	"application-layer.nbss":    3,
	"application-layer.socks5":  3,
	"application-layer.ntlm":    9,
	"application-layer.kerberos": 3,
	"application-layer.ldap":    3,
	"application-layer.ssh":     3,
	"application-layer.quic":    3,
	"application-layer.dcerpc":  3,
	"application-layer.msrdp":   3,
	"application-layer.spnego":  3,
	"application-layer.tls":     3,
	"application-layer.http2":   3,
	"application-layer.smb":     3,
	"application-layer.smb2":    3,
	"internet_control_message_protocol_v6": 3,
}

func schemaCeiling(ruleFile, opaqueRaw string) int {
	b, err := rules.RuleFS.ReadFile(ruleFile)
	if err != nil {
		return 0
	}
	text := string(b)
	leftovers := leftoverRawRe.FindAllStringSubmatch(text, -1)
	opaque := strings.ToLower(opaqueRaw)
	leftoverOK := true
	for _, m := range leftovers {
		if opaque == "" || !strings.Contains(opaque, strings.ToLower(m[1])) {
			leftoverOK = false
			break
		}
	}
	hasList := strings.Contains(text, "list: true") && (strings.Contains(text, "NewElement") || strings.Contains(text, "for {"))
	named := len(namedScalarRe.FindAllString(text, -1))
	hasArms := typeArmRe.MatchString(text) || strings.Contains(text, "if ") || strings.Contains(text, "switch ")

	if hasList {
		if leftoverOK {
			return 25
		}
		return 20
	}
	if leftoverOK && named >= 2 && hasArms {
		if named >= 5 || hasList {
			return 25
		}
		return 20
	}
	if leftoverOK && named >= 3 {
		return 20
	}
	if hasArms && named >= 1 {
		return 15
	}
	if named >= 2 {
		return 8
	}
	if named >= 1 && leftoverOK {
		return 8
	}
	return 0
}

func testsCeiling(ruleFile string, hasEth bool, portDispatched bool) int {
	n := failCount(ruleFile)
	stacked := hasEth || !portDispatched
	switch {
	case n < 1:
		return 0
	case n < 3:
		return 6
	case stacked:
		return 20
	default:
		return 12
	}
}

func trafficCeiling(sample string, hasEth bool) int {
	switch sample {
	case "L4":
		return 8
	case "L3":
		return 8
	case "L1", "L2":
		if hasEth {
			return 25
		}
		return 15
	default:
		return 0
	}
}

func p1MustChildNames() map[string]bool {
	out := map[string]bool{}
	root := "common/bin-parser"
	if _, err := os.Stat(root); err != nil {
		root = "."
	}
	matches, _ := filepath.Glob(filepath.Join(root, "*_test.go"))
	fset := token.NewFileSet()
	for _, path := range matches {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			id, ok := call.Fun.(*ast.Ident)
			if !ok || (id.Name != "mustChild" && id.Name != "parseEthernet") {
				return true
			}
			for _, a := range call.Args {
				bl, ok := a.(*ast.BasicLit)
				if !ok || bl.Kind != token.STRING {
					continue
				}
				s, err := strconv.Unquote(bl.Value)
				if err == nil && s != "" {
					out[s] = true
					if s == "HTTP Request" || s == "HTTP Response" {
						out["HTTP"] = true
					}
					if s == "TLS Record" || s == "Handshake" || s == "ClientHello" {
						out["TLS"] = true
					}
				}
			}
			return true
		})
	}
	return out
}

func yamlRootNodes(ruleFile string) []string {
	b, err := rules.RuleFS.ReadFile(ruleFile)
	if err != nil {
		return nil
	}
	var names []string
	inPkg := false
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "Package:") {
			inPkg = true
			continue
		}
		if !inPkg {
			continue
		}
		if len(line) > 2 && line[0] != ' ' && line[0] != '\t' && strings.Contains(line, ":") {
			break
		}
		trim := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(trim, ":") {
			name := strings.TrimSuffix(trim, ":")
			if name != "" && !strings.Contains(name, " ") {
				names = append(names, name)
			}
		}
		if strings.HasPrefix(line, "    ") && strings.Contains(line, "import:") {
			continue
		}
	}
	return names
}

func hasEthernetMustChild(sc ProtocolScorecard, nodes map[string]bool) bool {
	switch {
	case strings.Contains(sc.Rule, "/tls.yaml") && nodes["TLS"]:
		return true
	case strings.Contains(sc.Rule, "/http.yaml") && (nodes["HTTP"] || nodes["HTTP Request"] || nodes["SSDP"]):
		return true
	case strings.Contains(sc.Rule, "/http2.yaml") && (nodes["HTTP2"] || nodes["HTTP/2"]):
		return true
	case strings.Contains(sc.Rule, "/quic.yaml") && nodes["QUIC"]:
		return true
	case strings.Contains(sc.Rule, "/msrdp.yaml") && (nodes["TPKT"] || nodes["RDP"]):
		return true
	case strings.Contains(sc.Rule, "/ldap.yaml") && (nodes["LDAPMessage"] || nodes["LDAP"]):
		return true
	case strings.Contains(sc.Rule, "/ssh.yaml") && nodes["SSH"]:
		return true
	case strings.Contains(sc.Rule, "/mysql.yaml") && (nodes["MySQLPacket"] || nodes["MySQL"]):
		return true
	case strings.Contains(sc.Rule, "/redis.yaml") && nodes["Redis"]:
		return true
	case strings.Contains(sc.Rule, "/spnego.yaml") && nodes["SPNEGO"]:
		return true
	case strings.Contains(sc.Rule, "dcerpc.yaml") && nodes["DCERPC"]:
		return true
	}
	for _, n := range yamlRootNodes(sc.Rule) {
		if nodes[n] {
			return true
		}
	}
	return nodesHitByEvidence(sc, nodes)
}

func nodesHitByEvidence(sc ProtocolScorecard, nodes map[string]bool) bool {
	for name := range nodes {
		if name == "" {
			continue
		}
		if strings.Contains(sc.Evidence, name) {
			return true
		}
		if strings.Contains(sc.Name, name) {
			return true
		}
	}
	aliases := map[string]string{
		"IKEv1": "IKE", "IKEv2": "IKE", "NAT-T": "NATT",
		"FTP-DATA": "FTPData", "VNC/RFB": "VNC", "JSON-RPC": "JSONRPC",
		"Python pickle": "Pickle", "Jenkins remoting": "Jenkins",
		"Rsync daemon": "Rsync", "Zabbix agent": "Zabbix",
		"SaltStack": "Salt", ".NET Remoting": "NetRemoting",
		"Hessian2": "Hessian", "PHP serialize": "PHPSer",
		"IIOP/GIOP": "GIOP", "ONC RPC": "ONCRPC", "NFS": "ONCRPC",
		"TACACS+": "TACACS", "IPsec AH": "AH", "IPsec ESP": "ESP",
		"Ethernet 802.2": "LLC", "Ethernet 802.3": "LLC", "Ethernet SNAP": "LLC",
		"IEEE 802.1ad QinQ": "QinQ", "PPPoE Discovery": "PPPoEDiscovery",
		"PPPoE Session": "PPPoE", "ICMPv6 NDP": "ICMPv6",
		"mDNS": "MDNS", "NBT DG": "NBTDG", "WKSSVC": "DCERPC",
		"SPOOLSS": "DCERPC", "ATSVC": "DCERPC", "IObjectExporter": "DCERPC",
		"BOOTP": "DHCP", "HTTP/3": "QUIC", "SFTP": "SSH",
		"CLDAP": "LDAPMessage", "MariaDB": "MySQLPacket",
		"Redis Sentinel/Cluster": "Redis", "LDAP paged/SASL": "LDAPMessage",
		"GSS-API": "SPNEGO", "TPKT": "TPKT", "BER": "BER Element",
		"IEEE 802.11": "Dot11", "WPA/RSN": "RSN",
		"Linux SLL": "LinuxSLL", "SDP": "SDP",
	}
	if n, ok := aliases[sc.Name]; ok && nodes[n] {
		return true
	}
	return false
}
