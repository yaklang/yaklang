package main

import "C"

import (
	_ "unsafe"
	ai "github.com/yaklang/yaklang/common/ai"
	aiforge "github.com/yaklang/yaklang/common/aiforge"
	aitool "github.com/yaklang/yaklang/common/ai/aid/aitool"
	antlr4nasl "github.com/yaklang/yaklang/common/yak/antlr4nasl"
	aotlib "github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/aotlib"
	authhack "github.com/yaklang/yaklang/common/authhack"
	binx "github.com/yaklang/yaklang/common/binx"
	chaosmaker "github.com/yaklang/yaklang/common/chaosmaker"
	cli "github.com/yaklang/yaklang/common/utils/cli"
	comparer "github.com/yaklang/yaklang/common/utils/comparer"
	container "github.com/yaklang/yaklang/common/yak/yaklib/container"
	crawler "github.com/yaklang/yaklang/common/crawler"
	crawlerx "github.com/yaklang/yaklang/common/crawlerx"
	excelparser "github.com/yaklang/yaklang/common/utils/fileparser/excelparser"
	facades "github.com/yaklang/yaklang/common/facades"
	fileparser "github.com/yaklang/yaklang/common/utils/fileparser"
	filesys "github.com/yaklang/yaklang/common/utils/filesys"
	hids "github.com/yaklang/yaklang/common/hids"
	htmlquery "github.com/yaklang/yaklang/common/utils/htmlquery"
	httptpl "github.com/yaklang/yaklang/common/yak/httptpl"
	iiop "github.com/yaklang/yaklang/common/iiop"
	netstack_exports "github.com/yaklang/yaklang/common/netstack_exports"
	omnisearch "github.com/yaklang/yaklang/common/omnisearch"
	openapi "github.com/yaklang/yaklang/common/openapi"
	poc "github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	pprofutils "github.com/yaklang/yaklang/common/utils/pprofutils"
	sca "github.com/yaklang/yaklang/common/sca"
	sfreport "github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	simulator "github.com/yaklang/yaklang/common/simulator"
	ssaapi "github.com/yaklang/yaklang/common/yak/ssaapi"
	ssaconfig "github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	ssaproject "github.com/yaklang/yaklang/common/yak/ssaproject"
	syntaxflow "github.com/yaklang/yaklang/common/syntaxflow"
	syntaxflow_scan "github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	systemd "github.com/yaklang/yaklang/common/systemd"
	t3 "github.com/yaklang/yaklang/common/t3"
	tools "github.com/yaklang/yaklang/common/yak/yaklib/tools"
	webforest "github.com/yaklang/yaklang/common/utils/webforest"
	xhtml "github.com/yaklang/yaklang/common/xhtml"
	yak "github.com/yaklang/yaklang/common/yak"
	yakdiff "github.com/yaklang/yaklang/common/utils/yakgit/yakdiff"
	yakgit "github.com/yaklang/yaklang/common/utils/yakgit"
	yakhttp "github.com/yaklang/yaklang/common/yak/yaklib/yakhttp"
	yakit "github.com/yaklang/yaklang/common/yakgrpc/yakit"
	yaklib "github.com/yaklang/yaklang/common/yak/yaklib"
	yakshim "github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/shim"
	yserx "github.com/yaklang/yaklang/common/yserx"
	yso "github.com/yaklang/yaklang/common/yso"
)

func init() {}

//export yak_register_globals
func yak_register_globals() {
	registerRuntimeGlobals()
}

//go:noinline
func yakPrunedModulePanic(module string) {
	panic("yaklib module was pruned at link time but is still reachable: " + module +
		" (the compiler's used-module closure is missing it)")
}

//export yakUnusedModuleStub
//go:noinline
func yakUnusedModuleStub() { yakPrunedModulePanic("<unknown>") }

//export yakPrunedModuleStub_ai
//go:noinline
func yakPrunedModuleStub_ai() { yakPrunedModulePanic("ai") }

//export yakPrunedModuleStub_bin
//go:noinline
func yakPrunedModuleStub_bin() { yakPrunedModulePanic("bin") }

//export yakPrunedModuleStub_bot
//go:noinline
func yakPrunedModuleStub_bot() { yakPrunedModulePanic("bot") }

//export yakPrunedModuleStub_brute
//go:noinline
func yakPrunedModuleStub_brute() { yakPrunedModulePanic("brute") }

