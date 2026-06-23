package embed

import (
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/yak/yaklang/lib/builtin"
	"github.com/yaklang/yaklang/common/yak/yaklib"
)

// ModuleImportSpec describes how to import a yaklang module's exports
// into a dynamically generated Go file.
type ModuleImportSpec struct {
	// ModuleName is the yaklang module name (e.g., "poc", "ssa", "cli")
	ModuleName string

	// GoImportPath is the full Go import path (e.g., "github.com/yaklang/yaklang/common/utils/lowhttp/poc")
	GoImportPath string

	// ImportAlias is the alias to use in the import statement (e.g., "poc")
	ImportAlias string

	// ExportExpr is the Go expression that returns the exports map
	// (e.g., "poc.PoCExports", "ssaapi.Exports")
	ExportExpr string

	// IsGlobal indicates this is a global export (no module name prefix)
	IsGlobal bool
}

// moduleRegistry is the static registry of all yaklang modules and their
// real Go package paths. This replaces the fragile AST parsing approach.
var moduleRegistry = map[string]ModuleImportSpec{
	// === Core modules (from yaklib) ===
	"cli": {
		ModuleName:   "cli",
		GoImportPath: "github.com/yaklang/yaklang/common/utils/cli",
		ImportAlias:  "cli",
		ExportExpr:   "cli.CliExports",
	},
	"poc": {
		ModuleName:   "poc",
		GoImportPath: "github.com/yaklang/yaklang/common/utils/lowhttp/poc",
		ImportAlias:  "poc",
		ExportExpr:   "poc.PoCExports",
	},
	"http": {
		ModuleName:   "http",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib/yakhttp",
		ImportAlias:  "yakhttp",
		ExportExpr:   "yakhttp.HttpExports",
	},
	"codec": {
		ModuleName:   "codec",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.CodecExports",
	},

	// === SSA/SyntaxFlow modules ===
	"ssa": {
		ModuleName:   "ssa",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/ssaapi",
		ImportAlias:  "ssaapi",
		ExportExpr:   "ssaapi.YakExports",
	},
	"syntaxflow": {
		ModuleName:   "syntaxflow",
		GoImportPath: "github.com/yaklang/yaklang/common/syntaxflow",
		ImportAlias:  "syntaxflow",
		ExportExpr:   "syntaxflow.YakExports",
	},
	"sfreport": {
		ModuleName:   "sfreport",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/ssaapi/sfreport",
		ImportAlias:  "sfreport",
		ExportExpr:   "sfreport.Exports",
	},

	// === Other common modules ===
	"yakit": {
		ModuleName:   "yakit",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib/loglite",
		ImportAlias:  "loglite",
		ExportExpr:   "loglite.YakitExports",
	},
	"risk": {
		ModuleName:   "risk",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.RiskExports",
	},
	"dnslog": {
		ModuleName:   "dnslog",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.DNSLogExports",
	},
	"csrf": {
		ModuleName:   "csrf",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.CSRFExports",
	},
	"report": {
		ModuleName:   "report",
		GoImportPath: "github.com/yaklang/yaklang/common/yakgrpc/yakit",
		ImportAlias:  "yakit",
		ExportExpr:   "yakit.ReportExports",
	},
	"json": {
		ModuleName:   "json",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.JsonExports",
	},
	"xml": {
		ModuleName:   "xml",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.XMLExports",
	},
	"yaml": {
		ModuleName:   "yaml",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.YamlExports",
	},
	"re": {
		ModuleName:   "re",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.RegexpExport",
	},
	"str": {
		ModuleName:   "str",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.StringsExport",
	},
	"math": {
		ModuleName:   "math",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.MathExport",
	},
	"os": {
		ModuleName:   "os",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.SystemExports",
	},
	"file": {
		ModuleName:   "file",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.FileExport",
	},
	"io": {
		ModuleName:   "io",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.IoExports",
	},
	"sync": {
		ModuleName:   "sync",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.SyncExport",
	},
	"context": {
		ModuleName:   "context",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.ContextExports",
	},
	"time": {
		ModuleName:   "time",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.TimeExports",
	},
	"log": {
		ModuleName:   "log",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.LogExports",
	},
	"env": {
		ModuleName:   "env",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.EnvExports",
	},
	"tcp": {
		ModuleName:   "tcp",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.TcpExports",
	},
	"udp": {
		ModuleName:   "udp",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.UDPExport",
	},
	"dns": {
		ModuleName:   "dns",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.DnsExports",
	},
	"exec": {
		ModuleName:   "exec",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.ExecExports",
	},
	"ssh": {
		ModuleName:   "ssh",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.SSHExports",
	},
	"db": {
		ModuleName:   "db",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.DatabaseExports",
	},
	"js": {
		ModuleName:   "js",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.JSExports",
	},
	"x": {
		ModuleName:   "x",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.FunkExports",
	},
	"smb": {
		ModuleName:   "smb",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.SambaExports",
	},
	"ldap": {
		ModuleName:   "ldap",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.LdapExports",
	},
	"zip": {
		ModuleName:   "zip",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.ZipExports",
	},
	"gzip": {
		ModuleName:   "gzip",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.GzipExports",
	},
	"tls": {
		ModuleName:   "tls",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.TlsExports",
	},
	"mitm": {
		ModuleName:   "mitm",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.MitmExports",
	},
	"fuzz": {
		ModuleName:   "fuzz",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.FuzzExports",
	},
	"fuzzx": {
		ModuleName:   "fuzzx",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.FuzzxExports",
	},
	"httpserver": {
		ModuleName:   "httpserver",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.HttpServeExports",
	},
	"httpool": {
		ModuleName:   "httpool",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.HttpPoolExports",
	},
	"traceroute": {
		ModuleName:   "traceroute",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.TracerouteExports",
	},
	"spacengine": {
		ModuleName:   "spacengine",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.SpaceEngineExports",
	},
	"mmdb": {
		ModuleName:   "mmdb",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.MmdbExports",
	},
	"redis": {
		ModuleName:   "redis",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.RedisExports",
	},
	"rdp": {
		ModuleName:   "rdp",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.RdpExports",
	},
	"bot": {
		ModuleName:   "bot",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.BotExports",
	},

	// === Tools modules ===
	"tools": {
		ModuleName:   "tools",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib/tools",
		ImportAlias:  "tools",
		ExportExpr:   "tools.Exports",
	},
	"synscan": {
		ModuleName:   "synscan",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib/tools",
		ImportAlias:  "tools",
		ExportExpr:   "tools.SynxPortScanExports",
	},
	"finscan": {
		ModuleName:   "finscan",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib/tools",
		ImportAlias:  "tools",
		ExportExpr:   "tools.FinPortScanExports",
	},
	"servicescan": {
		ModuleName:   "servicescan",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib/tools",
		ImportAlias:  "tools",
		ExportExpr:   "tools.FingerprintScanExports",
	},
	"subdomain": {
		ModuleName:   "subdomain",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib/tools",
		ImportAlias:  "tools",
		ExportExpr:   "tools.SubDomainExports",
	},
	"brute": {
		ModuleName:   "brute",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib/tools",
		ImportAlias:  "tools",
		ExportExpr:   "tools.BruterExports",
	},
	"ping": {
		ModuleName:   "ping",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib/tools",
		ImportAlias:  "tools",
		ExportExpr:   "tools.PingExports",
	},

	// === Container module ===
	"container": {
		ModuleName:   "container",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib/container",
		ImportAlias:  "container",
		ExportExpr:   "container.ContainerExports",
	},

	// === Third-party / specialized modules ===
	"nuclei": {
		ModuleName:   "nuclei",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/httptpl",
		ImportAlias:  "httptpl",
		ExportExpr:   "httptpl.Exports",
	},
	"httptpl": {
		ModuleName:   "httptpl",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/httptpl",
		ImportAlias:  "httptpl",
		ExportExpr:   "httptpl.MatchOrExtractExports",
	},
	"crawler": {
		ModuleName:   "crawler",
		GoImportPath: "github.com/yaklang/yaklang/common/crawler",
		ImportAlias:  "crawler",
		ExportExpr:   "crawler.Exports",
	},
	"yso": {
		ModuleName:   "yso",
		GoImportPath: "github.com/yaklang/yaklang/common/yso",
		ImportAlias:  "yso",
		ExportExpr:   "yso.Exports",
	},
	"facades": {
		ModuleName:   "facades",
		GoImportPath: "github.com/yaklang/yaklang/common/facades",
		ImportAlias:  "facades",
		ExportExpr:   "facades.FacadesExports",
	},
	"t3": {
		ModuleName:   "t3",
		GoImportPath: "github.com/yaklang/yaklang/common/t3",
		ImportAlias:  "t3",
		ExportExpr:   "t3.Exports",
	},
	"iiop": {
		ModuleName:   "iiop",
		GoImportPath: "github.com/yaklang/yaklang/common/iiop",
		ImportAlias:  "iiop",
		ExportExpr:   "iiop.Exports",
	},
	"jwt": {
		ModuleName:   "jwt",
		GoImportPath: "github.com/yaklang/yaklang/common/authhack",
		ImportAlias:  "authhack",
		ExportExpr:   "authhack.JWTExports",
	},
	"java": {
		ModuleName:   "java",
		GoImportPath: "github.com/yaklang/yaklang/common/yserx",
		ImportAlias:  "yserx",
		ExportExpr:   "yserx.Exports",
	},
	"hids": {
		ModuleName:   "hids",
		GoImportPath: "github.com/yaklang/yaklang/common/hids",
		ImportAlias:  "hids",
		ExportExpr:   "hids.Exports",
	},
	"systemd": {
		ModuleName:   "systemd",
		GoImportPath: "github.com/yaklang/yaklang/common/systemd",
		ImportAlias:  "systemd",
		ExportExpr:   "systemd.Exports",
	},
	"xpath": {
		ModuleName:   "xpath",
		GoImportPath: "github.com/yaklang/yaklang/common/utils/htmlquery",
		ImportAlias:  "htmlquery",
		ExportExpr:   "htmlquery.Exports",
	},
	"filesys": {
		ModuleName:   "filesys",
		GoImportPath: "github.com/yaklang/yaklang/common/utils/filesys",
		ImportAlias:  "filesys",
		ExportExpr:   "filesys.Exports",
	},
	"fileparser": {
		ModuleName:   "fileparser",
		GoImportPath: "github.com/yaklang/yaklang/common/utils/fileparser",
		ImportAlias:  "fileparser",
		ExportExpr:   "fileparser.Exports",
	},
	"excel": {
		ModuleName:   "excel",
		GoImportPath: "github.com/yaklang/yaklang/common/utils/fileparser/excelparser",
		ImportAlias:  "excelparser",
		ExportExpr:   "excelparser.ExcelExports",
	},
	"xhtml": {
		ModuleName:   "xhtml",
		GoImportPath: "github.com/yaklang/yaklang/common/xhtml",
		ImportAlias:  "xhtml",
		ExportExpr:   "xhtml.Exports",
	},
	"nasl": {
		ModuleName:   "nasl",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/antlr4nasl",
		ImportAlias:  "antlr4nasl",
		ExportExpr:   "antlr4nasl.Exports",
	},
	"dyn": {
		ModuleName:   "dyn",
		GoImportPath: "github.com/yaklang/yaklang/common/yak",
		ImportAlias:  "yak",
		ExportExpr:   "yak.EvalExports",
	},
	"hook": {
		ModuleName:   "hook",
		GoImportPath: "github.com/yaklang/yaklang/common/yak",
		ImportAlias:  "yak",
		ExportExpr:   "yak.HooksExports",
	},
	"judge": {
		ModuleName:   "judge",
		GoImportPath: "github.com/yaklang/yaklang/common/utils/comparer",
		ImportAlias:  "comparer",
		ExportExpr:   "comparer.Exports",
	},
	"dictutil": {
		ModuleName:   "dictutil",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.DictUtilExports",
	},
	"webforest": {
		ModuleName:   "webforest",
		GoImportPath: "github.com/yaklang/yaklang/common/utils/webforest",
		ImportAlias:  "webforest",
		ExportExpr:   "webforest.Exports",
	},
	"jsonstream": {
		ModuleName:   "jsonstream",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.JsonStreamExports",
	},
	"jsonschema": {
		ModuleName:   "jsonschema",
		GoImportPath: "github.com/yaklang/yaklang/common/ai/aid/aitool",
		ImportAlias:  "aitool",
		ExportExpr:   "aitool.SchemaGeneratorExports",
	},
	"netstack": {
		ModuleName:   "netstack",
		GoImportPath: "github.com/yaklang/yaklang/common/netstack_exports",
		ImportAlias:  "netstack_exports",
		ExportExpr:   "netstack_exports.Exports",
	},
	"re2": {
		ModuleName:   "re2",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.Regexp2Export",
	},
	"regen": {
		ModuleName:   "regen",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.RegenExports",
	},
	"bufio": {
		ModuleName:   "bufio",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.BufioExport",
	},
	"timezone": {
		ModuleName:   "timezone",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.TimeZoneExports",
	},
	"filemonitor": {
		ModuleName:   "filemonitor",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.FileMonitorExports",
	},
	"filescanner": {
		ModuleName:   "filescanner",
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.FileScannerExports",
	},
	"pprof": {
		ModuleName:   "pprof",
		GoImportPath: "github.com/yaklang/yaklang/common/utils/pprofutils",
		ImportAlias:  "pprofutils",
		ExportExpr:   "pprofutils.Exports",
	},
	"liteforge": {
		ModuleName:   "liteforge",
		GoImportPath: "github.com/yaklang/yaklang/common/aiforge",
		ImportAlias:  "aiforge",
		ExportExpr:   "aiforge.LiteForgeExport",
	},
	"rag": {
		ModuleName:   "rag",
		GoImportPath: "github.com/yaklang/yaklang/common/yak",
		ImportAlias:  "yak",
		ExportExpr:   "yak.RagExports",
	},
	"ai": {
		ModuleName:   "ai",
		GoImportPath: "github.com/yaklang/yaklang/common/ai",
		ImportAlias:  "ai",
		ExportExpr:   "ai.Exports",
	},
}

