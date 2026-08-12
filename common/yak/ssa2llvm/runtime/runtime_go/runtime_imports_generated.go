package main

import "C"

import (
	cli "github.com/yaklang/yaklang/common/utils/cli"
	comparer "github.com/yaklang/yaklang/common/utils/comparer"
	filesys "github.com/yaklang/yaklang/common/utils/filesys"
	htmlquery "github.com/yaklang/yaklang/common/utils/htmlquery"
	poc "github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	xhtml "github.com/yaklang/yaklang/common/xhtml"
	httptpl "github.com/yaklang/yaklang/common/yak/httptpl"
	yakshim "github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/shim"
	ssaapi "github.com/yaklang/yaklang/common/yak/ssaapi"
	ssaconfig "github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	ssaproject "github.com/yaklang/yaklang/common/yak/ssaproject"
	yaklib "github.com/yaklang/yaklang/common/yak/yaklib"
	container "github.com/yaklang/yaklang/common/yak/yaklib/container"
	yakhttp "github.com/yaklang/yaklang/common/yak/yaklib/yakhttp"
	_ "unsafe"
)

func init() {}

//export yak_register_globals
func yak_register_globals() {
	registerRuntimeGlobals()
}

//export yakUnusedModuleStub
//go:noinline
func yakUnusedModuleStub() {}

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
	runtimeRegisterYaklibModule("codec", yaklib.CodecExports)
}

//export yak_register_module_container
func yak_register_module_container() {
	runtimeRegisterYaklibModule("container", container.ContainerExports)
}

//export yak_register_module_context
func yak_register_module_context() {
	runtimeRegisterYaklibModule("context", yaklib.ContextExports)
}

//export yak_register_module_dictutil
func yak_register_module_dictutil() {
	runtimeRegisterYaklibModule("dictutil", yaklib.DictUtilExports)
}

//export yak_register_module_dns
func yak_register_module_dns() {
	runtimeRegisterYaklibModule("dns", yaklib.DnsExports)
}

//export yak_register_module_env
func yak_register_module_env() {
	runtimeRegisterYaklibModule("env", yaklib.EnvExports)
}

//export yak_register_module_exec
func yak_register_module_exec() {
	runtimeRegisterYaklibModule("exec", yaklib.ExecExports)
}

//export yak_register_module_file
func yak_register_module_file() {
	runtimeRegisterYaklibModule("file", yaklib.FileExport)
}

//export yak_register_module_filesys
func yak_register_module_filesys() {
	runtimeRegisterYaklibModule("filesys", filesys.Exports)
}

//export yak_register_module_fuzz
func yak_register_module_fuzz() {
	runtimeRegisterYaklibModule("fuzz", yaklib.FuzzExports)
}

//export yak_register_module_gzip
func yak_register_module_gzip() {
	runtimeRegisterYaklibModule("gzip", yaklib.GzipExports)
}

//export yak_register_module_http
func yak_register_module_http() {
	runtimeRegisterYaklibModule("http", yakhttp.HttpExports)
}

//export yak_register_module_httpool
func yak_register_module_httpool() {
	runtimeRegisterYaklibModule("httpool", yaklib.HttpPoolExports)
}

//export yak_register_module_httptpl
func yak_register_module_httptpl() {
	runtimeRegisterYaklibModule("httptpl", httptpl.MatchOrExtractExports)
}

//export yak_register_module_io
func yak_register_module_io() {
	runtimeRegisterYaklibModule("io", yaklib.IoExports)
}

//export yak_register_module_json
func yak_register_module_json() {
	runtimeRegisterYaklibModule("json", yaklib.JsonExports)
}

//export yak_register_module_judge
func yak_register_module_judge() {
	runtimeRegisterYaklibModule("judge", comparer.Exports)
}

//export yak_register_module_log
func yak_register_module_log() {
	runtimeRegisterYaklibModule("log", yaklib.LogExports)
}

//export yak_register_module_math
func yak_register_module_math() {
	runtimeRegisterYaklibModule("math", yaklib.MathExport)
}

//export yak_register_module_os
func yak_register_module_os() {
	runtimeRegisterYaklibModule("os", yaklib.SystemExports)
}

//export yak_register_module_poc
func yak_register_module_poc() {
	runtimeRegisterYaklibModule("poc", poc.PoCExports)
}

//export yak_register_module_re
func yak_register_module_re() {
	runtimeRegisterYaklibModule("re", yaklib.RegexpExport)
}

//export yak_register_module_re2
func yak_register_module_re2() {
	runtimeRegisterYaklibModule("re2", yaklib.Regexp2Export)
}

//export yak_register_module_ssa
func yak_register_module_ssa() {
	runtimeRegisterYaklibModule("ssa", ssaapi.Exports)
	runtimeRegisterYaklibModule("ssa", ssaproject.Exports)
	runtimeRegisterYaklibModule("ssa", ssaconfig.Exports)
}

//export yak_register_module_str
func yak_register_module_str() {
	runtimeRegisterYaklibModule("str", yaklib.StringsExport)
}

//export yak_register_module_sync
func yak_register_module_sync() {
	runtimeRegisterYaklibModule("sync", yaklib.SyncExport)
}

//export yak_register_module_time
func yak_register_module_time() {
	runtimeRegisterYaklibModule("time", yaklib.TimeExports)
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

//export yak_register_module_zip
func yak_register_module_zip() {
	runtimeRegisterYaklibModule("zip", yaklib.ZipExports)
}