//export yakPrunedModuleStub_bufio
//go:noinline
func yakPrunedModuleStub_bufio() { yakPrunedModulePanic("bufio") }

//export yakPrunedModuleStub_cli
//go:noinline
func yakPrunedModuleStub_cli() { yakPrunedModulePanic("cli") }

//export yakPrunedModuleStub_codec
//go:noinline
func yakPrunedModuleStub_codec() { yakPrunedModulePanic("codec") }

//export yakPrunedModuleStub_container
//go:noinline
func yakPrunedModuleStub_container() { yakPrunedModulePanic("container") }

//export yakPrunedModuleStub_context
//go:noinline
func yakPrunedModuleStub_context() { yakPrunedModulePanic("context") }

//export yakPrunedModuleStub_crawler
//go:noinline
func yakPrunedModuleStub_crawler() { yakPrunedModulePanic("crawler") }

//export yakPrunedModuleStub_crawlerx
//go:noinline
func yakPrunedModuleStub_crawlerx() { yakPrunedModulePanic("crawlerx") }

//export yakPrunedModuleStub_csrf
//go:noinline
func yakPrunedModuleStub_csrf() { yakPrunedModulePanic("csrf") }

//export yakPrunedModuleStub_db
//go:noinline
func yakPrunedModuleStub_db() { yakPrunedModulePanic("db") }

//export yakPrunedModuleStub_dictutil
//go:noinline
func yakPrunedModuleStub_dictutil() { yakPrunedModulePanic("dictutil") }

//export yakPrunedModuleStub_diff
//go:noinline
func yakPrunedModuleStub_diff() { yakPrunedModulePanic("diff") }

//export yakPrunedModuleStub_dns
//go:noinline
func yakPrunedModuleStub_dns() { yakPrunedModulePanic("dns") }

//export yakPrunedModuleStub_dnslog
//go:noinline
func yakPrunedModuleStub_dnslog() { yakPrunedModulePanic("dnslog") }

//export yakPrunedModuleStub_dyn
//go:noinline
func yakPrunedModuleStub_dyn() { yakPrunedModulePanic("dyn") }

//export yakPrunedModuleStub_env
//go:noinline
func yakPrunedModuleStub_env() { yakPrunedModulePanic("env") }

//export yakPrunedModuleStub_excel
//go:noinline
func yakPrunedModuleStub_excel() { yakPrunedModulePanic("excel") }

//export yakPrunedModuleStub_exec
//go:noinline
func yakPrunedModuleStub_exec() { yakPrunedModulePanic("exec") }

//export yakPrunedModuleStub_facades
//go:noinline
func yakPrunedModuleStub_facades() { yakPrunedModulePanic("facades") }

//export yakPrunedModuleStub_file
//go:noinline
func yakPrunedModuleStub_file() { yakPrunedModulePanic("file") }

//export yakPrunedModuleStub_filemonitor
//go:noinline
func yakPrunedModuleStub_filemonitor() { yakPrunedModulePanic("filemonitor") }

//export yakPrunedModuleStub_fileparser
//go:noinline
func yakPrunedModuleStub_fileparser() { yakPrunedModulePanic("fileparser") }

//export yakPrunedModuleStub_filescanner
//go:noinline
func yakPrunedModuleStub_filescanner() { yakPrunedModulePanic("filescanner") }

//export yakPrunedModuleStub_filesys
//go:noinline
func yakPrunedModuleStub_filesys() { yakPrunedModulePanic("filesys") }

//export yakPrunedModuleStub_finscan
//go:noinline
func yakPrunedModuleStub_finscan() { yakPrunedModulePanic("finscan") }

//export yakPrunedModuleStub_fuzz
//go:noinline
func yakPrunedModuleStub_fuzz() { yakPrunedModulePanic("fuzz") }

//export yakPrunedModuleStub_fuzzx
//go:noinline
func yakPrunedModuleStub_fuzzx() { yakPrunedModulePanic("fuzzx") }

//export yakPrunedModuleStub_git
//go:noinline
func yakPrunedModuleStub_git() { yakPrunedModulePanic("git") }

//export yakPrunedModuleStub_gzip
//go:noinline
func yakPrunedModuleStub_gzip() { yakPrunedModulePanic("gzip") }

//export yakPrunedModuleStub_hids
//go:noinline
func yakPrunedModuleStub_hids() { yakPrunedModulePanic("hids") }

//export yakPrunedModuleStub_hook
//go:noinline
func yakPrunedModuleStub_hook() { yakPrunedModulePanic("hook") }