// GlobalExportSpecs defines the global exports (no module prefix).
// These are registered via runtimeRegisterYaklibGlobals() instead of runtimeRegisterYaklibModule().
var globalExportSpecs = []struct {
	Name       string
	ImportSpec ModuleImportSpec
}{
	{
		Name: "len",
		ImportSpec: ModuleImportSpec{
			ExportExpr: "runtimeYakBuiltinLen",
		},
	},
	{
		Name: "cap",
		ImportSpec: ModuleImportSpec{
			ExportExpr: "runtimeYakBuiltinCap",
		},
	},
}

// globalBuiltinNames is the set of global builtins that are always supported.
var globalBuiltinNames = map[string]bool{
	"len":       true,
	"cap":       true,
	"print":     true,
	"printf":    true,
	"println":   true,
	"sprintf":   true,
	"sprint":    true,
	"atoi":      true,
	"dump":      true,
	"die":       true,
	"fail":      true,
	"sleep":     true,
	"assert":    true,
	"uuid":      true,
	"timestamp": true,
}

// globalBuiltinImportSpecs maps global builtins to their import specs.
var globalBuiltinImportSpecs = map[string]ModuleImportSpec{
	"sprintf": {
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklang/lib/builtin",
		ImportAlias:  "builtin",
		ExportExpr:   "builtin.YaklangBaseLib",
	},
	"sprint": {
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklang/lib/builtin",
		ImportAlias:  "builtin",
		ExportExpr:   "builtin.YaklangBaseLib",
	},
	"atoi": {
		GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
		ImportAlias:  "yaklib",
		ExportExpr:   "yaklib.GlobalExport",
	},
}

// LookupModuleSpec returns the ModuleImportSpec for a given module name.
func LookupModuleSpec(moduleName string) (ModuleImportSpec, bool) {
	spec, ok := moduleRegistry[strings.TrimSpace(moduleName)]
	return spec, ok
}

func lookupRegisteredGlobalExport(name string) (tableExpr string, importSpec ModuleImportSpec, ok bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ModuleImportSpec{}, false
	}
	if _, exists := yaklib.GlobalExport[name]; exists {
		return "yaklib.GlobalExport", ModuleImportSpec{
			GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
			ImportAlias:  "yaklib",
		}, true
	}
	if _, exists := builtin.YaklangBaseLib[name]; exists {
		return "builtin.YaklangBaseLib", ModuleImportSpec{
			GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklang/lib/builtin",
			ImportAlias:  "builtin",
		}, true
	}
	return "", ModuleImportSpec{}, false
}

// AllModuleNames returns all registered module names, sorted.
func AllModuleNames() []string {
	names := make([]string, 0, len(moduleRegistry))
	for name := range moduleRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
