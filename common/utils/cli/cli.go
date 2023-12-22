package cli

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type cliExtraParams struct {
	optName      string
	optShortName string
	params       []string
	defaultValue interface{}
	helpInfo     string
	required     bool
	tempArgs     []string
	_type        string
}

var (
	Args []string

	cliParamInvalid = utils.NewBool(false)
	cliName         = "cmd"
	cliDocument     = ""
	errorMsg        = ""

	helpParam = &cliExtraParams{
		optShortName: "h",
		optName:      "help",
		params:       []string{"-h", "--help"},
		defaultValue: false,
		helpInfo:     "Show help information",
		required:     false,
		_type:        "bool",
	}

	currentExtraParams []*cliExtraParams = []*cliExtraParams{
		helpParam,
	}
)

func init() {
	Args = os.Args[:]
	if len(Args) > 1 {
		filename := filepath.Base(os.Args[1])
		fileSuffix := path.Ext(filename)
		cliName = strings.TrimSuffix(filename, fileSuffix)
	}
}

func InjectCliArgs(args []string) {
	Args = args
}

func (param *cliExtraParams) foundArgsIndex() int {
	args := _getArgs()
	if param.tempArgs != nil {
		args = param.tempArgs
	}
	for _, opt := range param.params {
		if ret := utils.StringArrayIndex(args, opt); ret < 0 {
			continue
		} else {
			return ret
		}
	}
	return -1
}

func (param *cliExtraParams) GetDefaultValue(i interface{}) interface{} {
	if param.defaultValue != nil {
		return param.defaultValue
	}

	if !param.required {
		return i
	}

	cliParamInvalid.Set()
	errorMsg += fmt.Sprintf("\n  Parameter [%s] error: miss parameter", param.optName)
	return i
}

type SetCliExtraParam func(c *cliExtraParams)

// help 用于输出命令行程序的帮助信息
// Example:
// ```
// cli.help()
// ```
func _help(w ...io.Writer) {
	var writer io.Writer = os.Stdout

	if len(w) > 0 {
		writer = w[0]
	}

	fmt.Fprintln(writer, "Usage: ")
	fmt.Fprintf(writer, "  %s [OPTIONS]\n", cliName)
	fmt.Fprintln(writer)
	if len(cliDocument) > 0 {
		fmt.Fprintln(writer, cliDocument)
		fmt.Fprintln(writer)
	}
	fmt.Fprintln(writer, "Flags:")
	for _, param := range currentExtraParams {
		paramType := param._type
		// bool类型不显示paramType
		if paramType == "bool" {
			paramType = ""
		}
		helpInfo := param.helpInfo
		if param.defaultValue != nil {
			helpInfo += fmt.Sprintf(" (default %v)", param.defaultValue)
		}
		flag := fmt.Sprintf("  %s %s", strings.Join(param.params, ", "), paramType)
		padding := ""
		if len(flag) < 30 {
			padding = strings.Repeat(" ", 30-len(flag))
		}

		fmt.Fprintf(writer, "%v%v%v\n", flag, padding, param.helpInfo)
	}
}

// check 用于检查命令行参数是否合法，这主要检查必要参数是否传入与传入值是否合法
// Example:
// ```
// target = cli.String("target", cli.SetRequired(true))
// cli.check()
// ```
func _cliCheck() {
	if helpParam.foundArgsIndex() != -1 {
		_help()
		os.Exit(1)
	} else if cliParamInvalid.IsSet() {
		errorMsg = strings.TrimSpace(errorMsg)
		if len(errorMsg) > 0 {
			fmt.Printf("Error:\n  %s\n\n", errorMsg)
		}
		_help()

	}
}

// SetCliName 设置此命令行程序的名称
// 这会在命令行输入 --help 或执行`cli.check()`后参数非法时显示
// Example:
// ```
// cli.SetCliName("example-tools")
// ```
func _cliSetName(name string) {
	cliName = name
}

// SetDoc 设置此命令行程序的文档
// 这会在命令行输入 --help 或执行`cli.check()`后参数非法时显示
// Example:
// ```
// cli.SetDoc("example-tools is a tool for example")
// ```
func _cliSetDocument(document string) {
	cliDocument = document
}