//export yakPrunedModuleStub_http
//go:noinline
func yakPrunedModuleStub_http() { yakPrunedModulePanic("http") }

//export yakPrunedModuleStub_httpool
//go:noinline
func yakPrunedModuleStub_httpool() { yakPrunedModulePanic("httpool") }

//export yakPrunedModuleStub_httpserver
//go:noinline
func yakPrunedModuleStub_httpserver() { yakPrunedModulePanic("httpserver") }

//export yakPrunedModuleStub_httptpl
//go:noinline
func yakPrunedModuleStub_httptpl() { yakPrunedModulePanic("httptpl") }

//export yakPrunedModuleStub_iiop
//go:noinline
func yakPrunedModuleStub_iiop() { yakPrunedModulePanic("iiop") }

//export yakPrunedModuleStub_io
//go:noinline
func yakPrunedModuleStub_io() { yakPrunedModulePanic("io") }

//export yakPrunedModuleStub_java
//go:noinline
func yakPrunedModuleStub_java() { yakPrunedModulePanic("java") }

//export yakPrunedModuleStub_js
//go:noinline
func yakPrunedModuleStub_js() { yakPrunedModulePanic("js") }

//export yakPrunedModuleStub_json
//go:noinline
func yakPrunedModuleStub_json() { yakPrunedModulePanic("json") }

//export yakPrunedModuleStub_jsonschema
//go:noinline
func yakPrunedModuleStub_jsonschema() { yakPrunedModulePanic("jsonschema") }

//export yakPrunedModuleStub_jsonstream
//go:noinline
func yakPrunedModuleStub_jsonstream() { yakPrunedModulePanic("jsonstream") }

//export yakPrunedModuleStub_judge
//go:noinline
func yakPrunedModuleStub_judge() { yakPrunedModulePanic("judge") }

//export yakPrunedModuleStub_jwt
//go:noinline
func yakPrunedModuleStub_jwt() { yakPrunedModulePanic("jwt") }

//export yakPrunedModuleStub_ldap
//go:noinline
func yakPrunedModuleStub_ldap() { yakPrunedModulePanic("ldap") }

//export yakPrunedModuleStub_liteforge
//go:noinline
func yakPrunedModuleStub_liteforge() { yakPrunedModulePanic("liteforge") }

//export yakPrunedModuleStub_log
//go:noinline
func yakPrunedModuleStub_log() { yakPrunedModulePanic("log") }

//export yakPrunedModuleStub_math
//go:noinline
func yakPrunedModuleStub_math() { yakPrunedModulePanic("math") }

//export yakPrunedModuleStub_mitm
//go:noinline
func yakPrunedModuleStub_mitm() { yakPrunedModulePanic("mitm") }

//export yakPrunedModuleStub_mmdb
//go:noinline
func yakPrunedModuleStub_mmdb() { yakPrunedModulePanic("mmdb") }

//export yakPrunedModuleStub_nasl
//go:noinline
func yakPrunedModuleStub_nasl() { yakPrunedModulePanic("nasl") }

//export yakPrunedModuleStub_netstack
//go:noinline
func yakPrunedModuleStub_netstack() { yakPrunedModulePanic("netstack") }

//export yakPrunedModuleStub_nuclei
//go:noinline
func yakPrunedModuleStub_nuclei() { yakPrunedModulePanic("nuclei") }

//export yakPrunedModuleStub_omnisearch
//go:noinline
func yakPrunedModuleStub_omnisearch() { yakPrunedModulePanic("omnisearch") }

//export yakPrunedModuleStub_openapi
//go:noinline
func yakPrunedModuleStub_openapi() { yakPrunedModulePanic("openapi") }

//export yakPrunedModuleStub_os
//go:noinline
func yakPrunedModuleStub_os() { yakPrunedModulePanic("os") }

//export yakPrunedModuleStub_ping
//go:noinline
func yakPrunedModuleStub_ping() { yakPrunedModulePanic("ping") }

//export yakPrunedModuleStub_poc
//go:noinline
func yakPrunedModuleStub_poc() { yakPrunedModulePanic("poc") }

//export yakPrunedModuleStub_pprof
//go:noinline
func yakPrunedModuleStub_pprof() { yakPrunedModulePanic("pprof") }

//export yakPrunedModuleStub_rag
//go:noinline
func yakPrunedModuleStub_rag() { yakPrunedModulePanic("rag") }

