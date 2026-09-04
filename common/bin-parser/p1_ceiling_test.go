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

// Unbounded leftover blob: "Name: raw" with no ",N" length. Spec-opaque leftovers
// (ICV/Ciphertext/Stub/TPDU/…) may stay raw only when OpaqueRaw names that field.
var leftoverRawRe = regexp.MustCompile(`(?m)^\s+([A-Za-z][^:\n]{0,40}): raw\s*$`)
var namedScalarRe = regexp.MustCompile(`(?m)^\s+[A-Za-z][^:\n]{0,40}:\s*(uint\d+|int\d+|string|raw,\d+|"del:)`)
var typeArmRe = regexp.MustCompile(`ProcessByType\(|ProcessSubNode\("(OPEN|OP_QUERY|OP_MSG|Command|C1|Handshake|AuthenStart|ASF|Int|Endpoint|Headers|BININT1|Version|Protocol|Values|Pairs|Frames|Acks|Topics|Client ID|Integer|Request|Response|Line|Prefix|Value|AuthenStart|ASF)"\)`)

var specOpaqueLeftover = map[string]bool{
	"ICV": true, "Ciphertext": true, "Stub": true, "TPDU": true,
	"RDATA": true, "Key Data": true, "Fragment": true, "Authentication": true,
	"Random": true, "NDR": true, "MsgGlobal": true, "Value": true,
}

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
	incomplete := false
	leftoverOK := true
	for _, m := range leftovers {
		name := strings.TrimSpace(m[1])
		if name == "type" || name == "endian" || name == "parser" {
			continue
		}
		if specOpaqueLeftover[name] && opaque != "" && strings.Contains(opaque, strings.ToLower(name)) {
			continue
		}
		leftoverOK = false
		incomplete = true
	}
	hasList := strings.Contains(text, "list: true") && (strings.Contains(text, "ele.Process()") || strings.Contains(text, "NewElement().Process()") || strings.Contains(text, "n = this.NewElement().Process()") || strings.Contains(text, "list-length-from-field"))
	named := len(namedScalarRe.FindAllString(text, -1))
	hasArms := typeArmRe.MatchString(text) || (strings.Contains(text, "if ") && strings.Contains(text, "ProcessSubNode("))

	// PROTOCOL_DELIVERY 3.1:
	// leftover blob without list/TLV → 8; type/command arms with named PDUs → 15; list/TLV → 20.
	if hasList {
		if leftoverOK && !incomplete {
			return 25
		}
		return 20
	}
	if incomplete {
		if hasArms && named >= 2 {
			return 15
		}
		if named >= 2 {
			return 8
		}
		return 0
	}
	if leftoverOK && named >= 4 && hasArms {
		return 25
	}
	if hasArms && named >= 2 {
		return 15
	}
	if named >= 2 {
		return 8
	}
	if named >= 1 {
		return 8
	}
	return 0
}

func testsCeiling(ruleFile string, hasEth bool, portDispatched bool) int {
	n := failCount(ruleFile)
	rows := successBranchRows(ruleFile)
	switch {
	case n < 1:
		return 0
	case n < 3:
		return 6
	case rows >= 2 && (hasEth || !portDispatched):
		return 20
	case hasEth || !portDispatched:
		return 16
	default:
		return 12
	}
}

// successBranchRows counts t.Run("proto/arm") success-path subtests (not fail-path files).
func successBranchRows(ruleFile string) int {
	key := strings.ToLower(ruleKey(ruleFile))
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(ruleFile), ".yaml"))
	needles := []string{key, base}
	for _, n := range yamlRootNodes(ruleFile) {
		needles = append(needles, strings.ToLower(n))
	}
	root := "common/bin-parser"
	if _, err := os.Stat(root); err != nil {
		root = "."
	}
	matches, _ := filepath.Glob(filepath.Join(root, "*_test.go"))
	fset := token.NewFileSet()
	count := 0
	for _, path := range matches {
		if strings.Contains(strings.ToLower(filepath.Base(path)), "fail") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel == nil || sel.Sel.Name != "Run" {
				return true
			}
			if len(call.Args) < 1 {
				return true
			}
			bl, ok := call.Args[0].(*ast.BasicLit)
			if !ok || bl.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(bl.Value)
			if err != nil {
				return true
			}
			low := strings.ToLower(s)
			if !strings.Contains(low, "/") {
				return true
			}
			for _, nd := range needles {
				if nd != "" && strings.Contains(low, nd) {
					count++
					break
				}
			}
			return true
		})
	}
	return count
}

func rootLastUnboundedRaw(ruleFile string) bool {
	text := mustReadRule(ruleFile)
	inPkg := false
	last := ""
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "Package:") {
			inPkg = true
			continue
		}
		if !inPkg {
			continue
		}
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' && strings.Contains(line, ":") {
			break
		}
		if strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "     ") && strings.Contains(line, ":") {
			last = strings.TrimSpace(line)
		}
	}
	if last == "" || !leftoverRawRe.MatchString("    "+last) {
		return false
	}
	name := strings.TrimSpace(strings.SplitN(last, ":", 2)[0])
	if strings.Contains(text, "list: true") {
		return false
	}
	if strings.Contains(text, `GetSubNode("`+name+`").SetMaxLength`) || strings.Contains(text, "SetMaxLength") {
		return false
	}
	return true
}

func deriveP1Gates(sc ProtocolScorecard, failN int, hasEth, ported bool) ProtocolScorecard {
	_, err := rules.RuleFS.ReadFile(sc.Rule)
	sc.G1 = err == nil
	ev := strings.ToLower(sc.Evidence)
	sc.G2 = strings.Contains(ev, "test") || strings.Contains(ev, "parserule") || strings.Contains(ev, "parseethernet")
	sc.G3 = !rootLastUnboundedRaw(sc.Rule)
	_, tlsErr := rules.RuleFS.ReadFile("application-layer/tls.yaml")
	_, httpErr := rules.RuleFS.ReadFile("application-layer/http.yaml")
	sc.G4 = tlsErr == nil && httpErr == nil
	if strings.Contains(sc.Rule, "msrdp.yaml") {
		sc.G4 = sc.G4 && strings.Contains(strings.ToLower(sc.OpaqueRaw), "tpdu")
	}
	sc.G5 = sc.SampleClass == "L1" || sc.SampleClass == "L2" || sc.SampleClass == "L3"
	sc.G6 = failN >= 1
	sc.G7 = !ported || hasEth
	sc.G8 = true
	return sc
}

func mustReadRule(ruleFile string) string {
	b, err := rules.RuleFS.ReadFile(ruleFile)
	if err != nil {
		return ""
	}
	return string(b)
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