// setDefaultValue 是一个选项函数，设置参数的默认值
// Example:
// ```
// cli.String("target", cli.SetDefaultValue("yaklang.com"))
// ```
func _cliSetDefaultValue(i interface{}) SetCliExtraParam {
	return func(c *cliExtraParams) {
		c.defaultValue = i
	}
}

// setHelp 是一个选项函数，设置参数的帮助信息
// 这会在命令行输入 --help 或执行`cli.check()`后参数非法时显示
// Example:
// ```
// cli.String("target", cli.SetHelp("target host or ip"))
// ```
func _cliSetHelpInfo(i string) SetCliExtraParam {
	return func(c *cliExtraParams) {
		c.helpInfo = i
	}
}

func SetTempArgs(args []string) SetCliExtraParam {
	return func(c *cliExtraParams) {
		c.tempArgs = args
	}
}

// setRequired 是一个选项函数，设置参数是否必须
// Example:
// ```
// cli.String("target", cli.SetRequired(true))
// ```
func _cliSetRequired(t bool) SetCliExtraParam {
	return func(c *cliExtraParams) {
		c.required = t
	}
}

func _getExtraParams(name string, opts ...SetCliExtraParam) *cliExtraParams {
	optName := name
	optShortName := ""
	if strings.Contains(name, " ") {
		nameSlice := strings.SplitN(name, " ", 2)
		optShortName = nameSlice[0]
		optName = nameSlice[1]
	} else if strings.Contains(name, ",") {
		nameSlice := strings.SplitN(name, ",", 2)
		optShortName = nameSlice[0]
		optName = nameSlice[1]
	}

	if len(name) == 1 && optShortName == "" {
		optShortName = name
		optName = name
	}

	param := &cliExtraParams{
		optName:      optName,
		optShortName: optShortName,
		params:       _getAvailableParams(optName, optShortName),
		required:     false,
		defaultValue: nil,
		helpInfo:     "",
	}
	for _, opt := range opts {
		opt(param)
	}
	currentExtraParams = append(currentExtraParams, param)
	return param
}

// ------------------------------------------------

func _getAvailableParams(optName, optShortName string) []string {
	optName, optShortName = strings.TrimLeft(optName, "-"), strings.TrimLeft(optShortName, "-")

	if optShortName == "" {
		return []string{fmt.Sprintf("--%v", optName)}
	}
	return []string{fmt.Sprintf("-%v", optShortName), fmt.Sprintf("--%v", optName)}
}

// Args 获取命令行参数
// Example:
// ```
// Args = cli.Args()
// ```
func _getArgs() []string {
	return Args
}

func _cliFromString(name string, opts ...SetCliExtraParam) (string, *cliExtraParams) {
	param := _getExtraParams(name, opts...)
	index := param.foundArgsIndex()
	if index < 0 {
		return utils.InterfaceToString(param.GetDefaultValue("")), param
	}
	args := _getArgs()
	if param.tempArgs != nil {
		args = param.tempArgs
	}
	if index+1 >= len(args) {
		// 防止数组越界
		return utils.InterfaceToString(param.GetDefaultValue("")), param
	}
	return args[index+1], param
}

// Bool 获取对应名称的命令行参数，并将其转换为 bool 类型返回
// Example:
// ```
// verbose = cli.Bool("verbose") // --verbose 则为true
// ```
func _cliBool(name string, opts ...SetCliExtraParam) bool {
	c := _getExtraParams(name, opts...)
	c._type = "bool"
	c.required = false

	index := c.foundArgsIndex()
	if index < 0 {
		return false // c.GetDefaultValue(false).(bool)
	}
	return true
}

// String 获取对应名称的命令行参数，并将其转换为 string 类型返回
// Example:
// ```
// target = cli.String("target") // --target yaklang.com 则 target 为 yaklang.com
// ```
func CliString(name string, opts ...SetCliExtraParam) string {
	s, c := _cliFromString(name, opts...)
	c._type = "string"
	return s
}

func parseInt(s string) int {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		log.Errorf("parse int[%s] failed: %s", s, err)
		return 0
	}
	return int(i)
}