//export yakPrunedModuleStub_rdp
//go:noinline
func yakPrunedModuleStub_rdp() { yakPrunedModulePanic("rdp") }

//export yakPrunedModuleStub_re
//go:noinline
func yakPrunedModuleStub_re() { yakPrunedModulePanic("re") }

//export yakPrunedModuleStub_re2
//go:noinline
func yakPrunedModuleStub_re2() { yakPrunedModulePanic("re2") }

//export yakPrunedModuleStub_redis
//go:noinline
func yakPrunedModuleStub_redis() { yakPrunedModulePanic("redis") }

//export yakPrunedModuleStub_regen
//go:noinline
func yakPrunedModuleStub_regen() { yakPrunedModulePanic("regen") }

//export yakPrunedModuleStub_report
//go:noinline
func yakPrunedModuleStub_report() { yakPrunedModulePanic("report") }

//export yakPrunedModuleStub_risk
//go:noinline
func yakPrunedModuleStub_risk() { yakPrunedModulePanic("risk") }

//export yakPrunedModuleStub_sandbox
//go:noinline
func yakPrunedModuleStub_sandbox() { yakPrunedModulePanic("sandbox") }

//export yakPrunedModuleStub_sca
//go:noinline
func yakPrunedModuleStub_sca() { yakPrunedModulePanic("sca") }

//export yakPrunedModuleStub_servicescan
//go:noinline
func yakPrunedModuleStub_servicescan() { yakPrunedModulePanic("servicescan") }

//export yakPrunedModuleStub_sfreport
//go:noinline
func yakPrunedModuleStub_sfreport() { yakPrunedModulePanic("sfreport") }

//export yakPrunedModuleStub_shared
//go:noinline
func yakPrunedModuleStub_shared() { yakPrunedModulePanic("shared") }

//export yakPrunedModuleStub_sharednet
//go:noinline
func yakPrunedModuleStub_sharednet() { yakPrunedModulePanic("sharednet") }

//export yakPrunedModuleStub_simulator
//go:noinline
func yakPrunedModuleStub_simulator() { yakPrunedModulePanic("simulator") }

//export yakPrunedModuleStub_smb
//go:noinline
func yakPrunedModuleStub_smb() { yakPrunedModulePanic("smb") }

//export yakPrunedModuleStub_spacengine
//go:noinline
func yakPrunedModuleStub_spacengine() { yakPrunedModulePanic("spacengine") }

//export yakPrunedModuleStub_ssa
//go:noinline
func yakPrunedModuleStub_ssa() { yakPrunedModulePanic("ssa") }

//export yakPrunedModuleStub_ssafront
//go:noinline
func yakPrunedModuleStub_ssafront() { yakPrunedModulePanic("ssafront") }

//export yakPrunedModuleStub_ssh
//go:noinline
func yakPrunedModuleStub_ssh() { yakPrunedModulePanic("ssh") }

//export yakPrunedModuleStub_str
//go:noinline
func yakPrunedModuleStub_str() { yakPrunedModulePanic("str") }

//export yakPrunedModuleStub_subdomain
//go:noinline
func yakPrunedModuleStub_subdomain() { yakPrunedModulePanic("subdomain") }

//export yakPrunedModuleStub_suricata
//go:noinline
func yakPrunedModuleStub_suricata() { yakPrunedModulePanic("suricata") }

//export yakPrunedModuleStub_sync
//go:noinline
func yakPrunedModuleStub_sync() { yakPrunedModulePanic("sync") }

//export yakPrunedModuleStub_synscan
//go:noinline
func yakPrunedModuleStub_synscan() { yakPrunedModulePanic("synscan") }

//export yakPrunedModuleStub_syntaxflow
//go:noinline
func yakPrunedModuleStub_syntaxflow() { yakPrunedModulePanic("syntaxflow") }

//export yakPrunedModuleStub_systemd
//go:noinline
func yakPrunedModuleStub_systemd() { yakPrunedModulePanic("systemd") }

//export yakPrunedModuleStub_t3
//go:noinline
func yakPrunedModuleStub_t3() { yakPrunedModulePanic("t3") }

//export yakPrunedModuleStub_tcp
//go:noinline
func yakPrunedModuleStub_tcp() { yakPrunedModulePanic("tcp") }

//export yakPrunedModuleStub_time
//go:noinline
func yakPrunedModuleStub_time() { yakPrunedModulePanic("time") }

