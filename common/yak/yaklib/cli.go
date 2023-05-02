package yaklib

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"github.com/yaklang/yaklang/common/utils"
)

type cliExtraParams struct {
	optName      string
	optShortName string
	params       []string
	defaultValue interface{}
	helpInfo     string
	required     bool
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

func (param *cliExtraParams) foundArgsIndex() int {
	args := _getArgs()
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

type setCliExtraParam func(c *cliExtraParams)

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

func _cliSetName(name string) {
	cliName = name
}

func _cliSetDocument(document string) {
	cliDocument = document
}

func _cliSetDefaultValue(i interface{}) setCliExtraParam {
	return func(c *cliExtraParams) {
		c.defaultValue = i
	}
}

func _cliSetHelpInfo(i string) setCliExtraParam {
	return func(c *cliExtraParams) {
		c.helpInfo = i
	}
}

func _cliSetRequired(t bool) setCliExtraParam {
	return func(c *cliExtraParams) {
		c.required = t
	}
}

func _getExtraParams(name string, opts ...setCliExtraParam) *cliExtraParams {
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
		return []string{_sfmt("--%v", optName)}
	}
	return []string{_sfmt("-%v", optShortName), _sfmt("--%v", optName)}
}

func _getArgs() []string {
	return Args
}

func _cliFromString(name string, opts ...setCliExtraParam) (string, *cliExtraParams) {
	param := _getExtraParams(name, opts...)
	index := param.foundArgsIndex()
	if index <= 0 {
		return utils.InterfaceToString(param.GetDefaultValue("")), param
	}
	args := _getArgs()
	if index+1 >= len(args) {
		// 防止数组越界
		return utils.InterfaceToString(param.GetDefaultValue("")), param
	}
	return args[index+1], param
}

func _cliBool(name string, opts ...setCliExtraParam) bool {
	c := _getExtraParams(name, opts...)
	c._type = "bool"
	c.required = false

	index := c.foundArgsIndex()
	if index < 0 {
		return false // c.GetDefaultValue(false).(bool)
	}
	return true
}

func _cliString(name string, opts ...setCliExtraParam) string {
	s, c := _cliFromString(name, opts...)
	c._type = "string"
	return s
}

func _cliInt(name string, opts ...setCliExtraParam) int {
	s, c := _cliFromString(name, opts...)
	c._type = "int"
	if s == "" {
		return 0
	}
	return parseInt(s)
}

func _cliFloat(name string, opts ...setCliExtraParam) float64 {
	s, c := _cliFromString(name, opts...)
	c._type = "float"
	if s == "" {
		return 0.0
	}
	return parseFloat(s)
}

func _cliUrls(name string, opts ...setCliExtraParam) []string {
	s, c := _cliFromString(name, opts...)
	c._type = "urls"
	ret := utils.ParseStringToUrlsWith3W(utils.ParseStringToHosts(s)...)
	if ret == nil {
		return []string{}
	}
	return ret
}

func _cliPort(name string, opts ...setCliExtraParam) []int {
	s, c := _cliFromString(name, opts...)
	c._type = "port"
	ret := utils.ParseStringToPorts(s)
	if ret == nil {
		return []int{}
	}
	return ret
}

func _cliHosts(name string, opts ...setCliExtraParam) []string {
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

func _cliFile(name string, opts ...setCliExtraParam) []byte {
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

func _cliFileOrContent(name string, opts ...setCliExtraParam) []byte {
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

func _cliLineDict(name string, opts ...setCliExtraParam) []string {
	bytes, c := _cliFromString(name, opts...)
	c._type = "file-or-content"
	raw := utils.StringAsFileParams(bytes)
	if raw == nil {
		cliParamInvalid.Set()
		errorMsg += fmt.Sprintf("\n  Parameter [%s] error: Empty file or content: %s", c.optName, bytes)
		return []string{}
	}

	return utils.ParseStringToLines(string(raw))
}

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

func _cliStringSlice(name string) []string {
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
	"String":      _cliString,
	"Int":         _cliInt,
	"Integer":     _cliInt,
	"Float":       _cliFloat,
	"Double":      _cliFloat,
	"YakitPlugin": _cliYakitPluginFiles,
	"StringSlice": _cliStringSlice,

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
	"help": _help,
	"check": func() {
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
	},
}