// Int 获取对应名称的命令行参数，并将其转换为 int 类型返回
// Example:
// ```
// port = cli.Int("port") // --port 80 则 port 为 80
// ```
func _cliInt(name string, opts ...SetCliExtraParam) int {
	s, c := _cliFromString(name, opts...)
	c._type = "int"
	if s == "" {
		return 0
	}
	return parseInt(s)
}

func parseFloat(s string) float64 {
	i, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Errorf("parse float[%s] failed: %s", s, err)
		return 0
	}
	return float64(i)
}

// Float 获取对应名称的命令行参数，并将其转换为 float 类型返回
// Example:
// ```
// percent = cli.Float("percent") // --percent 0.5 则 percent 为 0.5
func _cliFloat(name string, opts ...SetCliExtraParam) float64 {
	s, c := _cliFromString(name, opts...)
	c._type = "float"
	if s == "" {
		return 0.0
	}
	return parseFloat(s)
}

// Urls 获取对应名称的命令行参数，根据","切割并尝试将其转换为符合URL格式并返回 []string 类型
// Example:
// ```
// urls = cli.Urls("urls")
// // --urls yaklang.com:443,google.com:443 则 urls 为 ["https://yaklang.com", "https://google.com"]
// ```
func _cliUrls(name string, opts ...SetCliExtraParam) []string {
	s, c := _cliFromString(name, opts...)
	c._type = "urls"
	ret := utils.ParseStringToUrlsWith3W(utils.ParseStringToHosts(s)...)
	if ret == nil {
		return []string{}
	}
	return ret
}

func _cliPort(name string, opts ...SetCliExtraParam) []int {
	s, c := _cliFromString(name, opts...)
	c._type = "port"
	ret := utils.ParseStringToPorts(s)
	if ret == nil {
		return []int{}
	}
	return ret
}

// Hosts 获取对应名称的命令行参数，根据","切割并尝试解析CIDR网段并返回 []string 类型
// Example:
// ```
// hosts = cli.Hosts("hosts")
// // --hosts 192.168.0.0/24,172.17.0.1 则 hosts 为 192.168.0.0/24对应的所有IP和172.17.0.1
func _cliHosts(name string, opts ...SetCliExtraParam) []string {
	s, c := _cliFromString(name, opts...)
	c._type = "hosts"
	ret := utils.ParseStringToHosts(s)
	if ret == nil {
		cliParamInvalid.Set()
		errorMsg += fmt.Sprintf("\n  Parameter [%s] error: Parse string to host error: %s", c.optName, s)
		return []string{}
	}
	return ret
}

// File 获取对应名称的命令行参数，根据其传入的值读取其对应文件内容并返回 []byte 类型
// Example:
// ```
// file = cli.File("file")
// // --file /etc/passwd 则 file 为 /etc/passwd 文件中的内容
// ```
func _cliFile(name string, opts ...SetCliExtraParam) []byte {
	s, c := _cliFromString(name, opts...)
	c._type = "file"
	c.required = true

	if cliParamInvalid.IsSet() {
		return []byte{}
	}

	if utils.GetFirstExistedPath(s) == "" && !cliParamInvalid.IsSet() {
		cliParamInvalid.Set()
		errorMsg += fmt.Sprintf("\n  Parameter [%s] error: No such file: %s", c.optName, s)
		return []byte{}
	}
	raw, err := ioutil.ReadFile(s)
	if err != nil {
		cliParamInvalid.Set()
		errorMsg += fmt.Sprintf("\n  Parameter [%s] error: %s", c.optName, err.Error())
		return []byte{}
	}

	return raw
}

// FileOrContent 获取对应名称的命令行参数
// 根据其传入的值尝试读取其对应文件内容，如果无法读取则直接返回，最后返回 []byte 类型
// Example:
// ```
// foc = cli.FileOrContent("foc")
// // --foc /etc/passwd 则 foc 为 /etc/passwd 文件中的内容
// // --file "asd" 则 file 为 "asd"
// ```
func _cliFileOrContent(name string, opts ...SetCliExtraParam) []byte {
	s, c := _cliFromString(name, opts...)
	c._type = "file_or_content"
	ret := utils.StringAsFileParams(s)
	if ret == nil {
		cliParamInvalid.Set()
		errorMsg += fmt.Sprintf("\n  Parameter [%s] error: Empty file or content: %s", c.optName, s)
		return []byte{}
	}
	return ret
}