//export yakPrunedModuleStub_timezone
//go:noinline
func yakPrunedModuleStub_timezone() { yakPrunedModulePanic("timezone") }

//export yakPrunedModuleStub_tls
//go:noinline
func yakPrunedModuleStub_tls() { yakPrunedModulePanic("tls") }

//export yakPrunedModuleStub_tools
//go:noinline
func yakPrunedModuleStub_tools() { yakPrunedModulePanic("tools") }

//export yakPrunedModuleStub_traceroute
//go:noinline
func yakPrunedModuleStub_traceroute() { yakPrunedModulePanic("traceroute") }

//export yakPrunedModuleStub_udp
//go:noinline
func yakPrunedModuleStub_udp() { yakPrunedModulePanic("udp") }

//export yakPrunedModuleStub_webforest
//go:noinline
func yakPrunedModuleStub_webforest() { yakPrunedModulePanic("webforest") }

//export yakPrunedModuleStub_x
//go:noinline
func yakPrunedModuleStub_x() { yakPrunedModulePanic("x") }

//export yakPrunedModuleStub_xhtml
//go:noinline
func yakPrunedModuleStub_xhtml() { yakPrunedModulePanic("xhtml") }

//export yakPrunedModuleStub_xml
//go:noinline
func yakPrunedModuleStub_xml() { yakPrunedModulePanic("xml") }

//export yakPrunedModuleStub_xpath
//go:noinline
func yakPrunedModuleStub_xpath() { yakPrunedModulePanic("xpath") }

//export yakPrunedModuleStub_yakit
//go:noinline
func yakPrunedModuleStub_yakit() { yakPrunedModulePanic("yakit") }

//export yakPrunedModuleStub_yaml
//go:noinline
func yakPrunedModuleStub_yaml() { yakPrunedModulePanic("yaml") }

//export yakPrunedModuleStub_yso
//go:noinline
func yakPrunedModuleStub_yso() { yakPrunedModulePanic("yso") }

//export yakPrunedModuleStub_zip
//go:noinline
func yakPrunedModuleStub_zip() { yakPrunedModulePanic("zip") }

//export yak_register_module_ai
func yak_register_module_ai() {
	runtimeRegisterYaklibModule("ai", ai.Exports)
}

//export yak_register_module_bin
func yak_register_module_bin() {
	runtimeRegisterYaklibModule("bin", binx.Exports)
}

//export yak_register_module_bot
func yak_register_module_bot() {
	runtimeRegisterYaklibModule("bot", yaklib.BotExports)
}

//export yak_register_module_brute
func yak_register_module_brute() {
	runtimeRegisterYaklibModule("brute", tools.BruterExports)
}

//export yak_register_module_bufio
func yak_register_module_bufio() {
	runtimeRegisterYaklibModule("bufio", yaklib.BufioExport)
}

//export yak_register_module_cli
func yak_register_module_cli() {
	runtimeRegisterYaklibModule("cli", cli.CliExports)
}

//export yak_register_module_codec
func yak_register_module_codec() {
	runtimeRegisterYaklibModule("codec", aotlib.CodecExports)
}

//export yak_register_module_container
func yak_register_module_container() {
	runtimeRegisterYaklibModule("container", container.ContainerExports)
}

//export yak_register_module_context
func yak_register_module_context() {
	runtimeRegisterYaklibModule("context", yaklib.ContextExports)
}

//export yak_register_module_crawler
func yak_register_module_crawler() {
	runtimeRegisterYaklibModule("crawler", crawler.Exports)
}

//export yak_register_module_crawlerx
func yak_register_module_crawlerx() {
	runtimeRegisterYaklibModule("crawlerx", crawlerx.CrawlerXExports)
}

//export yak_register_module_csrf
func yak_register_module_csrf() {
	runtimeRegisterYaklibModule("csrf", yaklib.CSRFExports)
}

//export yak_register_module_db
func yak_register_module_db() {
	runtimeRegisterYaklibModule("db", yaklib.DatabaseExports)
}

//export yak_register_module_dictutil
func yak_register_module_dictutil() {
	runtimeRegisterYaklibModule("dictutil", yaklib.DictUtilExports)
}

//export yak_register_module_diff
func yak_register_module_diff() {
	runtimeRegisterYaklibModule("diff", yakdiff.Exports)
}

//export yak_register_module_dns
func yak_register_module_dns() {
	runtimeRegisterYaklibModule("dns", yaklib.DnsExports)
}

//export yak_register_module_dnslog
func yak_register_module_dnslog() {
	runtimeRegisterYaklibModule("dnslog", yaklib.DNSLogExports)
}

//export yak_register_module_dyn
func yak_register_module_dyn() {
	runtimeRegisterYaklibModule("dyn", yak.EvalExports)
}

//export yak_register_module_env
func yak_register_module_env() {
	runtimeRegisterYaklibModule("env", yaklib.EnvExports)
}

//export yak_register_module_excel
func yak_register_module_excel() {
	runtimeRegisterYaklibModule("excel", excelparser.ExcelExports)
}

//export yak_register_module_exec
func yak_register_module_exec() {
	runtimeRegisterYaklibModule("exec", yaklib.ExecExports)
}

//export yak_register_module_facades
func yak_register_module_facades() {
	runtimeRegisterYaklibModule("facades", facades.FacadesExports)
}

//export yak_register_module_file
func yak_register_module_file() {
	runtimeRegisterYaklibModule("file", aotlib.FileExports)
}

//export yak_register_module_filemonitor
func yak_register_module_filemonitor() {
	runtimeRegisterYaklibModule("filemonitor", yaklib.FileMonitorExports)
}

//export yak_register_module_fileparser
func yak_register_module_fileparser() {
	runtimeRegisterYaklibModule("fileparser", fileparser.Exports)
}

//export yak_register_module_filescanner
func yak_register_module_filescanner() {
	runtimeRegisterYaklibModule("filescanner", yaklib.FileScannerExports)
}

//export yak_register_module_filesys
func yak_register_module_filesys() {
	runtimeRegisterYaklibModule("filesys", filesys.Exports)
}

//export yak_register_module_finscan
func yak_register_module_finscan() {
	runtimeRegisterYaklibModule("finscan", tools.FinPortScanExports)
}

//export yak_register_module_fuzz
func yak_register_module_fuzz() {
	runtimeRegisterYaklibModule("fuzz", yaklib.FuzzExports)
}

//export yak_register_module_fuzzx
func yak_register_module_fuzzx() {
	runtimeRegisterYaklibModule("fuzzx", yaklib.FuzzxExports)
}

//export yak_register_module_git
func yak_register_module_git() {
	runtimeRegisterYaklibModule("git", yakgit.Exports)
}

//export yak_register_module_gzip
func yak_register_module_gzip() {
	runtimeRegisterYaklibModule("gzip", yaklib.GzipExports)
}

//export yak_register_module_hids
func yak_register_module_hids() {
	runtimeRegisterYaklibModule("hids", hids.Exports)
}

//export yak_register_module_hook
func yak_register_module_hook() {
	runtimeRegisterYaklibModule("hook", yak.HooksExports)
}

//export yak_register_module_http
func yak_register_module_http() {
	runtimeRegisterYaklibModule("http", yakhttp.HttpExports)
}

//export yak_register_module_httpool
func yak_register_module_httpool() {
	runtimeRegisterYaklibModule("httpool", yaklib.HttpPoolExports)
}

//export yak_register_module_httpserver
func yak_register_module_httpserver() {
	runtimeRegisterYaklibModule("httpserver", yaklib.HttpServeExports)
}

//export yak_register_module_httptpl
func yak_register_module_httptpl() {
	runtimeRegisterYaklibModule("httptpl", httptpl.MatchOrExtractExports)
}

//export yak_register_module_iiop
func yak_register_module_iiop() {
	runtimeRegisterYaklibModule("iiop", iiop.Exports)
}

//export yak_register_module_io
func yak_register_module_io() {
	runtimeRegisterYaklibModule("io", yaklib.IoExports)
}

//export yak_register_module_java
func yak_register_module_java() {
	runtimeRegisterYaklibModule("java", yserx.Exports)
}

//export yak_register_module_js
func yak_register_module_js() {
	runtimeRegisterYaklibModule("js", yaklib.JSExports)
}

//export yak_register_module_json
func yak_register_module_json() {
	runtimeRegisterYaklibModule("json", aotlib.JsonExports)
}

//export yak_register_module_jsonschema
func yak_register_module_jsonschema() {
	runtimeRegisterYaklibModule("jsonschema", aitool.SchemaGeneratorExports)
}

//export yak_register_module_jsonstream
func yak_register_module_jsonstream() {
	runtimeRegisterYaklibModule("jsonstream", yaklib.JsonStreamExports)
}

//export yak_register_module_judge
func yak_register_module_judge() {
	runtimeRegisterYaklibModule("judge", comparer.Exports)
}