// LineDict 获取对应名称的命令行参数
// 根据其传入的值尝试读取其对应文件内容，如果无法读取则作为字符串，最后根据换行符切割，返回 []string 类型
// Example:
// ```
// dict = cli.LineDict("dict")
// // --dict /etc/passwd 则 dict 为 /etc/passwd 文件中的逐行的内容
// // --dict "asd" 则 dict 为 ["asd"]
// ```
func _cliLineDict(name string, opts ...SetCliExtraParam) []string {
	s, c := _cliFromString(name, opts...)
	c._type = "file-or-content"
	raw := utils.StringAsFileParams(s)
	if raw == nil {
		cliParamInvalid.Set()
		errorMsg += fmt.Sprintf("\n  Parameter [%s] error: Empty file or content: %s", c.optName, s)
		return []string{}
	}

	return utils.ParseStringToLines(string(raw))
}

// YakitPlugin 获取名称为 yakit-plugin-file 的命令行参数
// 根据其传入的值读取其对应文件内容并根据"|"切割并返回 []string 类型，表示各个插件名
// Example:
// ```
// plugins = cli.YakitPlugin()
// // --yakit-plugin-file plugins.txt 则 plugins 为 plugins.txt 文件中的各个插件名
// ```
func _cliYakitPluginFiles() []string {
	paramName := "yakit-plugin-file"
	filename, c := _cliFromString(paramName, _cliSetDefaultValue(""))
	c._type = "yakit-plugin"

	raw, err := ioutil.ReadFile(filename)
	if err != nil {
		cliParamInvalid.Set()
		errorMsg += fmt.Sprintf("\n  Parameter [%s] error: %s", c.optName, err.Error())
		return []string{}
	}
	if raw == nil {
		cliParamInvalid.Set()
		errorMsg += fmt.Sprintf("\n  Parameter [%s] error: Can't read file: %s", c.optName, filename)
		return []string{}
	}
	return utils.PrettifyListFromStringSplited(string(raw), "|")
}

// StringSlice 获取对应名称的命令行参数，将其字符串根据","切割返回 []string 类型
// Example:
// ```
// targets = cli.StringSlice("targets")
// // --targets yaklang.com,google.com 则 targets 为 ["yaklang.com", "google.com"]
// ```
func CliStringSlice(name string) []string {
	rawStr, c := _cliFromString(name, _cliSetDefaultValue(""))
	c._type = "string-slice"

	if rawStr == "" {
		return []string{}
	}

	return utils.PrettifyListFromStringSplited(rawStr, ",")
}

var CliExports = map[string]interface{}{
	"Args":        _getArgs,
	"Bool":        _cliBool,
	"Have":        _cliBool,
	"String":      CliString,
	"Int":         _cliInt,
	"Integer":     _cliInt,
	"Float":       _cliFloat,
	"Double":      _cliFloat,
	"YakitPlugin": _cliYakitPluginFiles,
	"StringSlice": CliStringSlice,

	// 解析成 URL
	"Urls": _cliUrls,
	"Url":  _cliUrls,

	// 解析端口
	"Ports": _cliPort,
	"Port":  _cliPort,

	// 解析网络目标
	"Hosts":   _cliHosts,
	"Host":    _cliHosts,
	"Network": _cliHosts,
	"Net":     _cliHosts,

	// 解析文件之类的
	"File":          _cliFile,
	"FileOrContent": _cliFileOrContent,
	"LineDict":      _cliLineDict,

	// 设置param属性
	"setHelp":     _cliSetHelpInfo,
	"setDefault":  _cliSetDefaultValue,
	"setRequired": _cliSetRequired,

	// 设置cli属性
	"SetCliName": _cliSetName,
	"SetDoc":     _cliSetDocument,

	// 通用函数
	"help":  _help,
	"check": _cliCheck,
}