//export yak_register_module_jwt
func yak_register_module_jwt() {
	runtimeRegisterYaklibModule("jwt", authhack.JWTExports)
}

//export yak_register_module_ldap
func yak_register_module_ldap() {
	runtimeRegisterYaklibModule("ldap", yaklib.LdapExports)
}

//export yak_register_module_liteforge
func yak_register_module_liteforge() {
	runtimeRegisterYaklibModule("liteforge", aiforge.LiteForgeExport)
}

//export yak_register_module_log
func yak_register_module_log() {
	runtimeRegisterYaklibModule("log", aotlib.LogExports)
}

//export yak_register_module_math
func yak_register_module_math() {
	runtimeRegisterYaklibModule("math", yaklib.MathExport)
}

//export yak_register_module_mitm
func yak_register_module_mitm() {
	runtimeRegisterYaklibModule("mitm", yaklib.MitmExports)
}

//export yak_register_module_mmdb
func yak_register_module_mmdb() {
	runtimeRegisterYaklibModule("mmdb", yaklib.MmdbExports)
}

//export yak_register_module_nasl
func yak_register_module_nasl() {
	runtimeRegisterYaklibModule("nasl", antlr4nasl.Exports)
}

//export yak_register_module_netstack
func yak_register_module_netstack() {
	runtimeRegisterYaklibModule("netstack", netstack_exports.Exports)
}

//export yak_register_module_nuclei
func yak_register_module_nuclei() {
	runtimeRegisterYaklibModule("nuclei", httptpl.Exports)
}

//export yak_register_module_omnisearch
func yak_register_module_omnisearch() {
	runtimeRegisterYaklibModule("omnisearch", omnisearch.Exports)
}

//export yak_register_module_openapi
func yak_register_module_openapi() {
	runtimeRegisterYaklibModule("openapi", openapi.Exports)
}

//export yak_register_module_os
func yak_register_module_os() {
	runtimeRegisterYaklibModule("os", aotlib.SystemExports)
}

//export yak_register_module_ping
func yak_register_module_ping() {
	runtimeRegisterYaklibModule("ping", tools.PingExports)
}

//export yak_register_module_poc
func yak_register_module_poc() {
	runtimeRegisterYaklibModule("poc", poc.PoCExports)
}

//export yak_register_module_pprof
func yak_register_module_pprof() {
	runtimeRegisterYaklibModule("pprof", pprofutils.Exports)
}

//export yak_register_module_rag
func yak_register_module_rag() {
	runtimeRegisterYaklibModule("rag", yak.RagExports)
}

//export yak_register_module_rdp
func yak_register_module_rdp() {
	runtimeRegisterYaklibModule("rdp", yaklib.RdpExports)
}

//export yak_register_module_re
func yak_register_module_re() {
	runtimeRegisterYaklibModule("re", yaklib.RegexpExport)
}

//export yak_register_module_re2
func yak_register_module_re2() {
	runtimeRegisterYaklibModule("re2", yaklib.Regexp2Export)
}

//export yak_register_module_redis
func yak_register_module_redis() {
	runtimeRegisterYaklibModule("redis", yaklib.RedisExports)
}

//export yak_register_module_regen
func yak_register_module_regen() {
	runtimeRegisterYaklibModule("regen", yaklib.RegenExports)
}

//export yak_register_module_report
func yak_register_module_report() {
	runtimeRegisterYaklibModule("report", yakit.ReportExports)
}

//export yak_register_module_risk
func yak_register_module_risk() {
	runtimeRegisterYaklibModule("risk", yaklib.RiskExports)
}

//export yak_register_module_sandbox
func yak_register_module_sandbox() {
	runtimeRegisterYaklibModule("sandbox", yak.SandboxExports)
}

//export yak_register_module_sca
func yak_register_module_sca() {
	runtimeRegisterYaklibModule("sca", sca.Exports)
}

//export yak_register_module_servicescan
func yak_register_module_servicescan() {
	runtimeRegisterYaklibModule("servicescan", tools.FingerprintScanExports)
}

//export yak_register_module_sfreport
func yak_register_module_sfreport() {
	runtimeRegisterYaklibModule("sfreport", sfreport.Exports)
}

//export yak_register_module_simulator
func yak_register_module_simulator() {
	runtimeRegisterYaklibModule("simulator", simulator.Exports)
}

//export yak_register_module_smb
func yak_register_module_smb() {
	runtimeRegisterYaklibModule("smb", yaklib.SambaExports)
}

//export yak_register_module_spacengine
func yak_register_module_spacengine() {
	runtimeRegisterYaklibModule("spacengine", yaklib.SpaceEngineExports)
}

//export yak_register_module_ssa
func yak_register_module_ssa() {
	runtimeRegisterYaklibModule("ssa", ssaapi.Exports)
	runtimeRegisterYaklibModule("ssa", ssaproject.Exports)
	runtimeRegisterYaklibModule("ssa", ssaconfig.Exports)
}

//export yak_register_module_ssh
func yak_register_module_ssh() {
	runtimeRegisterYaklibModule("ssh", yaklib.SSHExports)
}

//export yak_register_module_str
func yak_register_module_str() {
	runtimeRegisterYaklibModule("str", aotlib.StringsExports)
}

//export yak_register_module_subdomain
func yak_register_module_subdomain() {
	runtimeRegisterYaklibModule("subdomain", tools.SubDomainExports)
}

//export yak_register_module_suricata
func yak_register_module_suricata() {
	runtimeRegisterYaklibModule("suricata", chaosmaker.ChaosMakerExports)
}

//export yak_register_module_sync
func yak_register_module_sync() {
	runtimeRegisterYaklibModule("sync", aotlib.SyncExports)
}

//export yak_register_module_synscan
func yak_register_module_synscan() {
	runtimeRegisterYaklibModule("synscan", tools.SynxPortScanExports)
}

//export yak_register_module_syntaxflow
func yak_register_module_syntaxflow() {
	runtimeRegisterYaklibModule("syntaxflow", syntaxflow.Exports)
	runtimeRegisterYaklibModule("syntaxflow", syntaxflow_scan.Exports)
}

//export yak_register_module_systemd
func yak_register_module_systemd() {
	runtimeRegisterYaklibModule("systemd", systemd.Exports)
}

//export yak_register_module_t3
func yak_register_module_t3() {
	runtimeRegisterYaklibModule("t3", t3.Exports)
}

//export yak_register_module_tcp
func yak_register_module_tcp() {
	runtimeRegisterYaklibModule("tcp", yaklib.TcpExports)
}

//export yak_register_module_time
func yak_register_module_time() {
	runtimeRegisterYaklibModule("time", aotlib.TimeExports)
}

//export yak_register_module_timezone
func yak_register_module_timezone() {
	runtimeRegisterYaklibModule("timezone", yaklib.TimeZoneExports)
}

//export yak_register_module_tls
func yak_register_module_tls() {
	runtimeRegisterYaklibModule("tls", yaklib.TlsExports)
}

//export yak_register_module_tools
func yak_register_module_tools() {
	runtimeRegisterYaklibModule("tools", tools.Exports)
}

//export yak_register_module_traceroute
func yak_register_module_traceroute() {
	runtimeRegisterYaklibModule("traceroute", yaklib.TracerouteExports)
}

//export yak_register_module_udp
func yak_register_module_udp() {
	runtimeRegisterYaklibModule("udp", yaklib.UDPExport)
}

//export yak_register_module_webforest
func yak_register_module_webforest() {
	runtimeRegisterYaklibModule("webforest", webforest.Exports)
}

//export yak_register_module_x
func yak_register_module_x() {
	runtimeRegisterYaklibModule("x", yaklib.FunkExports)
}

//export yak_register_module_xhtml
func yak_register_module_xhtml() {
	runtimeRegisterYaklibModule("xhtml", xhtml.Exports)
}

//export yak_register_module_xml
func yak_register_module_xml() {
	runtimeRegisterYaklibModule("xml", yaklib.XMLExports)
}

//export yak_register_module_xpath
func yak_register_module_xpath() {
	runtimeRegisterYaklibModule("xpath", htmlquery.Exports)
}

//export yak_register_module_yakit
func yak_register_module_yakit() {
	runtimeRegisterYaklibModule("yakit", yakshim.YakitExports)
}

//export yak_register_module_yaml
func yak_register_module_yaml() {
	runtimeRegisterYaklibModule("yaml", yaklib.YamlExports)
}

//export yak_register_module_yso
func yak_register_module_yso() {
	runtimeRegisterYaklibModule("yso", yso.Exports)
}

//export yak_register_module_zip
func yak_register_module_zip() {
	runtimeRegisterYaklibModule("zip", yaklib.ZipExports)
}

